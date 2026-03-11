package tab

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sqmus/internal/compiler"
)

func TestGenerateASCII(t *testing.T) {
	srcPath := filepath.Join("..", "..", "examples", "hello.sqm")
	src, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("read source: %v", err)
	}

	score, err := compiler.CompileSource(string(src))
	if err != nil {
		t.Fatalf("CompileSource() returned error: %v", err)
	}

	out, err := GenerateASCII(score)
	if err != nil {
		t.Fatalf("GenerateASCII() returned error: %v", err)
	}

	if !strings.Contains(out, "Tempo: 92") {
		t.Fatalf("missing tempo header in tab output:\n%s", out)
	}
	if !strings.Contains(out, "Time : 4/4") {
		t.Fatalf("missing time header in tab output:\n%s", out)
	}
	if !strings.Contains(out, "e|") || !strings.Contains(out, "B|") {
		t.Fatalf("expected string labels in output:\n%s", out)
	}
	if !strings.Contains(out, "3") {
		t.Fatalf("expected fret values in output:\n%s", out)
	}
}

func TestRenderPNG(t *testing.T) {
	srcPath := filepath.Join("..", "..", "examples", "hello.sqm")
	src, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("read source: %v", err)
	}

	score, err := compiler.CompileSource(string(src))
	if err != nil {
		t.Fatalf("CompileSource() returned error: %v", err)
	}

	data, err := RenderPNG(score)
	if err != nil {
		t.Fatalf("RenderPNG() returned error: %v", err)
	}
	if len(data) < 8 {
		t.Fatalf("PNG output too small")
	}
	if !bytes.Equal(data[:8], []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}) {
		t.Fatalf("missing PNG signature")
	}
}
