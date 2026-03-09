package parser

import (
	"strings"
	"testing"

	"sqmus/internal/ast"
)

func TestParseNearFullV01(t *testing.T) {
	src := `NAME Technique Suite

tp 120
time 4/4

el {
    tn E A D G B E
    drv g 0.6 t 0.5 l 0.9
    fx a 1 b 2
}

Section Main

b 1 {
    q: s2,5 hammer 7
    q: s2,7 pull 5
    q: s2,5 slide 7
    q: s1,7 bend
}

b Outro {
    h: [s1,0 s2,0 s3,1 s4,2]
    q: s1,8 vibrato
    q: s2,7 harmonic
    q: n
}
`

	file, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() returned error: %v", err)
	}

	if file.Name != "Technique Suite" {
		t.Fatalf("unexpected name: %q", file.Name)
	}
	if file.Tempo != 120 {
		t.Fatalf("unexpected tempo: %d", file.Tempo)
	}
	if file.Time.Beats != 4 || file.Time.Division != 4 {
		t.Fatalf("unexpected time signature: %+v", file.Time)
	}
	if file.Instrument == nil {
		t.Fatal("instrument should be parsed")
	}
	if file.Instrument.Type != ast.GuitarElectric {
		t.Fatalf("unexpected instrument type: %s", file.Instrument.Type)
	}
	if len(file.Instrument.Tuning.Strings) != 6 {
		t.Fatalf("expected explicit 6-string tuning, got: %+v", file.Instrument.Tuning)
	}
	if len(file.Instrument.Effects) != 2 {
		t.Fatalf("expected 2 effects, got %d", len(file.Instrument.Effects))
	}
	if !file.Instrument.Effects[0].Known {
		t.Fatalf("expected drv to be marked known")
	}
	if file.Instrument.Effects[1].Known {
		t.Fatalf("expected unknown effect to be marked unknown")
	}
	if len(file.Sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(file.Sections))
	}
	if len(file.Sections[0].Bars) != 2 {
		t.Fatalf("expected 2 bars, got %d", len(file.Sections[0].Bars))
	}

	bar1 := file.Sections[0].Bars[0]
	if bar1.ID.Kind != ast.BarIDNumber || bar1.ID.Number != 1 {
		t.Fatalf("unexpected first bar id: %+v", bar1.ID)
	}
	if len(bar1.Events) != 4 {
		t.Fatalf("expected 4 events in bar 1, got %d", len(bar1.Events))
	}
	if bar1.Events[0].Kind != ast.EventTechnique || bar1.Events[0].Technique == nil ||
		bar1.Events[0].Technique.Kind != ast.TechniqueHammer ||
		bar1.Events[0].Technique.TargetFret == nil || *bar1.Events[0].Technique.TargetFret != 7 {
		t.Fatalf("unexpected hammer event: %+v", bar1.Events[0])
	}
	if bar1.Events[3].Technique == nil || bar1.Events[3].Technique.Kind != ast.TechniqueBend || bar1.Events[3].Technique.TargetFret != nil {
		t.Fatalf("unexpected bend event: %+v", bar1.Events[3])
	}

	bar2 := file.Sections[0].Bars[1]
	if bar2.ID.Kind != ast.BarIDText || bar2.ID.Text != "Outro" {
		t.Fatalf("unexpected second bar id: %+v", bar2.ID)
	}
	if bar2.Events[0].Kind != ast.EventChord || len(bar2.Events[0].Chord) != 4 {
		t.Fatalf("unexpected chord event: %+v", bar2.Events[0])
	}
	if bar2.Events[3].Kind != ast.EventRest {
		t.Fatalf("expected rest event, got %+v", bar2.Events[3])
	}
}

func TestParseSupportsStandardTuningPreset(t *testing.T) {
	src := `NAME Preset

tp 92
time 4/4

ac {
    tn std
}

Section Main
b Intro {
    q: s2,0
}
`

	file, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() returned error: %v", err)
	}
	if file.Instrument == nil {
		t.Fatal("expected instrument")
	}
	if file.Instrument.Tuning.Preset != "std" {
		t.Fatalf("expected std preset, got %+v", file.Instrument.Tuning)
	}
}

func TestParseErrors(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "malformed time",
			src: `NAME A

tp 92
time 4/

el {
    tn std
}

Section Main
b 1 {
    q: s1,0
}
`,
			want: "time signature division",
		},
		{
			name: "bad duration",
			src: `NAME A

tp 92
time 4/4

el {
    tn std
}

Section Main
b 1 {
    z: s1,0
}
`,
			want: "unknown duration",
		},
		{
			name: "invalid note",
			src: `NAME A

tp 92
time 4/4

el {
    tn std
}

Section Main
b 1 {
    q: s7,0
}
`,
			want: "string number must be 1..6",
		},
		{
			name: "empty chord",
			src: `NAME A

tp 92
time 4/4

el {
    tn std
}

Section Main
b 1 {
    q: []
}
`,
			want: "chord cannot be empty",
		},
		{
			name: "invalid effect key",
			src: `NAME A

tp 92
time 4/4

el {
    tn std
    drv x 0.3
}

Section Main
b 1 {
    q: s1,0
}
`,
			want: "unknown parameter",
		},
		{
			name: "missing technique target",
			src: `NAME A

tp 92
time 4/4

el {
    tn std
}

Section Main
b 1 {
    q: s1,2 hammer
}
`,
			want: "requires target fret",
		},
		{
			name: "unexpected technique target",
			src: `NAME A

tp 92
time 4/4

el {
    tn std
}

Section Main
b 1 {
    q: s1,2 bend 7
}
`,
			want: "does not accept a target fret",
		},
		{
			name: "bar before section",
			src: `NAME A

tp 92
time 4/4

el {
    tn std
}

b 1 {
    q: s1,0
}
`,
			want: "bar declared before any Section",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.src)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error mismatch\n got: %v\nwant substring: %q", err, tc.want)
			}
		})
	}
}
