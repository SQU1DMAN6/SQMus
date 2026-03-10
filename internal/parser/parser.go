package parser

import (
	"fmt"
	"strconv"
	"strings"

	"sqmus/internal/ast"
	"sqmus/internal/lexer"
)

var knownEffectParams = map[string]map[string]struct{}{
	"drv": {
		"g": {},
		"t": {},
		"l": {},
		"m": {},
	},
	"dly": {
		"t": {},
		"f": {},
		"m": {},
	},
	"rev": {
		"r": {},
		"m": {},
	},
	"cho": {
		"d": {},
		"r": {},
		"m": {},
	},
	"amp": {
		"g": {},
		"t": {},
		"l": {},
	},
	"cab": {
		"t": {},
	},
	"pick": {
		"p": {},
		"a": {},
	},
	"str": {
		"d": {},
	},
	"body": {
		"r": {},
	},
	"noi": {
		"l": {},
	},
}

var durations = map[string]ast.Duration{
	"w": ast.DurationWhole,
	"h": ast.DurationHalf,
	"q": ast.DurationQuarter,
	"e": ast.DurationEighth,
	"s": ast.DurationSixteenth,
	"t": ast.DurationThirtySecond,
}

var techniqueAliases = map[string]ast.TechniqueKind{
	"hammer":   ast.TechniqueHammer,
	"hm":       ast.TechniqueHammer,
	"pull":     ast.TechniquePull,
	"pl":       ast.TechniquePull,
	"slide":    ast.TechniqueSlide,
	"sl":       ast.TechniqueSlide,
	"bend":     ast.TechniqueBend,
	"bd":       ast.TechniqueBend,
	"vibrato":  ast.TechniqueVibrato,
	"vb":       ast.TechniqueVibrato,
	"harmonic": ast.TechniqueHarmonic,
	"hg":       ast.TechniqueHarmonic,
}

var techniquesRequireTarget = map[ast.TechniqueKind]struct{}{
	ast.TechniqueHammer: {},
	ast.TechniquePull:   {},
	ast.TechniqueSlide:  {},
}

// ParseError captures parser failures with source position.
type ParseError struct {
	Line   int
	Column int
	Msg    string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("parse error at %d:%d: %s", e.Line, e.Column, e.Msg)
}

// Parse lexes and parses SQMus source into an AST.
func Parse(input string) (*ast.File, error) {
	tokens, err := lexer.Lex(input)
	if err != nil {
		return nil, err
	}
	p := &parser{
		tokens:         tokens,
		file:           &ast.File{},
		currentSection: -1,
	}
	return p.parseFile()
}

type parser struct {
	tokens []lexer.Token
	pos    int

	file           *ast.File
	currentSection int
}

func (p *parser) parseFile() (*ast.File, error) {
	for {
		p.skipNewlines()
		tok := p.current()
		if tok.Type == lexer.TokenEOF {
			break
		}

		switch tok.Type {
		case lexer.TokenName:
			if err := p.parseName(); err != nil {
				return nil, err
			}
		case lexer.TokenTempo:
			if err := p.parseTempo(); err != nil {
				return nil, err
			}
		case lexer.TokenTime:
			if err := p.parseTime(); err != nil {
				return nil, err
			}
		case lexer.TokenSection:
			if err := p.parseSection(); err != nil {
				return nil, err
			}
		case lexer.TokenBar:
			if err := p.parseBar(); err != nil {
				return nil, err
			}
		case lexer.TokenIdent:
			if tok.Literal == "b" {
				if err := p.parseBar(); err != nil {
					return nil, err
				}
				break
			}
			if isGuitarType(tok.Literal) {
				if err := p.parseInstrument(); err != nil {
					return nil, err
				}
				break
			}
			return nil, p.errorf(tok, "unexpected identifier %q", tok.Literal)
		default:
			return nil, p.errorf(tok, "unexpected token %s", tok.Type)
		}
	}

	return p.file, nil
}

func (p *parser) parseName() error {
	tok := p.advance()
	if p.file.Name != "" {
		return p.errorf(tok, "duplicate NAME declaration")
	}

	parts, err := p.collectLineValues()
	if err != nil {
		return err
	}
	if len(parts) == 0 {
		return p.errorf(tok, "NAME requires a value")
	}

	p.file.Name = strings.Join(parts, " ")
	return nil
}

func (p *parser) parseTempo() error {
	tok := p.advance()
	if p.file.Tempo != 0 {
		return p.errorf(tok, "duplicate tempo declaration")
	}

	valueTok := p.current()
	if valueTok.Type != lexer.TokenInt {
		return p.errorf(valueTok, "tempo must be an integer")
	}
	tempo, err := strconv.Atoi(valueTok.Literal)
	if err != nil || tempo <= 0 {
		return p.errorf(valueTok, "tempo must be a positive integer")
	}
	p.advance()

	if err := p.expectLineBoundary("tempo"); err != nil {
		return err
	}

	p.file.Tempo = tempo
	return nil
}

func (p *parser) parseTime() error {
	tok := p.advance()
	if p.file.Time.Beats != 0 || p.file.Time.Division != 0 {
		return p.errorf(tok, "duplicate time declaration")
	}

	beatsTok := p.current()
	if beatsTok.Type != lexer.TokenInt {
		return p.errorf(beatsTok, "time signature beats must be an integer")
	}
	beats, err := strconv.Atoi(beatsTok.Literal)
	if err != nil || beats <= 0 {
		return p.errorf(beatsTok, "time signature beats must be positive")
	}
	p.advance()

	if _, err := p.expect(lexer.TokenSlash, "expected '/' in time signature"); err != nil {
		return err
	}

	divTok := p.current()
	if divTok.Type != lexer.TokenInt {
		return p.errorf(divTok, "time signature division must be an integer")
	}
	division, err := strconv.Atoi(divTok.Literal)
	if err != nil || division <= 0 {
		return p.errorf(divTok, "time signature division must be positive")
	}
	p.advance()

	if err := p.expectLineBoundary("time"); err != nil {
		return err
	}

	p.file.Time = ast.TimeSignature{Beats: beats, Division: division}
	return nil
}

func (p *parser) parseInstrument() error {
	typeTok := p.advance()
	if p.file.Instrument != nil {
		return p.errorf(typeTok, "only one instrument block is supported")
	}

	inst := &ast.Instrument{Type: ast.GuitarType(typeTok.Literal)}
	tuningSeen := false

	if _, err := p.expect(lexer.TokenLBrace, "expected '{' after instrument type"); err != nil {
		return err
	}

	for {
		p.skipNewlines()
		if p.current().Type == lexer.TokenRBrace {
			p.advance()
			break
		}
		if p.current().Type == lexer.TokenEOF {
			return p.errorf(p.current(), "unterminated instrument block")
		}

		cmdTok := p.current()
		if !isWordToken(cmdTok.Type) {
			return p.errorf(cmdTok, "expected instrument command")
		}
		cmd := cmdTok.Literal
		p.advance()

		if cmd == "tn" {
			if tuningSeen {
				return p.errorf(cmdTok, "duplicate tuning command")
			}
			if err := p.parseTuning(inst); err != nil {
				return err
			}
			tuningSeen = true
			continue
		}

		if err := p.parseEffect(inst, cmdTok); err != nil {
			return err
		}
	}

	p.file.Instrument = inst
	return nil
}

func (p *parser) parseTuning(inst *ast.Instrument) error {
	parts := make([]string, 0, 6)
	for !p.atLineBoundary() {
		tok := p.current()
		if !isWordToken(tok.Type) {
			return p.errorf(tok, "invalid tuning token %q", tok.Literal)
		}
		parts = append(parts, tok.Literal)
		p.advance()
	}

	if len(parts) == 0 {
		return p.errorf(p.current(), "tn requires a preset or explicit tuning")
	}
	if len(parts) == 1 {
		inst.Tuning = ast.Tuning{Preset: parts[0]}
		return nil
	}
	if len(parts) != 6 {
		return p.errorf(p.current(), "explicit tuning must define 6 strings")
	}

	inst.Tuning = ast.Tuning{Strings: parts}
	return nil
}

func (p *parser) parseEffect(inst *ast.Instrument, effectTok lexer.Token) error {
	effectName := effectTok.Literal
	params := make([]ast.EffectParam, 0)

	for !p.atLineBoundary() {
		keyTok := p.current()
		if !isWordToken(keyTok.Type) {
			return p.errorf(keyTok, "expected effect parameter key")
		}
		p.advance()

		valueTok := p.current()
		if valueTok.Type != lexer.TokenInt && valueTok.Type != lexer.TokenFloat {
			return p.errorf(valueTok, "expected numeric value for parameter %q", keyTok.Literal)
		}
		value, err := strconv.ParseFloat(valueTok.Literal, 64)
		if err != nil {
			return p.errorf(valueTok, "invalid numeric value %q", valueTok.Literal)
		}
		p.advance()

		params = append(params, ast.EffectParam{Key: keyTok.Literal, Value: value})
	}

	allowed, known := knownEffectParams[effectName]
	if known {
		for _, param := range params {
			if _, ok := allowed[param.Key]; !ok {
				return p.errorf(effectTok, "unknown parameter %q for effect %q", param.Key, effectName)
			}
		}
	}

	inst.Effects = append(inst.Effects, ast.Effect{Name: effectName, Params: params, Known: known})
	return nil
}

func (p *parser) parseSection() error {
	tok := p.advance()
	parts, err := p.collectLineValues()
	if err != nil {
		return err
	}
	if len(parts) == 0 {
		return p.errorf(tok, "Section requires a name")
	}

	section := ast.Section{Name: strings.Join(parts, " ")}
	p.file.Sections = append(p.file.Sections, section)
	p.currentSection = len(p.file.Sections) - 1
	return nil
}

func (p *parser) parseBar() error {
	barTok := p.advance()
	if p.currentSection < 0 {
		return p.errorf(barTok, "bar declared before any Section")
	}

	idTok := p.current()
	var id ast.BarID
	switch idTok.Type {
	case lexer.TokenInt:
		n, err := strconv.Atoi(idTok.Literal)
		if err != nil || n < 0 {
			return p.errorf(idTok, "invalid numeric bar ID")
		}
		id = ast.BarID{Kind: ast.BarIDNumber, Number: n}
		p.advance()
	default:
		if !isWordToken(idTok.Type) {
			return p.errorf(idTok, "bar ID must be number or identifier")
		}
		id = ast.BarID{Kind: ast.BarIDText, Text: idTok.Literal}
		p.advance()
	}

	if _, err := p.expect(lexer.TokenLBrace, "expected '{' after bar ID"); err != nil {
		return err
	}

	bar := ast.Bar{ID: id}
	for {
		p.skipNewlines()
		if p.current().Type == lexer.TokenRBrace {
			p.advance()
			break
		}
		if p.current().Type == lexer.TokenEOF {
			return p.errorf(p.current(), "unterminated bar block")
		}

		event, err := p.parseEvent()
		if err != nil {
			return err
		}
		bar.Events = append(bar.Events, event)
	}

	p.file.Sections[p.currentSection].Bars = append(p.file.Sections[p.currentSection].Bars, bar)
	return nil
}

func (p *parser) parseEvent() (ast.Event, error) {
	durTok := p.current()
	if !isWordToken(durTok.Type) {
		return ast.Event{}, p.errorf(durTok, "expected event duration")
	}
	dur, ok := durations[durTok.Literal]
	if !ok {
		return ast.Event{}, p.errorf(durTok, "unknown duration %q", durTok.Literal)
	}
	p.advance()

	if _, err := p.expect(lexer.TokenColon, "expected ':' after duration"); err != nil {
		return ast.Event{}, err
	}

	if p.current().Type == lexer.TokenLBracket {
		chord, err := p.parseChord()
		if err != nil {
			return ast.Event{}, err
		}
		event := ast.Event{Duration: dur, Kind: ast.EventChord, Chord: chord}
		if err := p.expectEventBoundary(); err != nil {
			return ast.Event{}, err
		}
		return event, nil
	}

	if isWordToken(p.current().Type) && p.current().Literal == "n" {
		p.advance()
		event := ast.Event{Duration: dur, Kind: ast.EventRest}
		if err := p.expectEventBoundary(); err != nil {
			return ast.Event{}, err
		}
		return event, nil
	}

	note, err := p.parseNote()
	if err != nil {
		return ast.Event{}, err
	}

	event := ast.Event{Duration: dur, Kind: ast.EventNote, Note: &note}
	if isWordToken(p.current().Type) {
		if isTechniqueToken(p.current().Literal) {
			tech, err := p.parseTechnique()
			if err != nil {
				return ast.Event{}, err
			}
			event.Kind = ast.EventTechnique
			event.Technique = &tech
		}
	}

	if err := p.expectEventBoundary(); err != nil {
		return ast.Event{}, err
	}
	return event, nil
}

func (p *parser) parseChord() ([]ast.Note, error) {
	if _, err := p.expect(lexer.TokenLBracket, "expected '[' to start chord"); err != nil {
		return nil, err
	}

	notes := make([]ast.Note, 0, 4)
	for {
		if p.current().Type == lexer.TokenRBracket {
			if len(notes) == 0 {
				return nil, p.errorf(p.current(), "chord cannot be empty")
			}
			p.advance()
			return notes, nil
		}
		if p.current().Type == lexer.TokenEOF || p.current().Type == lexer.TokenNewline {
			return nil, p.errorf(p.current(), "unterminated chord")
		}

		note, err := p.parseNote()
		if err != nil {
			return nil, err
		}
		notes = append(notes, note)
	}
}

func (p *parser) parseNote() (ast.Note, error) {
	strTok := p.current()
	if !isWordToken(strTok.Type) {
		return ast.Note{}, p.errorf(strTok, "expected note string token")
	}
	if !strings.HasPrefix(strTok.Literal, "s") || len(strTok.Literal) < 2 {
		return ast.Note{}, p.errorf(strTok, "invalid note format %q", strTok.Literal)
	}
	stringNum, err := strconv.Atoi(strTok.Literal[1:])
	if err != nil || stringNum < 1 || stringNum > 6 {
		return ast.Note{}, p.errorf(strTok, "string number must be 1..6")
	}
	p.advance()

	if _, err := p.expect(lexer.TokenComma, "expected ',' in note"); err != nil {
		return ast.Note{}, err
	}

	fretTok := p.current()
	if fretTok.Type != lexer.TokenInt {
		return ast.Note{}, p.errorf(fretTok, "fret must be a non-negative integer")
	}
	fret, err := strconv.Atoi(fretTok.Literal)
	if err != nil || fret < 0 {
		return ast.Note{}, p.errorf(fretTok, "fret must be a non-negative integer")
	}
	p.advance()

	return ast.Note{String: stringNum, Fret: fret}, nil
}

func (p *parser) parseTechnique() (ast.Technique, error) {
	techTok := p.current()
	alias := techTok.Literal
	if !isTechniqueToken(alias) {
		return ast.Technique{}, p.errorf(techTok, "unknown technique %q", alias)
	}
	p.advance()

	usesToKeyword := false
	hasExplicitTarget := false
	if isWordToken(p.current().Type) && p.current().Literal == "to" {
		usesToKeyword = true
		hasExplicitTarget = true
		p.advance()
	} else if p.current().Type == lexer.TokenInt {
		hasExplicitTarget = true
	}

	kind, ok := resolveTechniqueAlias(alias, hasExplicitTarget)
	if !ok {
		return ast.Technique{}, p.errorf(techTok, "unknown technique %q", alias)
	}

	tech := ast.Technique{Kind: kind}
	_, requiresTarget := techniquesRequireTarget[kind]
	if requiresTarget {
		targetTok := p.current()
		if targetTok.Type != lexer.TokenInt {
			return ast.Technique{}, p.errorf(targetTok, "technique %q requires target fret", kind)
		}
		target, err := strconv.Atoi(targetTok.Literal)
		if err != nil || target < 0 {
			return ast.Technique{}, p.errorf(targetTok, "target fret must be non-negative")
		}
		p.advance()
		tech.TargetFret = &target
		return tech, nil
	}

	if usesToKeyword {
		return ast.Technique{}, p.errorf(p.current(), "technique %q does not accept a target fret", kind)
	}
	if p.current().Type == lexer.TokenInt || p.current().Type == lexer.TokenFloat {
		return ast.Technique{}, p.errorf(p.current(), "technique %q does not accept a target fret", kind)
	}

	return tech, nil
}

func (p *parser) collectLineValues() ([]string, error) {
	parts := []string{}
	for !p.atLineBoundary() {
		tok := p.current()
		if !isValueToken(tok.Type) {
			return nil, p.errorf(tok, "unexpected token %s in line value", tok.Type)
		}
		parts = append(parts, tok.Literal)
		p.advance()
	}
	return parts, nil
}

func (p *parser) expectEventBoundary() error {
	tok := p.current()
	if tok.Type == lexer.TokenNewline || tok.Type == lexer.TokenRBrace || tok.Type == lexer.TokenEOF {
		return nil
	}
	return p.errorf(tok, "unexpected token %q after event", tok.Literal)
}

func (p *parser) expectLineBoundary(ctx string) error {
	tok := p.current()
	if tok.Type == lexer.TokenNewline || tok.Type == lexer.TokenEOF {
		return nil
	}
	return p.errorf(tok, "unexpected token %q after %s declaration", tok.Literal, ctx)
}

func (p *parser) atLineBoundary() bool {
	tok := p.current().Type
	return tok == lexer.TokenNewline || tok == lexer.TokenRBrace || tok == lexer.TokenEOF
}

func (p *parser) skipNewlines() {
	for p.current().Type == lexer.TokenNewline {
		p.advance()
	}
}

func (p *parser) expect(tt lexer.TokenType, msg string) (lexer.Token, error) {
	tok := p.current()
	if tok.Type != tt {
		return lexer.Token{}, &ParseError{Line: tok.Line, Column: tok.Column, Msg: msg}
	}
	p.advance()
	return tok, nil
}

func (p *parser) current() lexer.Token {
	if p.pos >= len(p.tokens) {
		return p.tokens[len(p.tokens)-1]
	}
	return p.tokens[p.pos]
}

func (p *parser) advance() lexer.Token {
	tok := p.current()
	if p.pos < len(p.tokens)-1 {
		p.pos++
	}
	return tok
}

func (p *parser) errorf(tok lexer.Token, format string, args ...any) error {
	return &ParseError{Line: tok.Line, Column: tok.Column, Msg: fmt.Sprintf(format, args...)}
}

func isGuitarType(s string) bool {
	return s == string(ast.GuitarClassical) || s == string(ast.GuitarAcoustic) || s == string(ast.GuitarElectric)
}

func isWordToken(tt lexer.TokenType) bool {
	switch tt {
	case lexer.TokenIdent, lexer.TokenName, lexer.TokenTempo, lexer.TokenTime, lexer.TokenSection, lexer.TokenBar:
		return true
	default:
		return false
	}
}

func isValueToken(tt lexer.TokenType) bool {
	return isWordToken(tt) || tt == lexer.TokenInt || tt == lexer.TokenFloat
}

func isTechniqueToken(lit string) bool {
	_, ok := techniqueAliases[lit]
	return ok
}

func resolveTechniqueAlias(alias string, hasTarget bool) (ast.TechniqueKind, bool) {
	kind, ok := techniqueAliases[alias]
	if !ok {
		return "", false
	}

	// "hm" is accepted as both hammer and harmonic:
	// with a target fret => hammer, otherwise => harmonic.
	if alias == "hm" {
		if hasTarget {
			return ast.TechniqueHammer, true
		}
		return ast.TechniqueHarmonic, true
	}

	return kind, true
}
