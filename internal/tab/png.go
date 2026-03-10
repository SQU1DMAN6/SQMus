package tab

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"strings"

	"sqmus/internal/compiler"
)

const (
	glyphWidth  = 5
	glyphHeight = 7
	cellWidth   = 6
	cellHeight  = 8
	renderScale = 2
	margin      = 8
)

var glyphs = map[rune][glyphHeight]string{
	' ': {"00000", "00000", "00000", "00000", "00000", "00000", "00000"},
	'-': {"00000", "00000", "00000", "11111", "00000", "00000", "00000"},
	'|': {"00100", "00100", "00100", "00100", "00100", "00100", "00100"},
	':': {"00000", "00100", "00100", "00000", "00100", "00100", "00000"},
	'/': {"00001", "00010", "00100", "01000", "10000", "00000", "00000"},
	'#': {"01010", "11111", "01010", "01010", "11111", "01010", "01010"},
	'?': {"01110", "10001", "00001", "00010", "00100", "00000", "00100"},
	'0': {"01110", "10001", "10011", "10101", "11001", "10001", "01110"},
	'1': {"00100", "01100", "00100", "00100", "00100", "00100", "01110"},
	'2': {"01110", "10001", "00001", "00010", "00100", "01000", "11111"},
	'3': {"11110", "00001", "00001", "01110", "00001", "00001", "11110"},
	'4': {"00010", "00110", "01010", "10010", "11111", "00010", "00010"},
	'5': {"11111", "10000", "11110", "00001", "00001", "10001", "01110"},
	'6': {"00110", "01000", "10000", "11110", "10001", "10001", "01110"},
	'7': {"11111", "00001", "00010", "00100", "01000", "10000", "10000"},
	'8': {"01110", "10001", "10001", "01110", "10001", "10001", "01110"},
	'9': {"01110", "10001", "10001", "01111", "00001", "00010", "11100"},
	'A': {"01110", "10001", "10001", "11111", "10001", "10001", "10001"},
	'B': {"11110", "10001", "10001", "11110", "10001", "10001", "11110"},
	'C': {"01110", "10001", "10000", "10000", "10000", "10001", "01110"},
	'D': {"11110", "10001", "10001", "10001", "10001", "10001", "11110"},
	'E': {"11111", "10000", "10000", "11110", "10000", "10000", "11111"},
	'F': {"11111", "10000", "10000", "11110", "10000", "10000", "10000"},
	'G': {"01110", "10001", "10000", "10111", "10001", "10001", "01110"},
	'I': {"11111", "00100", "00100", "00100", "00100", "00100", "11111"},
	'M': {"10001", "11011", "10101", "10001", "10001", "10001", "10001"},
	'O': {"01110", "10001", "10001", "10001", "10001", "10001", "01110"},
	'P': {"11110", "10001", "10001", "11110", "10000", "10000", "10000"},
	'T': {"11111", "00100", "00100", "00100", "00100", "00100", "00100"},
}

// GeneratePNG renders ASCII tab content into a PNG file.
func GeneratePNG(score *compiler.Score, outputPath string) error {
	ascii, err := GenerateASCII(score)
	if err != nil {
		return err
	}
	return WritePNGFromASCII(ascii, outputPath)
}

// WritePNGFromASCII writes a PNG text rendering of a tab string.
func WritePNGFromASCII(asciiTab, outputPath string) error {
	if strings.TrimSpace(asciiTab) == "" {
		return fmt.Errorf("tab text is empty")
	}
	if outputPath == "" {
		return fmt.Errorf("output path is required")
	}

	lines := strings.Split(strings.TrimRight(asciiTab, "\n"), "\n")
	maxChars := 0
	for _, line := range lines {
		if len(line) > maxChars {
			maxChars = len(line)
		}
	}
	if maxChars == 0 {
		return fmt.Errorf("tab text has no drawable content")
	}

	width := (maxChars*cellWidth+margin*2)*renderScale + 1
	height := (len(lines)*cellHeight+margin*2)*renderScale + 1
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)

	for row, line := range lines {
		for col, r := range line {
			drawGlyph(img, margin+col*cellWidth, margin+row*cellHeight, r)
		}
	}

	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create png file: %w", err)
	}
	defer f.Close()

	if err := png.Encode(f, img); err != nil {
		return fmt.Errorf("encode png: %w", err)
	}
	return nil
}

func drawGlyph(img *image.RGBA, x, y int, r rune) {
	if r >= 'a' && r <= 'z' {
		r = r - ('a' - 'A')
	}
	glyph, ok := glyphs[r]
	if !ok {
		glyph = glyphs['?']
	}

	for gy := 0; gy < glyphHeight; gy++ {
		row := glyph[gy]
		for gx := 0; gx < glyphWidth; gx++ {
			if row[gx] != '1' {
				continue
			}
			px := (x + gx) * renderScale
			py := (y + gy) * renderScale
			for sy := 0; sy < renderScale; sy++ {
				for sx := 0; sx < renderScale; sx++ {
					img.Set(px+sx, py+sy, color.Black)
				}
			}
		}
	}
}
