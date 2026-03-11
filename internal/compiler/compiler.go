package compiler

import (
	"fmt"
	"math"
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
	Name           string
	Tempo          int
	Time           ast.TimeSignature
	PPQ            int
	InstrumentType ast.GuitarType
	Config         GuitarConfig
	DrumConfig     DrumConfig
	StringNames    [6]string // string 1..6 (high to low)
	OpenMIDINotes  [6]int    // string 1..6 (high to low)
	Notes          []NoteEvent
	Drums          []DrumEvent
	TotalTicks     int
}

// GuitarConfig stores normalized guitar synth configuration values.
type GuitarConfig struct {
	Drive          float64
	Tone           float64
	Level          float64
	Mix            float64
	DelayTimeMS    float64
	DelayFeedback  float64
	DelayMix       float64
	ReverbRoom     float64
	ReverbMix      float64
	ChorusDepth    float64
	ChorusRate     float64
	ChorusMix      float64
	AmpGain        float64
	CabTone        float64
	PickupPosition float64
	StringDamping  float64
	PickAttack     float64
	BodyResonance  float64
	NoiseLevel     float64
}

// DrumConfig stores drum kit settings.
type DrumConfig struct {
	Kit   string
	Level float64
}

// NoteEvent is one resolved musical note in timeline order.
type NoteEvent struct {
	StartTicks          int
	DurationTicks       int
	MIDI                int
	Velocity            uint8
	String              int
	Fret                int
	Technique           ast.TechniqueKind
	TechniqueTargetMIDI int
	Augmented           bool
}

// DrumEvent is one resolved drum hit in timeline order.
type DrumEvent struct {
	StartTicks    int
	DurationTicks int
	Kind          ast.DrumKind
	Style         ast.DrumStyle
	Velocity      uint8
	Augmented     bool
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

	stringNames, openMIDINotes, guitarType, cfg, err := resolveInstrument(file.Instrument)
	if err != nil {
		return nil, err
	}
	drumCfg := resolveDrums(file.Drums)

	score := &Score{
		Name:           file.Name,
		Tempo:          tempo,
		Time:           timeSig,
		PPQ:            defaultPPQ,
		InstrumentType: guitarType,
		Config:         cfg,
		DrumConfig:     drumCfg,
		StringNames:    stringNames,
		OpenMIDINotes:  openMIDINotes,
		Notes:          make([]NoteEvent, 0, 64),
		Drums:          make([]DrumEvent, 0, 32),
	}

	tick := 0
	for _, section := range file.Sections {
		for _, bar := range section.Bars {
			for _, event := range bar.Events {
				dur, ok := durationTicks[event.Duration]
				if !ok {
					return nil, fmt.Errorf("unsupported duration %q", event.Duration)
				}
				if event.Augmented {
					dur += dur / 2
				}

				switch event.Kind {
				case ast.EventRest:
					// Nothing to emit.
				case ast.EventChord:
					for _, note := range event.Chord {
						noteEvent, err := emitNoteEvent(note, tick, dur, openMIDINotes, nil, event.Augmented)
						if err != nil {
							return nil, err
						}
						score.Notes = append(score.Notes, noteEvent)
					}
				case ast.EventNote:
					if event.Note == nil {
						return nil, fmt.Errorf("note event is missing note payload")
					}
					noteEvent, err := emitNoteEvent(*event.Note, tick, dur, openMIDINotes, nil, event.Augmented)
					if err != nil {
						return nil, err
					}
					score.Notes = append(score.Notes, noteEvent)
				case ast.EventTechnique:
					if event.Note == nil || event.Technique == nil {
						return nil, fmt.Errorf("technique event is missing payload")
					}
					noteEvent, err := emitNoteEvent(*event.Note, tick, dur, openMIDINotes, event.Technique, event.Augmented)
					if err != nil {
						return nil, err
					}
					score.Notes = append(score.Notes, noteEvent)
				case ast.EventDrum:
					if len(event.Drums) == 0 {
						return nil, fmt.Errorf("drum event is missing hits")
					}
					for _, hit := range event.Drums {
						drumEvent := DrumEvent{
							StartTicks:    tick,
							DurationTicks: dur,
							Kind:          hit.Kind,
							Style:         hit.Style,
							Velocity:      drumVelocity(hit.Style),
							Augmented:     event.Augmented,
						}
						score.Drums = append(score.Drums, drumEvent)
					}
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

	sort.Slice(score.Drums, func(i, j int) bool {
		if score.Drums[i].StartTicks != score.Drums[j].StartTicks {
			return score.Drums[i].StartTicks < score.Drums[j].StartTicks
		}
		return score.Drums[i].Kind < score.Drums[j].Kind
	})

	score.TotalTicks = tick
	return score, nil
}

func emitNoteEvent(note ast.Note, startTicks, durationTicks int, openMIDINotes [6]int, technique *ast.Technique, augmented bool) (NoteEvent, error) {
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

	velocity := uint8(96)
	techKind := ast.TechniqueKind("")
	targetMIDI := 0

	if technique != nil {
		techKind = technique.Kind
		switch technique.Kind {
		case ast.TechniqueHammer, ast.TechniquePull, ast.TechniqueSlide:
			if technique.TargetFret == nil {
				return NoteEvent{}, fmt.Errorf("technique %q requires target fret", technique.Kind)
			}
			targetMIDI = open + *technique.TargetFret
			if targetMIDI < 0 || targetMIDI > 127 {
				return NoteEvent{}, fmt.Errorf("technique target out of MIDI range: %d", targetMIDI)
			}
			velocity = 90
		case ast.TechniqueBend:
			targetMIDI = midi + 2
			if targetMIDI > 127 {
				targetMIDI = 127
			}
			velocity = 98
		case ast.TechniqueVibrato:
			targetMIDI = midi
			velocity = 95
		case ast.TechniqueHarmonic:
			midi += 12
			if midi > 127 {
				midi = 127
			}
			targetMIDI = midi
			velocity = 82
		}
	}

	return NoteEvent{
		StartTicks:          startTicks,
		DurationTicks:       durationTicks,
		MIDI:                midi,
		Velocity:            velocity,
		String:              note.String,
		Fret:                note.Fret,
		Technique:           techKind,
		TechniqueTargetMIDI: targetMIDI,
		Augmented:           augmented,
	}, nil
}

func resolveInstrument(inst *ast.Instrument) ([6]string, [6]int, ast.GuitarType, GuitarConfig, error) {
	var names [6]string
	var open [6]int

	guitarType := ast.GuitarElectric
	if inst != nil && inst.Type != "" {
		guitarType = inst.Type
	}
	cfg := defaultGuitarConfig(guitarType)

	lowToHigh := tuningPresets["std"]
	if inst != nil {
		if len(inst.Tuning.Strings) == 6 {
			lowToHigh = inst.Tuning.Strings
		} else if inst.Tuning.Preset != "" {
			preset, ok := tuningPresets[inst.Tuning.Preset]
			if !ok {
				return names, open, guitarType, cfg, fmt.Errorf("unknown tuning preset %q", inst.Tuning.Preset)
			}
			lowToHigh = preset
		}
		applyEffects(&cfg, inst.Effects)
	}

	defaultOctaves := [6]int{2, 2, 3, 3, 3, 4} // low to high
	for lowIdx, noteName := range lowToHigh {
		midi, err := parsePitch(noteName, defaultOctaves[lowIdx])
		if err != nil {
			return names, open, guitarType, cfg, fmt.Errorf("invalid tuning note %q: %w", noteName, err)
		}

		highIdx := 5 - lowIdx
		open[highIdx] = midi
		names[highIdx] = normalizePitchLabel(noteName)
	}

	if strings.EqualFold(names[0], "E") {
		names[0] = "e"
	}
	return names, open, guitarType, cfg, nil
}

func resolveDrums(drums *ast.DrumInstrument) DrumConfig {
	cfg := DrumConfig{Kit: "std", Level: 0.85}
	if drums == nil {
		return cfg
	}
	if strings.TrimSpace(drums.Kit) != "" {
		cfg.Kit = strings.TrimSpace(drums.Kit)
	}
	if drums.Level > 0 {
		cfg.Level = clamp01(drums.Level)
	}
	return cfg
}

func defaultGuitarConfig(guitarType ast.GuitarType) GuitarConfig {
	cfg := GuitarConfig{
		Drive:          0.18,
		Tone:           0.55,
		Level:          0.90,
		Mix:            1.0,
		DelayTimeMS:    0,
		DelayFeedback:  0,
		DelayMix:       0,
		ReverbRoom:     0.15,
		ReverbMix:      0.10,
		ChorusDepth:    0,
		ChorusRate:     0,
		ChorusMix:      0,
		AmpGain:        0.65,
		CabTone:        0.55,
		PickupPosition: 0.62,
		StringDamping:  0.30,
		PickAttack:     0.45,
		BodyResonance:  0.35,
		NoiseLevel:     0.012,
	}

	switch guitarType {
	case ast.GuitarAcoustic:
		cfg.Drive = 0.05
		cfg.Tone = 0.62
		cfg.AmpGain = 0.35
		cfg.PickupPosition = 0.48
		cfg.StringDamping = 0.24
		cfg.PickAttack = 0.52
		cfg.BodyResonance = 0.70
		cfg.NoiseLevel = 0.020
	case ast.GuitarClassical:
		cfg.Drive = 0.0
		cfg.Tone = 0.58
		cfg.AmpGain = 0.28
		cfg.PickupPosition = 0.42
		cfg.StringDamping = 0.20
		cfg.PickAttack = 0.32
		cfg.BodyResonance = 0.78
		cfg.NoiseLevel = 0.018
	}
	return cfg
}

func applyEffects(cfg *GuitarConfig, effects []ast.Effect) {
	for _, effect := range effects {
		params := map[string]float64{}
		for _, param := range effect.Params {
			params[param.Key] = param.Value
		}

		switch effect.Name {
		case "drv":
			if v, ok := params["g"]; ok {
				cfg.Drive = clamp01(v)
			}
			if v, ok := params["t"]; ok {
				cfg.Tone = clamp01(v)
			}
			if v, ok := params["l"]; ok {
				cfg.Level = clamp01(v)
			}
			if v, ok := params["m"]; ok {
				cfg.Mix = clamp01(v)
			}
		case "dly":
			if v, ok := params["t"]; ok {
				cfg.DelayTimeMS = clamp(v, 0, 2000)
			}
			if v, ok := params["f"]; ok {
				cfg.DelayFeedback = clamp01(v)
			}
			if v, ok := params["m"]; ok {
				cfg.DelayMix = clamp01(v)
			}
		case "rev":
			if v, ok := params["r"]; ok {
				cfg.ReverbRoom = clamp01(v)
			}
			if v, ok := params["m"]; ok {
				cfg.ReverbMix = clamp01(v)
			}
		case "cho":
			if v, ok := params["d"]; ok {
				cfg.ChorusDepth = clamp01(v)
			}
			if v, ok := params["r"]; ok {
				cfg.ChorusRate = clamp(v, 0, 8)
			}
			if v, ok := params["m"]; ok {
				cfg.ChorusMix = clamp01(v)
			}
		case "amp":
			if v, ok := params["g"]; ok {
				cfg.AmpGain = clamp01(v)
			}
			if v, ok := params["t"]; ok {
				cfg.CabTone = clamp01(v)
			}
			if v, ok := params["l"]; ok {
				cfg.Level = clamp01(v)
			}
		case "cab":
			if v, ok := params["t"]; ok {
				cfg.CabTone = clamp01(v)
			}
		case "pick":
			if v, ok := params["p"]; ok {
				cfg.PickupPosition = clamp01(v)
			}
			if v, ok := params["a"]; ok {
				cfg.PickAttack = clamp01(v)
			}
		case "str":
			if v, ok := params["d"]; ok {
				cfg.StringDamping = clamp01(v)
			}
		case "body":
			if v, ok := params["r"]; ok {
				cfg.BodyResonance = clamp01(v)
			}
		case "noi":
			if v, ok := params["l"]; ok {
				cfg.NoiseLevel = clamp01(v)
			}
		}
	}
}

func drumVelocity(style ast.DrumStyle) uint8 {
	switch style {
	case ast.DrumStyleGhost:
		return 60
	case ast.DrumStyleRim:
		return 92
	case ast.DrumStyleFlam:
		return 105
	case ast.DrumStyleAccent:
		return 118
	default:
		return 96
	}
}

func clamp01(v float64) float64 {
	if math.IsNaN(v) {
		return 0
	}
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func clamp(v, lo, hi float64) float64 {
	if math.IsNaN(v) {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
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
