package compiler

import (
	"os"
	"path/filepath"
	"testing"

	"sqmus/internal/ast"
)

func TestCompileSourceHelloExample(t *testing.T) {
	srcPath := filepath.Join("..", "..", "examples", "hello.sqm")
	src, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("read example source: %v", err)
	}

	score, err := CompileSource(string(src))
	if err != nil {
		t.Fatalf("CompileSource() returned error: %v", err)
	}

	if score.Name != "Simple Riff" {
		t.Fatalf("unexpected name: %q", score.Name)
	}
	if score.Tempo != 92 {
		t.Fatalf("unexpected tempo: %d", score.Tempo)
	}
	if score.Time.Beats != 4 || score.Time.Division != 4 {
		t.Fatalf("unexpected time signature: %+v", score.Time)
	}
	if len(score.Notes) != 9 {
		t.Fatalf("unexpected note count: %d", len(score.Notes))
	}
	if len(score.Drums) != 4 {
		t.Fatalf("unexpected drum count: %d", len(score.Drums))
	}
	if score.TotalTicks != 7440 {
		t.Fatalf("unexpected total ticks: %d", score.TotalTicks)
	}
	if score.OpenMIDINotes[0] != 64 || score.OpenMIDINotes[5] != 40 {
		t.Fatalf("unexpected open string MIDI values: %+v", score.OpenMIDINotes)
	}
	if score.Config.Drive <= 0.45 || score.Config.Drive >= 0.50 {
		t.Fatalf("expected mapped drive setting, got %.3f", score.Config.Drive)
	}
	if score.Config.AmpGain <= 0.70 || score.Config.AmpGain >= 0.74 {
		t.Fatalf("expected mapped amp gain setting, got %.3f", score.Config.AmpGain)
	}

	hammer := findNote(score.Notes, 960, 2)
	if hammer == nil {
		t.Fatalf("expected hammer note at tick 960 on string 2")
	}
	if hammer.Technique != ast.TechniqueHammer || hammer.TechniqueTargetMIDI <= hammer.MIDI {
		t.Fatalf("expected hammer metadata, got %+v", *hammer)
	}

	bend := findNote(score.Notes, 2640, 2)
	if bend == nil {
		t.Fatalf("expected bend note at tick 2640 on string 2")
	}
	if bend.Technique != ast.TechniqueBend || bend.TechniqueTargetMIDI <= bend.MIDI {
		t.Fatalf("expected bend metadata, got %+v", *bend)
	}
}

func TestCompileSourceTechniqueVariants(t *testing.T) {
	src := `NAME Technique Test

tp 100
time 4/4

el {
    tn std
    amp g 0.7 t 0.6
}

dr {
    kit std
}

Section Main
b 1 {
    q: s2,5 hammer 7
    q: s2,7 pull 5
    q: s2,5 slide 7
    q: s1,7 bend
    q: s1,8 vibrato
    q: s1,7 harmonic
}

b 2 {
    q: dk
}
`

	score, err := CompileSource(src)
	if err != nil {
		t.Fatalf("CompileSource() returned error: %v", err)
	}
	if len(score.Notes) != 6 {
		t.Fatalf("expected 6 notes, got %d", len(score.Notes))
	}
	if len(score.Drums) != 1 {
		t.Fatalf("expected 1 drum hit, got %d", len(score.Drums))
	}

	kinds := map[ast.TechniqueKind]bool{}
	for _, note := range score.Notes {
		if note.Technique != "" {
			kinds[note.Technique] = true
		}
	}
	for _, want := range []ast.TechniqueKind{
		ast.TechniqueHammer,
		ast.TechniquePull,
		ast.TechniqueSlide,
		ast.TechniqueBend,
		ast.TechniqueVibrato,
		ast.TechniqueHarmonic,
	} {
		if !kinds[want] {
			t.Fatalf("missing compiled technique %q", want)
		}
	}
}

func TestAugmentedDurationTicks(t *testing.T) {
	src := `NAME Aug

tp 120
time 4/4

el {
    tn std
}

Section Main
b 1 {
    q aug: s1,0
}
`

	score, err := CompileSource(src)
	if err != nil {
		t.Fatalf("CompileSource() returned error: %v", err)
	}
	if score.TotalTicks != 720 {
		t.Fatalf("expected augmented quarter duration (720 ticks), got %d", score.TotalTicks)
	}
}

func findNote(notes []NoteEvent, startTick, stringNum int) *NoteEvent {
	for i := range notes {
		if notes[i].StartTicks == startTick && notes[i].String == stringNum {
			return &notes[i]
		}
	}
	return nil
}
