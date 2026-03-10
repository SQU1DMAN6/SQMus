package tab

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"sqmus/internal/compiler"
)

func TestGeneratePNG(t *testing.T) {
	srcPath := filepath.Join("..", "..", "examples", "hello.sqm")
	src, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	score, err := compiler.CompileSource(string(src))
	if err != nil {
		t.Fatalf("CompileSource() returned error: %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "hello.tab.png")
	if err := GeneratePNG(score, outPath); err != nil {
		t.Fatalf("GeneratePNG() returned error: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read png output: %v", err)
	}
	if len(data) < 8 {
		t.Fatalf("png output too small: %d", len(data))
	}
	pngSignature := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	if !bytes.Equal(data[:8], pngSignature) {
		t.Fatalf("invalid PNG signature")
	}
}
