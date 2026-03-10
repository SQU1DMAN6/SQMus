package audio

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"os/exec"

	"sqmus/internal/ast"
	"sqmus/internal/compiler"
)

const (
	defaultSampleRate = 44100
	maxInt16          = 32767
)

// RenderWAV renders a score to a mono 16-bit PCM WAV file.
func RenderWAV(score *compiler.Score, outputPath string) error {
	if score == nil {
		return fmt.Errorf("score is nil")
	}
	if outputPath == "" {
		return fmt.Errorf("output path is required")
	}

	data, err := encodeWAV(score, defaultSampleRate)
	if err != nil {
		return err
	}
	return os.WriteFile(outputPath, data, 0o644)
}

func Play(score *compiler.Score) error {
	if score == nil {
		return fmt.Errorf("score is nil")
	}

	tmpFile, err := os.CreateTemp("", "sqmus-*.wav")
	if err != nil {
		return fmt.Errorf("create temp wav: %w", err)
	}
	tmpPath := tmpFile.Name()
	if closeErr := tmpFile.Close(); closeErr != nil {
		return fmt.Errorf("close temp wav: %w", closeErr)
	}
	defer os.Remove(tmpPath)

	if err := RenderWAV(score, tmpPath); err != nil {
		return err
	}

	players := [][]string{
		{"ffplay", "-nodisp", "-autoexit", "-loglevel", "quiet", tmpPath},
		{"aplay", tmpPath},
		{"afplay", tmpPath},
	}

	var lastErr error
	for _, player := range players {
		if _, err := exec.LookPath(player[0]); err != nil {
			continue
		}
		cmd := exec.Command(player[0], player[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		if err := cmd.Run(); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}

	if lastErr != nil {
		return fmt.Errorf("unable to play audio using ffplay/aplay/afplay: %w", lastErr)
	}
	return fmt.Errorf("no supported audio player found (tried ffplay, aplay, afplay)")
}

func encodeWAV(score *compiler.Score, sampleRate int) ([]byte, error) {
	if score.Tempo <= 0 {
		return nil, fmt.Errorf("invalid tempo %d", score.Tempo)
	}
	if score.PPQ <= 0 {
		return nil, fmt.Errorf("invalid PPQ %d", score.PPQ)
	}

	secondsPerTick := 60.0 / (float64(score.Tempo) * float64(score.PPQ))
	totalSeconds := float64(score.TotalTicks)*secondsPerTick + 0.8 // keep tail for ambience
	if totalSeconds < 1.0 {
		totalSeconds = 1.0
	}
	totalSamples := int(totalSeconds * float64(sampleRate))
	if totalSamples <= 0 {
		totalSamples = sampleRate
	}

	samples := renderNotes(score, sampleRate, totalSamples, secondsPerTick)
	samples = applyToneFilter(samples, score.Config, sampleRate)
	samples = applyDrive(samples, score.Config)
	samples = applyChorus(samples, score.Config, sampleRate)
	samples = applyDelay(samples, score.Config, sampleRate)
	samples = applyReverb(samples, score.Config, sampleRate)

	pcm := make([]int16, totalSamples)
	for i, sample := range samples {
		if sample > 1.0 {
			sample = 1.0
		} else if sample < -1.0 {
			sample = -1.0
		}
		pcm[i] = int16(sample * maxInt16)
	}

	return wavBytes(pcm, sampleRate), nil
}

func renderNotes(score *compiler.Score, sampleRate, totalSamples int, secondsPerTick float64) []float64 {
	samples := make([]float64, totalSamples)
	cfg := score.Config

	attackSamples := int((0.002 + (1.0-clamp01(cfg.PickAttack))*0.006) * float64(sampleRate))
	releaseSamples := int((0.010 + clamp01(cfg.StringDamping)*0.016) * float64(sampleRate))
	if attackSamples < 1 {
		attackSamples = 1
	}
	if releaseSamples < 1 {
		releaseSamples = 1
	}

	for _, note := range score.Notes {
		startSample := int(float64(note.StartTicks) * secondsPerTick * float64(sampleRate))
		durationSamples := int(float64(note.DurationTicks) * secondsPerTick * float64(sampleRate))
		if durationSamples < 1 {
			durationSamples = 1
		}
		endSample := startSample + durationSamples
		if startSample >= totalSamples {
			continue
		}
		if startSample < 0 {
			startSample = 0
		}
		if endSample > totalSamples {
			endSample = totalSamples
		}

		baseGain := (0.23 + float64(note.Velocity)/255.0*0.52) * clamp01(cfg.Level)
		if baseGain <= 0 {
			baseGain = 0.1
		}

		for i := startSample; i < endSample; i++ {
			local := i - startSample
			progress := float64(local) / float64(durationSamples)
			remaining := endSample - i

			env := math.Exp(-progress * (1.2 + clamp01(cfg.StringDamping)*3.2))
			if local < attackSamples {
				env *= float64(local) / float64(attackSamples)
			}
			if remaining < releaseSamples {
				env *= float64(remaining) / float64(releaseSamples)
			}
			if env < 0 {
				env = 0
			}

			freq := noteFrequency(note, progress, cfg)
			t := float64(local) / float64(sampleRate)
			phase := 2 * math.Pi * freq * t

			fundamental := math.Sin(phase)
			second := math.Sin(2 * phase)
			third := math.Sin(3 * phase)
			body := math.Sin(0.5 * phase)

			pick := clamp01(cfg.PickAttack)
			tone := clamp01((cfg.Tone*0.7 + cfg.CabTone*0.3))
			pickup := clamp01(cfg.PickupPosition)

			signal := fundamental*(0.70-0.25*pickup) +
				second*(0.18+0.35*tone+0.15*pick) +
				third*(0.08+0.26*tone)
			signal += body * (0.05 + 0.18*clamp01(cfg.BodyResonance))

			if note.Technique == ast.TechniqueHarmonic {
				signal = second*0.55 + third*0.30 + math.Sin(4*phase)*0.18
			}

			noise := pseudoNoise(i, note.MIDI, note.String) * (0.01 + 0.05*clamp01(cfg.NoiseLevel))
			samples[i] += (signal + noise) * env * baseGain
		}
	}

	return samples
}

func noteFrequency(note compiler.NoteEvent, progress float64, cfg compiler.GuitarConfig) float64 {
	base := float64(note.MIDI)
	target := base
	if note.TechniqueTargetMIDI > 0 {
		target = float64(note.TechniqueTargetMIDI)
	}

	switch note.Technique {
	case ast.TechniqueHammer, ast.TechniquePull:
		if target != base {
			s := smoothStep(clamp(progress/0.28, 0, 1))
			return midiToFreqFloat(base + (target-base)*s)
		}
	case ast.TechniqueSlide:
		if target != base {
			s := smoothStep(clamp(progress, 0, 1))
			return midiToFreqFloat(base + (target-base)*s)
		}
	case ast.TechniqueBend:
		bendTarget := target
		if bendTarget == base {
			bendTarget = base + 2
		}
		bendProgress := clamp(progress/0.68, 0, 1)
		s := smoothStep(bendProgress)
		return midiToFreqFloat(base + (bendTarget-base)*s)
	case ast.TechniqueVibrato:
		depth := 0.15 + clamp01(cfg.Tone)*0.25
		rate := 5.2 + clamp01(cfg.PickAttack)*2.4
		mod := math.Sin(2*math.Pi*rate*progress) * depth
		return midiToFreqFloat(base + mod)
	}

	return midiToFreqFloat(base)
}

func applyToneFilter(samples []float64, cfg compiler.GuitarConfig, sampleRate int) []float64 {
	if len(samples) == 0 {
		return samples
	}

	cutoff := 550.0 + (cfg.Tone*0.65+cfg.CabTone*0.35)*5200.0
	if cutoff < 80 {
		cutoff = 80
	}

	filtered := make([]float64, len(samples))
	rc := 1.0 / (2 * math.Pi * cutoff)
	dt := 1.0 / float64(sampleRate)
	alpha := dt / (rc + dt)
	filtered[0] = samples[0]
	for i := 1; i < len(samples); i++ {
		filtered[i] = filtered[i-1] + alpha*(samples[i]-filtered[i-1])
	}

	darkness := 1.0 - clamp01(cfg.Tone*0.75+cfg.CabTone*0.25)
	blend := 0.10 + darkness*0.75
	for i := range samples {
		samples[i] = samples[i]*(1-blend) + filtered[i]*blend
	}
	return samples
}

func applyDrive(samples []float64, cfg compiler.GuitarConfig) []float64 {
	drive := clamp01(cfg.Drive*0.7 + cfg.AmpGain*0.6)
	if drive <= 0.001 {
		return samples
	}

	amount := 1.0 + drive*10.0
	norm := math.Tanh(amount)
	mix := 0.55 + clamp01(cfg.Mix)*0.45
	for i := range samples {
		dry := samples[i]
		wet := math.Tanh(dry*amount) / norm
		samples[i] = dry*(1-mix) + wet*mix
	}
	return samples
}

func applyChorus(samples []float64, cfg compiler.GuitarConfig, sampleRate int) []float64 {
	mix := clamp01(cfg.ChorusMix)
	if mix <= 0.001 || cfg.ChorusDepth <= 0.001 {
		return samples
	}

	out := make([]float64, len(samples))
	copy(out, samples)
	depth := clamp01(cfg.ChorusDepth)
	rate := clamp(cfg.ChorusRate, 0.1, 6.0)
	baseDelay := int((0.012 + depth*0.010) * float64(sampleRate))
	modWidth := int((0.003 + depth*0.008) * float64(sampleRate))
	if baseDelay < 1 {
		baseDelay = 1
	}

	for i := range out {
		lfo := 0.5 + 0.5*math.Sin(2*math.Pi*rate*float64(i)/float64(sampleRate))
		delay := baseDelay + int(float64(modWidth)*lfo)
		if i-delay < 0 {
			continue
		}
		wet := samples[i-delay]
		out[i] = out[i]*(1-mix) + (out[i]+wet*0.7)*mix
	}
	return out
}

func applyDelay(samples []float64, cfg compiler.GuitarConfig, sampleRate int) []float64 {
	mix := clamp01(cfg.DelayMix)
	if mix <= 0.001 || cfg.DelayTimeMS <= 1 {
		return samples
	}

	delaySamples := int(cfg.DelayTimeMS / 1000.0 * float64(sampleRate))
	if delaySamples < 1 {
		return samples
	}

	feedback := clamp01(cfg.DelayFeedback)
	out := make([]float64, len(samples))
	copy(out, samples)
	fb := make([]float64, len(samples))

	for i := range out {
		dry := samples[i]
		delayed := 0.0
		if i-delaySamples >= 0 {
			delayed = fb[i-delaySamples]
		}
		fb[i] = dry + delayed*feedback
		out[i] = dry*(1-mix) + delayed*mix
	}
	return out
}

func applyReverb(samples []float64, cfg compiler.GuitarConfig, sampleRate int) []float64 {
	mix := clamp01(cfg.ReverbMix)
	if mix <= 0.001 {
		return samples
	}

	room := clamp01(cfg.ReverbRoom)
	d1 := int((0.021 + room*0.050) * float64(sampleRate))
	d2 := int((0.033 + room*0.070) * float64(sampleRate))
	d3 := int((0.047 + room*0.090) * float64(sampleRate))
	if d1 < 1 {
		d1 = 1
	}
	if d2 < 1 {
		d2 = 1
	}
	if d3 < 1 {
		d3 = 1
	}

	out := make([]float64, len(samples))
	copy(out, samples)
	for i := range out {
		wet := 0.0
		if i-d1 >= 0 {
			wet += out[i-d1] * 0.45
		}
		if i-d2 >= 0 {
			wet += out[i-d2] * 0.33
		}
		if i-d3 >= 0 {
			wet += out[i-d3] * 0.22
		}
		out[i] = out[i]*(1-mix) + (out[i]+wet)*mix
	}
	return out
}

func wavBytes(samples []int16, sampleRate int) []byte {
	const (
		numChannels   = 1
		bitsPerSample = 16
	)
	bytesPerSample := bitsPerSample / 8
	dataSize := len(samples) * bytesPerSample
	byteRate := sampleRate * numChannels * bytesPerSample
	blockAlign := numChannels * bytesPerSample

	var b bytes.Buffer
	b.WriteString("RIFF")
	_ = binary.Write(&b, binary.LittleEndian, uint32(36+dataSize))
	b.WriteString("WAVE")
	b.WriteString("fmt ")
	_ = binary.Write(&b, binary.LittleEndian, uint32(16))
	_ = binary.Write(&b, binary.LittleEndian, uint16(1))
	_ = binary.Write(&b, binary.LittleEndian, uint16(numChannels))
	_ = binary.Write(&b, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(&b, binary.LittleEndian, uint32(byteRate))
	_ = binary.Write(&b, binary.LittleEndian, uint16(blockAlign))
	_ = binary.Write(&b, binary.LittleEndian, uint16(bitsPerSample))
	b.WriteString("data")
	_ = binary.Write(&b, binary.LittleEndian, uint32(dataSize))
	for _, sample := range samples {
		_ = binary.Write(&b, binary.LittleEndian, sample)
	}
	return b.Bytes()
}

func midiToFreqFloat(midi float64) float64 {
	return 440.0 * math.Pow(2.0, (midi-69.0)/12.0)
}

func pseudoNoise(sampleIdx, midi, stringNum int) float64 {
	a := math.Sin(float64(sampleIdx*13 + midi*17 + stringNum*7))
	b := math.Sin(float64(sampleIdx*5 + midi*11 + stringNum*3))
	return 0.5 * (a + b)
}

func smoothStep(t float64) float64 {
	t = clamp(t, 0, 1)
	return t * t * (3 - 2*t)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
