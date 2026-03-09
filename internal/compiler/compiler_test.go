package compiler

import (
	"os"
	"path/filepath"
	"testing"
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
	if score.TotalTicks != 5280 {
		t.Fatalf("unexpected total ticks: %d", score.TotalTicks)
	}
	if score.OpenMIDINotes[0] != 64 || score.OpenMIDINotes[5] != 40 {
		t.Fatalf("unexpected open string MIDI values: %+v", score.OpenMIDINotes)
	}
}

func TestCompileSourceTechnique(t *testing.T) {
	src := `NAME Technique Test

tempo 100
time 4/4

el {
    tn std
}

Section Main
b 1 {
    q: s2,5 hammer 7
    q: s2,7 pull 5
    q: s2,5 slide 7
    q: s1,7 bend
}
`

	score, err := CompileSource(src)
	if err != nil {
		t.Fatalf("CompileSource() returned error: %v", err)
	}
	if len(score.Notes) != 4 {
		t.Fatalf("expected 4 notes, got %d", len(score.Notes))
	}
}
