package audio

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"os/exec"

	"sqmus/internal/compiler"
)

const (
	defaultSampleRate = 44100
	defaultGain       = 0.18
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
	totalSeconds := float64(score.TotalTicks)*secondsPerTick + 0.6 // keep short release tail
	if totalSeconds < 1.0 {
		totalSeconds = 1.0
	}
	totalSamples := int(totalSeconds * float64(sampleRate))
	if totalSamples <= 0 {
		totalSamples = sampleRate
	}

	samples := make([]float64, totalSamples)
	attackSamples := int(0.005 * float64(sampleRate))
	releaseSamples := int(0.008 * float64(sampleRate))
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

		freq := midiToFreq(note.MIDI)
		for i := startSample; i < endSample; i++ {
			local := i - startSample
			env := 1.0
			if local < attackSamples {
				env = float64(local) / float64(attackSamples)
			}
			remaining := endSample - i
			if remaining < releaseSamples {
				rel := float64(remaining) / float64(releaseSamples)
				if rel < env {
					env = rel
				}
			}
			t := float64(local) / float64(sampleRate)
			samples[i] += math.Sin(2*math.Pi*freq*t) * env * defaultGain
		}
	}

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

func midiToFreq(midi int) float64 {
	return 440.0 * math.Pow(2.0, float64(midi-69)/12.0)
}
