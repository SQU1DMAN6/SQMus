package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"sqmus/internal/audio"
	"sqmus/internal/compiler"
	"sqmus/internal/midi"
	"sqmus/internal/tab"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}

	switch args[0] {
	case "compile":
		return runCompile(args[1:], stdout, stderr)
	case "tab":
		return runTab(args[1:], stdout, stderr)
	case "midi":
		return runMIDI(args[1:], stdout, stderr)
	case "png", "tabpng":
		return runTabPNG(args[1:], stdout, stderr)
	case "wav":
		return runWAV(args[1:], stdout, stderr)
	case "play":
		return runPlay(args[1:], stdout, stderr)
	case "export":
		return runExport(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		printUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n", args[0])
		printUsage(stderr)
		return 2
	}
}

func runCompile(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("compile")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	input, err := oneArg(fs.Args(), "compile")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	score, err := loadScore(input)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	fmt.Fprintf(stdout, "Compiled %q\n", score.Name)
	fmt.Fprintf(stdout, "Tempo: %d BPM\n", score.Tempo)
	fmt.Fprintf(stdout, "Time : %d/%d\n", score.Time.Beats, score.Time.Division)
	fmt.Fprintf(stdout, "Notes: %d\n", len(score.Notes))
	fmt.Fprintf(stdout, "Ticks: %d\n", score.TotalTicks)
	return 0
}

func runTab(args []string, stdout, stderr io.Writer) int {
	args = normalizeFlagOrder(args)
	fs := newFlagSet("tab")
	outPath := fs.String("o", "", "Output file path (default: stdout)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	input, err := oneArg(fs.Args(), "tab")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	score, err := loadScore(input)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	asciiTab, err := tab.GenerateASCII(score)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if *outPath == "" {
		fmt.Fprint(stdout, asciiTab)
		return 0
	}

	if err := os.WriteFile(*outPath, []byte(asciiTab), 0o644); err != nil {
		fmt.Fprintf(stderr, "write tab output: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Wrote tab to %s\n", *outPath)
	return 0
}

func runMIDI(args []string, stdout, stderr io.Writer) int {
	args = normalizeFlagOrder(args)
	fs := newFlagSet("midi")
	outPath := fs.String("o", "", "Output MIDI path (default: <input>.mid)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	input, err := oneArg(fs.Args(), "midi")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	score, err := loadScore(input)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	output := *outPath
	if output == "" {
		output = replaceExt(input, ".mid")
	}
	if err := midi.WriteFile(score, output); err != nil {
		fmt.Fprintf(stderr, "write MIDI: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Wrote MIDI to %s\n", output)
	return 0
}

func runWAV(args []string, stdout, stderr io.Writer) int {
	args = normalizeFlagOrder(args)
	fs := newFlagSet("wav")
	outPath := fs.String("o", "", "Output WAV path (default: <input>.wav)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	input, err := oneArg(fs.Args(), "wav")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	score, err := loadScore(input)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	output := *outPath
	if output == "" {
		output = replaceExt(input, ".wav")
	}
	if err := audio.RenderWAV(score, output); err != nil {
		fmt.Fprintf(stderr, "write WAV: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Wrote WAV to %s\n", output)
	return 0
}

func runTabPNG(args []string, stdout, stderr io.Writer) int {
	args = normalizeFlagOrder(args)
	fs := newFlagSet("png")
	outPath := fs.String("o", "", "Output PNG path (default: <input>.tab.png)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	input, err := oneArg(fs.Args(), "png")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	score, err := loadScore(input)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	output := *outPath
	if output == "" {
		output = replaceExt(input, ".tab.png")
	}
	if err := tab.GeneratePNG(score, output); err != nil {
		fmt.Fprintf(stderr, "write PNG: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Wrote PNG tab to %s\n", output)
	return 0
}

func runPlay(args []string, stdout, stderr io.Writer) int {
	args = normalizeFlagOrder(args)
	fs := newFlagSet("play")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	input, err := oneArg(fs.Args(), "play")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	score, err := loadScore(input)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	fmt.Fprintln(stdout, "Rendering and playing...")
	if err := audio.Play(score); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func runExport(args []string, stdout, stderr io.Writer) int {
	args = normalizeFlagOrder(args)
	fs := newFlagSet("export")
	outDir := fs.String("dir", ".", "Output directory for all exports")
	prefix := fs.String("name", "", "Override output basename")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	input, err := oneArg(fs.Args(), "export")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	score, err := loadScore(input)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	base := *prefix
	if strings.TrimSpace(base) == "" {
		base = strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))
	}

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fmt.Fprintf(stderr, "create output directory: %v\n", err)
		return 1
	}

	tabOut := filepath.Join(*outDir, base+".tab.txt")
	pngOut := filepath.Join(*outDir, base+".tab.png")
	midiOut := filepath.Join(*outDir, base+".mid")
	wavOut := filepath.Join(*outDir, base+".wav")

	asciiTab, err := tab.GenerateASCII(score)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := os.WriteFile(tabOut, []byte(asciiTab), 0o644); err != nil {
		fmt.Fprintf(stderr, "write tab output: %v\n", err)
		return 1
	}
	if err := tab.WritePNGFromASCII(asciiTab, pngOut); err != nil {
		fmt.Fprintf(stderr, "write PNG tab output: %v\n", err)
		return 1
	}
	if err := midi.WriteFile(score, midiOut); err != nil {
		fmt.Fprintf(stderr, "write MIDI: %v\n", err)
		return 1
	}
	if err := audio.RenderWAV(score, wavOut); err != nil {
		fmt.Fprintf(stderr, "write WAV: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "Wrote tab : %s\n", tabOut)
	fmt.Fprintf(stdout, "Wrote png : %s\n", pngOut)
	fmt.Fprintf(stdout, "Wrote midi: %s\n", midiOut)
	fmt.Fprintf(stdout, "Wrote wav : %s\n", wavOut)
	return 0
}

func loadScore(path string) (*compiler.Score, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return compiler.CompileSource(string(src))
}

func oneArg(args []string, cmd string) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("usage: sqmus %s <file.sqm>", cmd)
	}
	if strings.TrimSpace(args[0]) == "" {
		return "", errors.New("input path cannot be empty")
	}
	return args[0], nil
}

func replaceExt(path, ext string) string {
	base := strings.TrimSuffix(path, filepath.Ext(path))
	return base + ext
}

// normalizeFlagOrder allows flag arguments after the positional input path.
func normalizeFlagOrder(args []string) []string {
	if len(args) <= 1 {
		return args
	}

	flags := make([]string, 0, len(args))
	positionals := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
			if strings.Contains(arg, "=") {
				continue
			}
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				flags = append(flags, args[i+1])
				i++
			}
			continue
		}
		positionals = append(positionals, arg)
	}

	return append(flags, positionals...)
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "SQMus CLI")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  sqmus compile <song.sqm>")
	fmt.Fprintln(w, "  sqmus tab <song.sqm> [-o output.txt]")
	fmt.Fprintln(w, "  sqmus png <song.sqm> [-o output.png]")
	fmt.Fprintln(w, "  sqmus midi <song.sqm> [-o output.mid]")
	fmt.Fprintln(w, "  sqmus wav <song.sqm> [-o output.wav]")
	fmt.Fprintln(w, "  sqmus play <song.sqm>")
	fmt.Fprintln(w, "  sqmus export <song.sqm> [-dir out] [-name basename]")
}
