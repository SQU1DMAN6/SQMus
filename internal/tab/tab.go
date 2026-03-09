package tab

import (
	"fmt"
	"strconv"
	"strings"

	"sqmus/internal/compiler"
)

const unitDivisor = 4 // sixteenth-note grid when PPQ is quarter-note based.

// GenerateASCII renders a score into ASCII tablature.
func GenerateASCII(score *compiler.Score) (string, error) {
	if score == nil {
		return "", fmt.Errorf("score is nil")
	}

	unitTicks := score.PPQ / unitDivisor
	if unitTicks <= 0 {
		unitTicks = 120
	}

	gridUnits := 1
	if score.TotalTicks > 0 {
		gridUnits = (score.TotalTicks+unitTicks-1)/unitTicks + 2
	}

	lines := make([][]rune, 6)
	for i := 0; i < 6; i++ {
		lines[i] = make([]rune, gridUnits)
		for j := range lines[i] {
			lines[i][j] = '-'
		}
	}

	for _, note := range score.Notes {
		if note.String < 1 || note.String > 6 {
			continue
		}
		lineIdx := note.String - 1
		pos := note.StartTicks / unitTicks
		if pos < 0 {
			pos = 0
		}

		fret := strconv.Itoa(note.Fret)
		ensureLineWidth(lines, pos+len(fret)+1)
		for i, ch := range fret {
			lines[lineIdx][pos+i] = ch
		}
	}

	barUnits := barLengthUnits(score, unitTicks)
	if barUnits > 0 {
		for i := range lines {
			for pos := barUnits; pos < len(lines[i]); pos += barUnits {
				if lines[i][pos] == '-' {
					lines[i][pos] = '|'
				}
			}
		}
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Tempo: %d\n", score.Tempo))
	b.WriteString(fmt.Sprintf("Time : %d/%d\n\n", score.Time.Beats, score.Time.Division))

	for i := 0; i < 6; i++ {
		label := score.StringNames[i]
		if label == "" {
			label = defaultLabelByString(i + 1)
		}
		if len(label) > 2 {
			label = label[:2]
		}
		b.WriteString(label)
		b.WriteRune('|')
		b.WriteString(string(lines[i]))
		b.WriteString("|\n")
	}

	return b.String(), nil
}

func ensureLineWidth(lines [][]rune, width int) {
	for i := range lines {
		if len(lines[i]) >= width {
			continue
		}
		extra := make([]rune, width-len(lines[i]))
		for j := range extra {
			extra[j] = '-'
		}
		lines[i] = append(lines[i], extra...)
	}
}

func barLengthUnits(score *compiler.Score, unitTicks int) int {
	if score.Time.Beats <= 0 || score.Time.Division <= 0 || unitTicks <= 0 {
		return 0
	}
	barTicks := score.Time.Beats * score.PPQ * 4 / score.Time.Division
	if barTicks <= 0 {
		return 0
	}
	units := barTicks / unitTicks
	if units <= 0 {
		return 0
	}
	return units
}

func defaultLabelByString(stringNumber int) string {
	switch stringNumber {
	case 1:
		return "e"
	case 2:
		return "B"
	case 3:
		return "G"
	case 4:
		return "D"
	case 5:
		return "A"
	default:
		return "E"
	}
}
