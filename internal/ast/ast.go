package ast

// File is the root AST node for a SQMus source file.
type File struct {
	Name       string
	Tempo      int
	Time       TimeSignature
	Instrument *Instrument
	Sections   []Section
}

// TimeSignature is the song time signature.
type TimeSignature struct {
	Beats    int
	Division int
}

// GuitarType identifies the guitar family used by an instrument block.
type GuitarType string

const (
	GuitarClassical GuitarType = "cl"
	GuitarAcoustic  GuitarType = "ac"
	GuitarElectric  GuitarType = "el"
)

// Instrument describes tuning and effects for the song guitar.
type Instrument struct {
	Type    GuitarType
	Tuning  Tuning
	Effects []Effect
}

// Tuning can be a preset or an explicit set of string notes.
type Tuning struct {
	Preset  string
	Strings []string
}

// Effect represents one effect command line inside an instrument block.
type Effect struct {
	Name   string
	Params []EffectParam
	Known  bool
}

// EffectParam is a key/value parameter for an effect.
type EffectParam struct {
	Key   string
	Value float64
}

// Section groups bars under a named musical section.
type Section struct {
	Name string
	Bars []Bar
}

// BarIDKind identifies if a bar ID is numeric or symbolic.
type BarIDKind string

const (
	BarIDNumber BarIDKind = "number"
	BarIDText   BarIDKind = "text"
)

// BarID stores a bar identifier.
type BarID struct {
	Kind   BarIDKind
	Number int
	Text   string
}

// Bar contains one bar's events.
type Bar struct {
	ID     BarID
	Events []Event
}

// Duration is the event duration symbol.
type Duration string

const (
	DurationWhole        Duration = "w"
	DurationHalf         Duration = "h"
	DurationQuarter      Duration = "q"
	DurationEighth       Duration = "e"
	DurationSixteenth    Duration = "s"
	DurationThirtySecond Duration = "t"
)

// EventKind identifies the event payload type.
type EventKind string

const (
	EventRest      EventKind = "rest"
	EventNote      EventKind = "note"
	EventChord     EventKind = "chord"
	EventTechnique EventKind = "technique"
)

// Event is one timed musical event inside a bar.
type Event struct {
	Duration  Duration
	Kind      EventKind
	Note      *Note
	Chord     []Note
	Technique *Technique
}

// Note is a guitar string/fret location.
type Note struct {
	String int
	Fret   int
}

// TechniqueKind identifies a note technique.
type TechniqueKind string

const (
	TechniqueHammer   TechniqueKind = "hammer"
	TechniquePull     TechniqueKind = "pull"
	TechniqueSlide    TechniqueKind = "slide"
	TechniqueBend     TechniqueKind = "bend"
	TechniqueVibrato  TechniqueKind = "vibrato"
	TechniqueHarmonic TechniqueKind = "harmonic"
)

// Technique augments a single note event.
type Technique struct {
	Kind       TechniqueKind
	TargetFret *int
}
