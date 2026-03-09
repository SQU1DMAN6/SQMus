package compiler

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"sqmus/internal/ast"
	"sqmus/internal/parser"
)

const defaultPPQ = 480

var durationTicks = map[ast.Duration]int{
	ast.DurationWhole:        4 * defaultPPQ,
	ast.DurationHalf:         2 * defaultPPQ,
	ast.DurationQuarter:      defaultPPQ,
	ast.DurationEighth:       defaultPPQ / 2,
	ast.DurationSixteenth:    defaultPPQ / 4,
	ast.DurationThirtySecond: defaultPPQ / 8,
}

var tuningPresets = map[string][]string{
	"std":   {"E", "A", "D", "G", "B", "E"},
	"dropD": {"D", "A", "D", "G", "B", "E"},
	"Dstd":  {"D", "G", "C", "F", "A", "D"},
	"openG": {"D", "G", "D", "G", "B", "D"},
}

var semitoneByName = map[string]int{
	"C":  0,
	"C#": 1,
	"DB": 1,
	"D":  2,
	"D#": 3,
	"EB": 3,
	"E":  4,
	"F":  5,
	"F#": 6,
	"GB": 6,
	"G":  7,
	"G#": 8,
	"AB": 8,
	"A":  9,
	"A#": 10,
	"BB": 10,
	"B":  11,
}

var canonicalPitchName = map[int]string{
	0:  "C",
	1:  "C#",
	2:  "D",
	3:  "D#",
	4:  "E",
	5:  "F",
	6:  "F#",
	7:  "G",
	8:  "G#",
	9:  "A",
	10: "A#",
	11: "B",
}

// Score is the compiler IR consumed by output generators.
type Score struct {
	Name          string
	Tempo         int
	Time          ast.TimeSignature
	PPQ           int
	StringNames   [6]string // string 1..6 (high to low)
	OpenMIDINotes [6]int    // string 1..6 (high to low)
	Notes         []NoteEvent
	TotalTicks    int
}

// NoteEvent is one resolved musical note in timeline order.
type NoteEvent struct {
	StartTicks    int
	DurationTicks int
	MIDI          int
	Velocity      uint8
	String        int
	Fret          int
}

// CompileSource parses and compiles SQMus source into a score.
func CompileSource(src string) (*Score, error) {
	file, err := parser.Parse(src)
	if err != nil {
		return nil, err
	}
	return CompileAST(file)
}

// CompileAST compiles a parsed AST into a linear score IR.
func CompileAST(file *ast.File) (*Score, error) {
	if file == nil {
		return nil, fmt.Errorf("ast file is nil")
	}
	if file.Name == "" {
		return nil, fmt.Errorf("missing NAME declaration")
	}
	if len(file.Sections) == 0 {
		return nil, fmt.Errorf("at least one Section is required")
	}

	tempo := file.Tempo
	if tempo <= 0 {
		tempo = 120
	}
	timeSig := file.Time
	if timeSig.Beats <= 0 || timeSig.Division <= 0 {
		timeSig = ast.TimeSignature{Beats: 4, Division: 4}
	}

	stringNames, openMIDINotes, err := resolveInstrumentTuning(file.Instrument)
	if err != nil {
		return nil, err
	}

	score := &Score{
		Name:          file.Name,
		Tempo:         tempo,
		Time:          timeSig,
		PPQ:           defaultPPQ,
		StringNames:   stringNames,
		OpenMIDINotes: openMIDINotes,
		Notes:         make([]NoteEvent, 0, 64),
	}

	tick := 0
	for _, section := range file.Sections {
		for _, bar := range section.Bars {
			for _, event := range bar.Events {
				dur, ok := durationTicks[event.Duration]
				if !ok {
					return nil, fmt.Errorf("unsupported duration %q", event.Duration)
				}

				switch event.Kind {
				case ast.EventRest:
					// Nothing to emit.
				case ast.EventChord:
					for _, note := range event.Chord {
						noteEvent, err := emitNoteEvent(note, tick, dur, openMIDINotes)
						if err != nil {
							return nil, err
						}
						score.Notes = append(score.Notes, noteEvent)
					}
				case ast.EventNote, ast.EventTechnique:
					if event.Note == nil {
						return nil, fmt.Errorf("note event is missing note payload")
					}
					noteEvent, err := emitNoteEvent(*event.Note, tick, dur, openMIDINotes)
					if err != nil {
						return nil, err
					}
					score.Notes = append(score.Notes, noteEvent)
				default:
					return nil, fmt.Errorf("unsupported event kind %q", event.Kind)
				}

				tick += dur
			}
		}
	}

	sort.Slice(score.Notes, func(i, j int) bool {
		if score.Notes[i].StartTicks != score.Notes[j].StartTicks {
			return score.Notes[i].StartTicks < score.Notes[j].StartTicks
		}
		if score.Notes[i].String != score.Notes[j].String {
			return score.Notes[i].String < score.Notes[j].String
		}
		return score.Notes[i].MIDI < score.Notes[j].MIDI
	})

	score.TotalTicks = tick
	return score, nil
}

func emitNoteEvent(note ast.Note, startTicks, durationTicks int, openMIDINotes [6]int) (NoteEvent, error) {
	if note.String < 1 || note.String > 6 {
		return NoteEvent{}, fmt.Errorf("invalid string number %d", note.String)
	}
	if note.Fret < 0 {
		return NoteEvent{}, fmt.Errorf("invalid fret number %d", note.Fret)
	}

	open := openMIDINotes[note.String-1]
	midi := open + note.Fret
	if midi < 0 || midi > 127 {
		return NoteEvent{}, fmt.Errorf("note out of MIDI range: string=%d fret=%d midi=%d", note.String, note.Fret, midi)
	}

	return NoteEvent{
		StartTicks:    startTicks,
		DurationTicks: durationTicks,
		MIDI:          midi,
		Velocity:      96,
		String:        note.String,
		Fret:          note.Fret,
	}, nil
}

func resolveInstrumentTuning(inst *ast.Instrument) ([6]string, [6]int, error) {
	var names [6]string
	var open [6]int

	lowToHigh := tuningPresets["std"]
	if inst != nil {
		if len(inst.Tuning.Strings) == 6 {
			lowToHigh = inst.Tuning.Strings
		} else if inst.Tuning.Preset != "" {
			preset, ok := tuningPresets[inst.Tuning.Preset]
			if !ok {
				return names, open, fmt.Errorf("unknown tuning preset %q", inst.Tuning.Preset)
			}
			lowToHigh = preset
		}
	}

	defaultOctaves := [6]int{2, 2, 3, 3, 3, 4} // low to high
	for lowIdx, noteName := range lowToHigh {
		midi, err := parsePitch(noteName, defaultOctaves[lowIdx])
		if err != nil {
			return names, open, fmt.Errorf("invalid tuning note %q: %w", noteName, err)
		}

		highIdx := 5 - lowIdx
		open[highIdx] = midi
		names[highIdx] = normalizePitchLabel(noteName)
	}

	if strings.EqualFold(names[0], "E") {
		names[0] = "e"
	}
	return names, open, nil
}

func parsePitch(raw string, defaultOctave int) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("empty note")
	}

	upper := strings.ToUpper(raw)
	i := 1
	if len(upper) >= 2 && (upper[1] == '#' || upper[1] == 'B') {
		i = 2
	}
	if i > len(upper) {
		return 0, fmt.Errorf("malformed note %q", raw)
	}

	pitchName := upper[:i]
	semi, ok := semitoneByName[pitchName]
	if !ok {
		return 0, fmt.Errorf("unknown pitch class %q", pitchName)
	}

	octave := defaultOctave
	if i < len(upper) {
		n, err := strconv.Atoi(upper[i:])
		if err != nil {
			return 0, fmt.Errorf("invalid octave %q", upper[i:])
		}
		octave = n
	}

	midi := (octave+1)*12 + semi
	if midi < 0 || midi > 127 {
		return 0, fmt.Errorf("pitch %q out of MIDI range", raw)
	}
	return midi, nil
}

func normalizePitchLabel(raw string) string {
	if raw == "" {
		return "?"
	}
	midi, err := parsePitch(raw, 4)
	if err != nil {
		upper := strings.ToUpper(strings.TrimSpace(raw))
		if upper == "" {
			return "?"
		}
		return upper
	}
	return canonicalPitchName[midi%12]
}
