package tab

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"strconv"

	"sqmus/internal/compiler"
)

const (
	pngLeftMargin  = 56
	pngTopMargin   = 28
	pngRightMargin = 28
	pngBottom      = 32
	pngLineSpacing = 28
	pngColWidth    = 26
	pngFontScale   = 3
	pngSystemGap   = 26
	pngMaxWidth    = 1400
)

var (
	colBackground = color.RGBA{R: 250, G: 250, B: 250, A: 255}
	colLine       = color.RGBA{R: 32, G: 32, B: 32, A: 255}
	colText       = color.RGBA{R: 20, G: 20, B: 20, A: 255}
	colGrid       = color.RGBA{R: 210, G: 210, B: 210, A: 255}
	colBeat       = color.RGBA{R: 165, G: 165, B: 165, A: 255}
)

// RenderPNG renders a score into a PNG tablature image.
func RenderPNG(score *compiler.Score) ([]byte, error) {
	if score == nil {
		return nil, fmt.Errorf("score is nil")
	}

	unitTicks := score.PPQ / unitDivisor
	if unitTicks <= 0 {
		unitTicks = 120
	}
	gridUnits := 1
	if score.TotalTicks > 0 {
		gridUnits = (score.TotalTicks+unitTicks-1)/unitTicks + 2
	}

	colsPerSystem := columnsPerSystem(gridUnits)
	systems := (gridUnits + colsPerSystem - 1) / colsPerSystem
	staffHeight := pngLineSpacing * 5
	width := pngLeftMargin + colsPerSystem*pngColWidth + pngRightMargin
	height := pngTopMargin + systems*staffHeight + (systems-1)*pngSystemGap + pngBottom
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: colBackground}, image.Point{}, draw.Src)

	barUnits := barLengthUnits(score, unitTicks)
	beatUnits := beatLengthUnits(score, unitTicks)
	textOffset := (7 * pngFontScale) / 2

	for sys := 0; sys < systems; sys++ {
		systemStart := sys * colsPerSystem
		systemEnd := systemStart + colsPerSystem
		if systemEnd > gridUnits {
			systemEnd = gridUnits
		}

		baseY := pngTopMargin + sys*(staffHeight+pngSystemGap)
		lineYs := make([]int, 6)
		for i := 0; i < 6; i++ {
			lineYs[i] = baseY + i*pngLineSpacing
			drawHLine(img, 0, width-1, lineYs[i], colLine)
		}

		drawGridLines(img, lineYs[0], lineYs[5], systemStart, systemEnd, colsPerSystem, barUnits, beatUnits)

		for i := 0; i < 6; i++ {
			label := score.StringNames[i]
			if label == "" {
				label = defaultLabelByString(i + 1)
			}
			drawText(img, 8, lineYs[i]-textOffset, label, pngFontScale, colText)
		}

		for _, note := range score.Notes {
			if note.String < 1 || note.String > 6 {
				continue
			}
			unit := note.StartTicks / unitTicks
			if unit < systemStart || unit >= systemEnd {
				continue
			}
			localUnit := unit - systemStart
			fret := strconv.Itoa(note.Fret)
			digitWidth := textWidth(fret, pngFontScale)
			x := pngLeftMargin + localUnit*pngColWidth + (pngColWidth-digitWidth)/2
			y := lineYs[note.String-1] - textOffset
			drawText(img, x, y, fret, pngFontScale, colText)
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// WritePNG writes the PNG tab output to disk.
func WritePNG(score *compiler.Score, outputPath string) error {
	if outputPath == "" {
		return fmt.Errorf("output path is required")
	}
	data, err := RenderPNG(score)
	if err != nil {
		return err
	}
	return os.WriteFile(outputPath, data, 0o644)
}

func drawHLine(img *image.RGBA, x0, x1, y int, c color.RGBA) {
	if y < 0 || y >= img.Bounds().Dy() {
		return
	}
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	if x0 < 0 {
		x0 = 0
	}
	if x1 >= img.Bounds().Dx() {
		x1 = img.Bounds().Dx() - 1
	}
	for x := x0; x <= x1; x++ {
		img.SetRGBA(x, y, c)
	}
}

func drawVLine(img *image.RGBA, x, y0, y1 int, c color.RGBA) {
	if x < 0 || x >= img.Bounds().Dx() {
		return
	}
	if y0 > y1 {
		y0, y1 = y1, y0
	}
	if y0 < 0 {
		y0 = 0
	}
	if y1 >= img.Bounds().Dy() {
		y1 = img.Bounds().Dy() - 1
	}
	for y := y0; y <= y1; y++ {
		img.SetRGBA(x, y, c)
	}
}

func drawVLineWidth(img *image.RGBA, x, y0, y1 int, c color.RGBA, width int) {
	if width <= 1 {
		drawVLine(img, x, y0, y1, c)
		return
	}
	half := width / 2
	for dx := -half; dx <= half; dx++ {
		drawVLine(img, x+dx, y0, y1, c)
	}
}

func drawText(img *image.RGBA, x, y int, text string, scale int, c color.RGBA) {
	if scale < 1 {
		scale = 1
	}
	cursor := x
	for _, ch := range text {
		glyph, ok := font5x7[ch]
		if !ok {
			glyph = font5x7['?']
		}
		drawGlyph(img, cursor, y, glyph, scale, c)
		cursor += (len(glyph[0]) + 1) * scale
	}
}

func drawGlyph(img *image.RGBA, x, y int, glyph []string, scale int, c color.RGBA) {
	for row := 0; row < len(glyph); row++ {
		for col := 0; col < len(glyph[row]); col++ {
			if glyph[row][col] != '1' {
				continue
			}
			for sy := 0; sy < scale; sy++ {
				for sx := 0; sx < scale; sx++ {
					px := x + col*scale + sx
					py := y + row*scale + sy
					if px >= 0 && py >= 0 && px < img.Bounds().Dx() && py < img.Bounds().Dy() {
						img.SetRGBA(px, py, c)
					}
				}
			}
		}
	}
}

func textWidth(text string, scale int) int {
	if scale < 1 {
		scale = 1
	}
	width := 0
	for _, ch := range text {
		glyph, ok := font5x7[ch]
		if !ok {
			glyph = font5x7['?']
		}
		width += (len(glyph[0]) + 1) * scale
	}
	if width > 0 {
		width -= scale
	}
	return width
}

func columnsPerSystem(gridUnits int) int {
	if gridUnits <= 0 {
		return 1
	}
	usable := pngMaxWidth - pngLeftMargin - pngRightMargin
	if usable < pngColWidth*8 {
		usable = pngColWidth * 8
	}
	cols := usable / pngColWidth
	if cols < 8 {
		cols = 8
	}
	if cols > gridUnits {
		cols = gridUnits
	}
	return cols
}

func beatLengthUnits(score *compiler.Score, unitTicks int) int {
	if score.Time.Division <= 0 || unitTicks <= 0 {
		return 0
	}
	beatTicks := score.PPQ * 4 / score.Time.Division
	if beatTicks <= 0 {
		return 0
	}
	units := beatTicks / unitTicks
	if units <= 0 {
		return 0
	}
	return units
}

func drawGridLines(img *image.RGBA, top, bottom, systemStart, systemEnd, colsPerSystem, barUnits, beatUnits int) {
	for local := 0; local <= colsPerSystem; local++ {
		global := systemStart + local
		if global > systemEnd {
			break
		}
		x := pngLeftMargin + local*pngColWidth
		if barUnits > 0 && global%barUnits == 0 && global != 0 {
			drawVLineWidth(img, x, top-6, bottom+6, colLine, 2)
			continue
		}
		if beatUnits > 0 && global%beatUnits == 0 && global != 0 {
			drawVLine(img, x, top-4, bottom+4, colBeat)
			continue
		}
		drawVLine(img, x, top-2, bottom+2, colGrid)
	}
}

var font5x7 = map[rune][]string{
	'0': {"111", "101", "101", "101", "101", "101", "111"},
	'1': {"010", "110", "010", "010", "010", "010", "111"},
	'2': {"111", "001", "001", "111", "100", "100", "111"},
	'3': {"111", "001", "001", "111", "001", "001", "111"},
	'4': {"101", "101", "101", "111", "001", "001", "001"},
	'5': {"111", "100", "100", "111", "001", "001", "111"},
	'6': {"111", "100", "100", "111", "101", "101", "111"},
	'7': {"111", "001", "001", "010", "010", "010", "010"},
	'8': {"111", "101", "101", "111", "101", "101", "111"},
	'9': {"111", "101", "101", "111", "001", "001", "111"},
	'A': {"111", "101", "101", "111", "101", "101", "101"},
	'B': {"110", "101", "101", "110", "101", "101", "110"},
	'C': {"111", "100", "100", "100", "100", "100", "111"},
	'D': {"110", "101", "101", "101", "101", "101", "110"},
	'E': {"111", "100", "100", "110", "100", "100", "111"},
	'F': {"111", "100", "100", "110", "100", "100", "100"},
	'G': {"111", "100", "100", "101", "101", "101", "111"},
	'e': {"000", "000", "110", "101", "111", "100", "111"},
	'#': {"101", "101", "111", "101", "111", "101", "101"},
	'b': {"100", "100", "110", "101", "101", "101", "110"},
	'c': {"000", "000", "110", "100", "100", "100", "110"},
	'?': {"111", "001", "010", "010", "010", "000", "010"},
}
