package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLICompileAndExport(t *testing.T) {
	input := filepath.Join("..", "..", "examples", "hello.sqm")
	outDir := t.TempDir()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"compile", input}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("compile command failed with code %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Compiled") {
		t.Fatalf("unexpected compile output: %s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"export", "-dir", outDir, input}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("export command failed with code %d, stderr=%s", code, stderr.String())
	}

	for _, name := range []string{"hello.png", "hello.mid", "hello.wav"} {
		path := filepath.Join(outDir, name)
		st, err := os.Stat(path)
		if err != nil {
			t.Fatalf("expected output file %s: %v", path, err)
		}
		if st.Size() == 0 {
			t.Fatalf("output file is empty: %s", path)
		}
	}
}

func TestCLIUsageForUnknownCommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"nope"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(stderr.String(), "unknown command") {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}
