package parser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseExampleHelloSQM(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "hello.sqm")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read example file: %v", err)
	}

	file, err := Parse(string(data))
	if err != nil {
		t.Fatalf("Parse() returned error for examples/hello.sqm: %v", err)
	}

	if file.Name != "Simple Riff" {
		t.Fatalf("unexpected name: %q", file.Name)
	}
	if file.Tempo != 92 {
		t.Fatalf("unexpected tempo: %d", file.Tempo)
	}
	if file.Time.Beats != 4 || file.Time.Division != 4 {
		t.Fatalf("unexpected time signature: %+v", file.Time)
	}
	if file.Instrument == nil {
		t.Fatal("instrument was not parsed")
	}
	if len(file.Sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(file.Sections))
	}
	if len(file.Sections[0].Bars) != 3 {
		t.Fatalf("expected 3 bars, got %d", len(file.Sections[0].Bars))
	}

	eventCount := 0
	for _, bar := range file.Sections[0].Bars {
		eventCount += len(bar.Events)
	}
	if eventCount != 10 {
		t.Fatalf("expected 10 events across all bars, got %d", eventCount)
	}
}
