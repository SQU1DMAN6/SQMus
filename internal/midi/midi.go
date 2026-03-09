package midi

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"sort"

	"sqmus/internal/compiler"
)

const (
	statusNoteOff = 0x80
	statusNoteOn  = 0x90
)

type timedEvent struct {
	tick     int
	priority int
	payload  []byte
}

// Encode converts a compiled score into a Standard MIDI File (format 0).
func Encode(score *compiler.Score) ([]byte, error) {
	if score == nil {
		return nil, fmt.Errorf("score is nil")
	}
	if score.PPQ <= 0 {
		return nil, fmt.Errorf("invalid PPQ: %d", score.PPQ)
	}
	if score.Tempo <= 0 {
		return nil, fmt.Errorf("invalid tempo: %d", score.Tempo)
	}

	events := make([]timedEvent, 0, len(score.Notes)*2+2)
	events = append(events, tempoEvent(score.Tempo))
	events = append(events, timeSignatureEvent(score.Time.Beats, score.Time.Division))

	for _, note := range score.Notes {
		if note.MIDI < 0 || note.MIDI > 127 {
			return nil, fmt.Errorf("note MIDI out of range: %d", note.MIDI)
		}
		if note.DurationTicks <= 0 {
			continue
		}
		events = append(events,
			timedEvent{
				tick:     note.StartTicks,
				priority: 2,
				payload:  []byte{statusNoteOn, byte(note.MIDI), noteVelocity(note.Velocity)},
			},
			timedEvent{
				tick:     note.StartTicks + note.DurationTicks,
				priority: 1,
				payload:  []byte{statusNoteOff, byte(note.MIDI), 0},
			},
		)
	}

	sort.Slice(events, func(i, j int) bool {
		if events[i].tick != events[j].tick {
			return events[i].tick < events[j].tick
		}
		return events[i].priority < events[j].priority
	})

	var track bytes.Buffer
	prevTick := 0
	for _, event := range events {
		if event.tick < prevTick {
			return nil, fmt.Errorf("event ordering failure")
		}
		delta := event.tick - prevTick
		track.Write(encodeVarLen(delta))
		track.Write(event.payload)
		prevTick = event.tick
	}

	track.Write([]byte{0x00, 0xFF, 0x2F, 0x00})

	var out bytes.Buffer
	out.WriteString("MThd")
	if err := binary.Write(&out, binary.BigEndian, uint32(6)); err != nil {
		return nil, err
	}
	if err := binary.Write(&out, binary.BigEndian, uint16(0)); err != nil {
		return nil, err
	}
	if err := binary.Write(&out, binary.BigEndian, uint16(1)); err != nil {
		return nil, err
	}
	if err := binary.Write(&out, binary.BigEndian, uint16(score.PPQ)); err != nil {
		return nil, err
	}

	out.WriteString("MTrk")
	if err := binary.Write(&out, binary.BigEndian, uint32(track.Len())); err != nil {
		return nil, err
	}
	out.Write(track.Bytes())

	return out.Bytes(), nil
}

// WriteFile writes MIDI bytes to disk.
func WriteFile(score *compiler.Score, path string) error {
	data, err := Encode(score)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func tempoEvent(bpm int) timedEvent {
	microsPerQuarter := 60000000 / bpm
	payload := []byte{0xFF, 0x51, 0x03, byte(microsPerQuarter >> 16), byte(microsPerQuarter >> 8), byte(microsPerQuarter)}
	return timedEvent{tick: 0, priority: 0, payload: payload}
}

func timeSignatureEvent(beats, division int) timedEvent {
	if beats <= 0 {
		beats = 4
	}
	exponent := denominatorExponent(division)
	payload := []byte{0xFF, 0x58, 0x04, byte(beats), byte(exponent), 24, 8}
	return timedEvent{tick: 0, priority: 0, payload: payload}
}

func denominatorExponent(division int) int {
	if division <= 0 {
		return 2
	}
	exp := 0
	value := 1
	for value < division && exp < 8 {
		value <<= 1
		exp++
	}
	if value == division {
		return exp
	}
	return 2
}

func noteVelocity(v uint8) byte {
	if v == 0 {
		return 96
	}
	return v
}

func encodeVarLen(value int) []byte {
	if value < 0 {
		value = 0
	}
	buf := []byte{byte(value & 0x7F)}
	for value >>= 7; value > 0; value >>= 7 {
		buf = append([]byte{byte((value & 0x7F) | 0x80)}, buf...)
	}
	return buf
}
