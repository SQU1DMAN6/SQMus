package audio

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"sqmus/internal/compiler"
)

func TestRenderWAV(t *testing.T) {
	srcPath := filepath.Join("..", "..", "examples", "hello.sqm")
	src, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	score, err := compiler.CompileSource(string(src))
	if err != nil {
		t.Fatalf("CompileSource() returned error: %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "test.wav")
	if err := RenderWAV(score, outPath); err != nil {
		t.Fatalf("RenderWAV() returned error: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read wav output: %v", err)
	}
	if len(data) < 44 {
		t.Fatalf("WAV output too small: %d", len(data))
	}
	if !bytes.Equal(data[:4], []byte("RIFF")) {
		t.Fatalf("missing RIFF header")
	}
	if !bytes.Equal(data[8:12], []byte("WAVE")) {
		t.Fatalf("missing WAVE header")
	}
	nonZero := false
	for _, b := range data[44:] {
		if b != 0 {
			nonZero = true
			break
		}
	}
	if !nonZero {
		t.Fatalf("audio payload is unexpectedly silent")
	}
}
