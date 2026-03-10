package midi

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"sort"

	"sqmus/internal/ast"
	"sqmus/internal/compiler"
)

const (
	statusNoteOff = 0x80
	statusNoteOn  = 0x90
	statusCC      = 0xB0
	statusPitch   = 0xE0

	pitchRangeSemitones = 2.0
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

	events := make([]timedEvent, 0, len(score.Notes)*8+12)
	events = append(events, tempoEvent(score.Tempo))
	events = append(events, timeSignatureEvent(score.Time.Beats, score.Time.Division))
	events = append(events, pitchBendRangeEvents()...)

	for _, note := range score.Notes {
		if note.MIDI < 0 || note.MIDI > 127 {
			return nil, fmt.Errorf("note MIDI out of range: %d", note.MIDI)
		}
		if note.DurationTicks <= 0 {
			continue
		}

		start := note.StartTicks
		end := note.StartTicks + note.DurationTicks
		events = append(events,
			pitchBendEvent(start, 0, 2),
			timedEvent{tick: start, priority: 5, payload: []byte{statusNoteOn, byte(note.MIDI), noteVelocity(note.Velocity)}},
			timedEvent{tick: end, priority: 3, payload: []byte{statusNoteOff, byte(note.MIDI), 0}},
			pitchBendEvent(end, 0, 4),
		)

		events = append(events, techniquePitchEvents(note)...)
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

func techniquePitchEvents(note compiler.NoteEvent) []timedEvent {
	start := note.StartTicks
	end := note.StartTicks + note.DurationTicks
	if end <= start {
		return nil
	}

	base := float64(note.MIDI)
	target := float64(note.TechniqueTargetMIDI)
	if target == 0 {
		target = base
	}

	events := make([]timedEvent, 0)
	switch note.Technique {
	case ast.TechniqueHammer, ast.TechniquePull:
		if target == base {
			return nil
		}
		events = append(events, linearBendEvents(start, end, base, target, 0.30)...)
	case ast.TechniqueSlide:
		if target == base {
			return nil
		}
		events = append(events, linearBendEvents(start, end, base, target, 1.0)...)
	case ast.TechniqueBend:
		if target == base {
			target = base + 2
		}
		events = append(events, linearBendEvents(start, end, base, target, 0.70)...)
	case ast.TechniqueVibrato:
		steps := maxInt(8, note.DurationTicks/60)
		for i := 0; i <= steps; i++ {
			progress := float64(i) / float64(steps)
			tick := start + int(float64(note.DurationTicks)*progress)
			delta := math.Sin(progress*2*math.Pi*4.0) * 0.25
			events = append(events, pitchBendEvent(tick, delta, 2))
		}
	}

	if len(events) > 0 {
		events = append(events, pitchBendEvent(end, 0, 4))
	}
	return events
}

func linearBendEvents(start, end int, fromMIDI, toMIDI, transitionPortion float64) []timedEvent {
	dur := end - start
	if dur <= 0 {
		return nil
	}
	if transitionPortion <= 0 {
		transitionPortion = 1.0
	}
	if transitionPortion > 1 {
		transitionPortion = 1
	}

	transitionTicks := int(float64(dur) * transitionPortion)
	if transitionTicks < 1 {
		transitionTicks = 1
	}
	steps := maxInt(6, transitionTicks/60)
	events := make([]timedEvent, 0, steps+2)
	for i := 0; i <= steps; i++ {
		progress := float64(i) / float64(steps)
		tick := start + int(float64(transitionTicks)*progress)
		value := fromMIDI + (toMIDI-fromMIDI)*smoothStep(progress)
		delta := value - fromMIDI
		events = append(events, pitchBendEvent(tick, delta, 2))
	}

	if transitionTicks < dur {
		events = append(events, pitchBendEvent(start+transitionTicks, toMIDI-fromMIDI, 2))
	}
	return events
}

func pitchBendRangeEvents() []timedEvent {
	// Set pitch bend range to +/-2 semitones via RPN 0,0.
	return []timedEvent{
		ccEvent(0, 101, 0, 1),
		ccEvent(0, 100, 0, 1),
		ccEvent(0, 6, 2, 1),
		ccEvent(0, 38, 0, 1),
		ccEvent(0, 101, 127, 1),
		ccEvent(0, 100, 127, 1),
	}
}

func ccEvent(tick int, controller int, value int, priority int) timedEvent {
	return timedEvent{tick: tick, priority: priority, payload: []byte{statusCC, byte(controller & 0x7F), byte(value & 0x7F)}}
}

func pitchBendEvent(tick int, semitoneDelta float64, priority int) timedEvent {
	ratio := semitoneDelta / pitchRangeSemitones
	if ratio > 1 {
		ratio = 1
	}
	if ratio < -1 {
		ratio = -1
	}
	value := 8192 + int(ratio*8192)
	if value < 0 {
		value = 0
	}
	if value > 16383 {
		value = 16383
	}
	lsb := byte(value & 0x7F)
	msb := byte((value >> 7) & 0x7F)
	return timedEvent{tick: tick, priority: priority, payload: []byte{statusPitch, lsb, msb}}
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

func smoothStep(t float64) float64 {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return t * t * (3 - 2*t)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
