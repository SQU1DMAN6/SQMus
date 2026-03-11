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

// Play renders and plays a score using a system audio player if available.
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

	guitar := renderGuitar(score, sampleRate, totalSamples, secondsPerTick)
	guitar = applyBodyResonance(guitar, score.Config, sampleRate)
	guitar = applyAmpSim(guitar, score.Config, sampleRate)
	guitar = applyChorus(guitar, score.Config, sampleRate)
	guitar = applyDelay(guitar, score.Config, sampleRate)
	guitar = applyReverb(guitar, score.Config, sampleRate)

	drums := renderDrums(score, sampleRate, totalSamples, secondsPerTick)
	mix := mixSamples(guitar, drums, score.DrumConfig.Level)

	pcm := make([]int16, totalSamples)
	for i, sample := range mix {
		if sample > 1.0 {
			sample = 1.0
		} else if sample < -1.0 {
			sample = -1.0
		}
		pcm[i] = int16(sample * maxInt16)
	}

	return wavBytes(pcm, sampleRate), nil
}

func renderGuitar(score *compiler.Score, sampleRate, totalSamples int, secondsPerTick float64) []float64 {
	samples := make([]float64, totalSamples)
	cfg := score.Config

	attackSamples := int((0.0015 + (1.0-clamp01(cfg.PickAttack))*0.006) * float64(sampleRate))
	releaseSamples := int((0.010 + clamp01(cfg.StringDamping)*0.018) * float64(sampleRate))
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

		baseGain := (0.25 + float64(note.Velocity)/255.0*0.52) * clamp01(cfg.Level)
		if baseGain <= 0 {
			baseGain = 0.1
		}

		renderStringNote(samples, note, cfg, sampleRate, startSample, endSample, durationSamples, attackSamples, releaseSamples, baseGain)
	}

	return samples
}

func renderStringNote(samples []float64, note compiler.NoteEvent, cfg compiler.GuitarConfig, sampleRate, startSample, endSample, durationSamples, attackSamples, releaseSamples int, baseGain float64) {
	baseFreq := midiToFreqFloat(float64(note.MIDI))
	targetFreq := baseFreq
	if note.TechniqueTargetMIDI > 0 {
		targetFreq = midiToFreqFloat(float64(note.TechniqueTargetMIDI))
	}
	minFreq := math.Min(baseFreq, targetFreq)
	if note.Technique == ast.TechniqueBend || note.Technique == ast.TechniqueVibrato {
		minFreq = baseFreq
	}
	if minFreq < 20 {
		minFreq = 20
	}

	inharm := stringInharmonicity(note.String)
	minFreq *= inharm

	bufferLen := int(float64(sampleRate)/minFreq) + 4
	if bufferLen < 32 {
		bufferLen = 32
	}
	if bufferLen > sampleRate {
		bufferLen = sampleRate
	}

	buf := make([]float64, bufferLen)
	pickPos := clamp(cfg.PickupPosition, 0.05, 0.95)
	pickAttack := clamp01(cfg.PickAttack)
	noiseLevel := clamp01(cfg.NoiseLevel)
	pluckAmp := 0.20 + pickAttack*0.45 + noiseLevel*0.6
	seed := uint32(note.MIDI*701 + note.String*131 + bufferLen*17)
	fillNoise(buf, seed)
	smoothNoise(buf, 0.45+pickAttack*0.1)
	shapeExcitation(buf)
	for i := range buf {
		buf[i] *= pluckAmp
	}
	delay := int(pickPos * float64(bufferLen-1))
	if delay < 1 {
		delay = 1
	}
	for i := delay; i < bufferLen; i++ {
		buf[i] -= buf[i-delay] * 0.25
	}
	if note.Technique == ast.TechniqueHarmonic {
		for i := range buf {
			buf[i] *= math.Sin(math.Pi * float64(i) / float64(bufferLen))
		}
	}

	writeIndex := 0
	prevFiltered := 0.0
	lp := 0.16 + (1.0-clamp01(cfg.Tone))*0.40 + clamp01(cfg.StringDamping)*0.18
	if lp < 0.03 {
		lp = 0.03
	}
	if lp > 0.80 {
		lp = 0.80
	}
	damping := 0.995 - clamp01(cfg.StringDamping)*0.008
	damping -= float64(6-note.String) * 0.0006
	if damping < 0.955 {
		damping = 0.955
	}

	pickupDelay := int(float64(bufferLen) * (0.12 + pickPos*0.28))
	if pickupDelay < 1 {
		pickupDelay = 1
	}
	pickupBuf := make([]float64, pickupDelay+1)

	burstSamples := int((0.012 + pickAttack*0.016) * float64(sampleRate))
	if burstSamples < 1 {
		burstSamples = 1
	}
	noiseHP := 0.0

	for i := startSample; i < endSample; i++ {
		local := i - startSample
		progress := float64(local) / float64(durationSamples)
		remaining := endSample - i

		env := math.Exp(-progress * (1.0 + clamp01(cfg.StringDamping)*2.2))
		if local < attackSamples {
			env *= float64(local) / float64(attackSamples)
		}
		if remaining < releaseSamples {
			env *= float64(remaining) / float64(releaseSamples)
		}
		if env < 0 {
			env = 0
		}

		freq := noteFrequency(note, progress, cfg) * inharm
		if freq < 20 {
			freq = 20
		}
		delayLen := float64(sampleRate) / freq
		if delayLen > float64(bufferLen-2) {
			delayLen = float64(bufferLen - 2)
		}
		readIndex := float64(writeIndex) - delayLen
		for readIndex < 0 {
			readIndex += float64(bufferLen)
		}
		i0 := int(readIndex)
		i1 := (i0 + 1) % bufferLen
		frac := readIndex - float64(i0)
		sample := buf[i0]*(1-frac) + buf[i1]*frac

		filtered := sample*(1-lp) + prevFiltered*lp
		prevFiltered = filtered
		filtered *= damping

		buf[writeIndex] = filtered
		writeIndex = (writeIndex + 1) % bufferLen

		pickupIndex := local % len(pickupBuf)
		pickupDelayed := pickupBuf[pickupIndex]
		pickupBuf[pickupIndex] = filtered
		out := filtered - pickupDelayed*0.18

		if local < burstSamples {
			decay := 1 - float64(local)/float64(burstSamples)
			burst := decay * (0.012 + pickAttack*0.10 + noiseLevel*0.06)
			n := pseudoNoise(i, note.MIDI, note.String)
			noiseHP = n - noiseHP*0.95
			out += noiseHP * burst
		}

		samples[i] += out * env * baseGain
	}
}

func renderDrums(score *compiler.Score, sampleRate, totalSamples int, secondsPerTick float64) []float64 {
	if len(score.Drums) == 0 {
		return make([]float64, totalSamples)
	}

	samples := make([]float64, totalSamples)
	for _, hit := range score.Drums {
		startSample := int(float64(hit.StartTicks) * secondsPerTick * float64(sampleRate))
		durationSamples := int(float64(hit.DurationTicks) * secondsPerTick * float64(sampleRate))
		if durationSamples < int(0.03*float64(sampleRate)) {
			durationSamples = int(0.03 * float64(sampleRate))
		}
		if hit.Kind == ast.DrumHiHat && hit.Style == ast.DrumStyleOpen {
			durationSamples = int(0.35 * float64(sampleRate))
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

		velocity := float64(hit.Velocity) / 127.0
		amp := velocity * 0.8
		switch hit.Kind {
		case ast.DrumKick:
			renderKick(samples, startSample, endSample, sampleRate, amp)
		case ast.DrumSnare:
			renderSnare(samples, startSample, endSample, sampleRate, amp, hit.Style)
		case ast.DrumHiHat:
			renderHiHat(samples, startSample, endSample, sampleRate, amp, hit.Style)
		case ast.DrumRide:
			renderRide(samples, startSample, endSample, sampleRate, amp)
		case ast.DrumCrash:
			renderCrash(samples, startSample, endSample, sampleRate, amp)
		case ast.DrumTom1:
			renderTom(samples, startSample, endSample, sampleRate, amp, 180)
		case ast.DrumTom2:
			renderTom(samples, startSample, endSample, sampleRate, amp, 140)
		case ast.DrumTom3:
			renderTom(samples, startSample, endSample, sampleRate, amp, 110)
		case ast.DrumClap:
			renderClap(samples, startSample, endSample, sampleRate, amp)
		case ast.DrumCowbell:
			renderCowbell(samples, startSample, endSample, sampleRate, amp)
		case ast.DrumPerc:
			renderPerc(samples, startSample, endSample, sampleRate, amp)
		}
	}

	return samples
}

func renderKick(samples []float64, start, end, sampleRate int, amp float64) {
	length := end - start
	for i := 0; i < length; i++ {
		t := float64(i) / float64(sampleRate)
		freq := 90.0 - 50.0*t*4
		if freq < 40 {
			freq = 40
		}
		env := math.Exp(-t * 16)
		samples[start+i] += math.Sin(2*math.Pi*freq*t) * env * amp
	}
}

func renderSnare(samples []float64, start, end, sampleRate int, amp float64, style ast.DrumStyle) {
	length := end - start
	toneFreq := 190.0
	noiseGain := 0.65
	if style == ast.DrumStyleRim {
		noiseGain = 0.35
		toneFreq = 260.0
	}
	for i := 0; i < length; i++ {
		t := float64(i) / float64(sampleRate)
		env := math.Exp(-t * 20)
		noise := pseudoNoise(start+i, 38, 2) * noiseGain
		tone := math.Sin(2*math.Pi*toneFreq*t) * 0.35
		samples[start+i] += (noise + tone) * env * amp
	}
}

func renderHiHat(samples []float64, start, end, sampleRate int, amp float64, style ast.DrumStyle) {
	length := end - start
	decay := 38.0
	if style == ast.DrumStyleOpen {
		decay = 8.5
	}
	for i := 0; i < length; i++ {
		t := float64(i) / float64(sampleRate)
		env := math.Exp(-t * decay)
		noise := pseudoNoise(start+i, 42, 5)
		noise = noise - 0.5*pseudoNoise(start+i+1, 42, 5)
		samples[start+i] += noise * env * amp * 0.6
	}
}

func renderRide(samples []float64, start, end, sampleRate int, amp float64) {
	renderCrash(samples, start, end, sampleRate, amp*0.6)
}

func renderCrash(samples []float64, start, end, sampleRate int, amp float64) {
	length := end - start
	for i := 0; i < length; i++ {
		t := float64(i) / float64(sampleRate)
		env := math.Exp(-t * 4.2)
		noise := pseudoNoise(start+i, 49, 6)
		samples[start+i] += noise * env * amp * 0.7
	}
}

func renderTom(samples []float64, start, end, sampleRate int, amp float64, freq float64) {
	length := end - start
	for i := 0; i < length; i++ {
		t := float64(i) / float64(sampleRate)
		env := math.Exp(-t * 10)
		samples[start+i] += math.Sin(2*math.Pi*freq*t) * env * amp
	}
}

func renderClap(samples []float64, start, end, sampleRate int, amp float64) {
	length := end - start
	for i := 0; i < length; i++ {
		t := float64(i) / float64(sampleRate)
		env := math.Exp(-t * 30)
		noise := pseudoNoise(start+i, 39, 3)
		samples[start+i] += noise * env * amp * 0.5
	}
}

func renderCowbell(samples []float64, start, end, sampleRate int, amp float64) {
	length := end - start
	for i := 0; i < length; i++ {
		t := float64(i) / float64(sampleRate)
		env := math.Exp(-t * 14)
		samples[start+i] += math.Sin(2*math.Pi*560*t) * env * amp * 0.5
	}
}

func renderPerc(samples []float64, start, end, sampleRate int, amp float64) {
	length := end - start
	for i := 0; i < length; i++ {
		t := float64(i) / float64(sampleRate)
		env := math.Exp(-t * 18)
		noise := pseudoNoise(start+i, 60, 4)
		samples[start+i] += noise * env * amp * 0.4
	}
}

func mixSamples(guitar, drums []float64, drumLevel float64) []float64 {
	if len(guitar) == 0 {
		return drums
	}
	if len(drums) == 0 {
		return guitar
	}
	out := make([]float64, len(guitar))
	level := clamp01(drumLevel)
	for i := range guitar {
		out[i] = guitar[i] + drums[i]*level
	}
	return out
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
		depth := 0.12 + clamp01(cfg.Tone)*0.18
		rate := 5.2 + clamp01(cfg.PickAttack)*2.4
		mod := math.Sin(2*math.Pi*rate*progress) * depth
		return midiToFreqFloat(base + mod)
	}

	return midiToFreqFloat(base)
}

func applyBodyResonance(samples []float64, cfg compiler.GuitarConfig, sampleRate int) []float64 {
	if len(samples) == 0 {
		return samples
	}
	mix := clamp01(cfg.BodyResonance)
	if mix <= 0.001 {
		return samples
	}

	f1 := 95.0
	f2 := 190.0
	f3 := 285.0
	if cfg.AmpGain > 0.55 {
		f1 = 120.0
		f2 = 240.0
		f3 = 360.0
	}

	b1 := newBiquadBandpass(f1, 0.6, sampleRate)
	b2 := newBiquadBandpass(f2, 0.55, sampleRate)
	b3 := newBiquadBandpass(f3, 0.5, sampleRate)

	out := make([]float64, len(samples))
	for i, x := range samples {
		res := (b1.process(x) + b2.process(x) + b3.process(x)) * 0.6
		out[i] = x + res*mix*0.28
	}
	return out
}

func applyAmpSim(samples []float64, cfg compiler.GuitarConfig, sampleRate int) []float64 {
	if len(samples) == 0 {
		return samples
	}

	drive := clamp01(cfg.Drive)
	ampGain := clamp01(cfg.AmpGain)
	tone := clamp01(cfg.Tone)
	cab := clamp01(cfg.CabTone)
	mix := clamp01(cfg.Mix)

	preGain := 1.0 + ampGain*4.6 + drive*5.6
	preGain *= 0.8 + tone*0.4

	highpass := newBiquadHighpass(80.0+cab*30.0, 0.7, sampleRate)
	bright := newBiquadHighpass(1300.0+tone*900.0, 0.7, sampleRate)
	presence := newBiquadBandpass(2600.0+tone*1000.0, 1.2, sampleRate)
	lowpass := newBiquadLowpass(2800.0+cab*3600.0, 0.7, sampleRate)

	brightMix := 0.03 + tone*0.12
	presenceMix := 0.02 + tone*0.08

	out := make([]float64, len(samples))
	for i, x := range samples {
		dry := x
		x = highpass.process(x)
		x += bright.process(x) * brightMix

		pre := x * preGain
		sat := math.Tanh(pre)
		sat += presence.process(sat) * presenceMix

		y := sat*mix + dry*(1-mix)
		out[i] = lowpass.process(y)
	}
	return out
}

type biquad struct {
	b0 float64
	b1 float64
	b2 float64
	a1 float64
	a2 float64
	z1 float64
	z2 float64
}

func (b *biquad) process(x float64) float64 {
	y := b.b0*x + b.z1
	b.z1 = b.b1*x - b.a1*y + b.z2
	b.z2 = b.b2*x - b.a2*y
	return y
}

func newBiquadLowpass(freq, q float64, sampleRate int) biquad {
	w0 := 2 * math.Pi * freq / float64(sampleRate)
	cosw0 := math.Cos(w0)
	sinw0 := math.Sin(w0)
	alpha := sinw0 / (2 * q)

	b0 := (1 - cosw0) / 2
	b1 := 1 - cosw0
	b2 := (1 - cosw0) / 2
	a0 := 1 + alpha
	a1 := -2 * cosw0
	a2 := 1 - alpha

	return biquad{
		b0: b0 / a0,
		b1: b1 / a0,
		b2: b2 / a0,
		a1: a1 / a0,
		a2: a2 / a0,
	}
}

func newBiquadHighpass(freq, q float64, sampleRate int) biquad {
	w0 := 2 * math.Pi * freq / float64(sampleRate)
	cosw0 := math.Cos(w0)
	sinw0 := math.Sin(w0)
	alpha := sinw0 / (2 * q)

	b0 := (1 + cosw0) / 2
	b1 := -(1 + cosw0)
	b2 := (1 + cosw0) / 2
	a0 := 1 + alpha
	a1 := -2 * cosw0
	a2 := 1 - alpha

	return biquad{
		b0: b0 / a0,
		b1: b1 / a0,
		b2: b2 / a0,
		a1: a1 / a0,
		a2: a2 / a0,
	}
}

func newBiquadBandpass(freq, q float64, sampleRate int) biquad {
	w0 := 2 * math.Pi * freq / float64(sampleRate)
	cosw0 := math.Cos(w0)
	sinw0 := math.Sin(w0)
	alpha := sinw0 / (2 * q)

	b0 := alpha
	b1 := 0.0
	b2 := -alpha
	a0 := 1 + alpha
	a1 := -2 * cosw0
	a2 := 1 - alpha

	return biquad{
		b0: b0 / a0,
		b1: b1 / a0,
		b2: b2 / a0,
		a1: a1 / a0,
		a2: a2 / a0,
	}
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
	x := uint32(sampleIdx*1664525 + midi*1013904223 + stringNum*374761393)
	x ^= x >> 13
	x *= 1274126177
	x ^= x >> 16
	return float64(int32(x)) * (1.0 / 2147483647.0)
}

func stringInharmonicity(stringNum int) float64 {
	if stringNum < 1 || stringNum > 6 {
		return 1.0
	}
	return 1.0 + float64(6-stringNum)*0.0006
}

func fillNoise(buf []float64, seed uint32) {
	s := seed
	for i := range buf {
		s = s*1664525 + 1013904223
		buf[i] = float64(int32(s)) * (1.0 / 2147483647.0)
	}
}

func smoothNoise(buf []float64, alpha float64) {
	if alpha < 0.05 {
		alpha = 0.05
	}
	if alpha > 0.9 {
		alpha = 0.9
	}
	prev := 0.0
	for i := range buf {
		prev += (buf[i] - prev) * alpha
		buf[i] = prev
	}
}

func shapeExcitation(buf []float64) {
	n := len(buf)
	if n == 0 {
		return
	}
	for i := range buf {
		t := float64(i) / float64(n-1)
		shape := 1.0 - math.Abs(t*2-1)
		buf[i] *= 0.6 + shape*0.8
	}
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
