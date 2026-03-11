package midi

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"sqmus/internal/compiler"
)

func TestEncodeAndWriteFile(t *testing.T) {
	srcPath := filepath.Join("..", "..", "examples", "hello.sqm")
	src, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	score, err := compiler.CompileSource(string(src))
	if err != nil {
		t.Fatalf("CompileSource() returned error: %v", err)
	}

	data, err := Encode(score)
	if err != nil {
		t.Fatalf("Encode() returned error: %v", err)
	}
	if len(data) < 32 {
		t.Fatalf("encoded MIDI too small: %d bytes", len(data))
	}
	if !bytes.Equal(data[:4], []byte("MThd")) {
		t.Fatalf("missing MThd header")
	}
	if !bytes.Contains(data, []byte("MTrk")) {
		t.Fatalf("missing MTrk chunk")
	}
	if !bytes.Contains(data, []byte{0xE0}) {
		t.Fatalf("expected pitch-bend events for techniques")
	}
	if !bytes.Contains(data, []byte{0xB0, 101, 0}) {
		t.Fatalf("expected pitch-bend range configuration events")
	}
	if !bytes.Contains(data, []byte{0x99}) {
		t.Fatalf("expected drum channel note events")
	}

	outPath := filepath.Join(t.TempDir(), "test.mid")
	if err := WriteFile(score, outPath); err != nil {
		t.Fatalf("WriteFile() returned error: %v", err)
	}
	st, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("stat written file: %v", err)
	}
	if st.Size() == 0 {
		t.Fatalf("written MIDI file is empty")
	}
}
