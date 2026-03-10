package lexer

import (
	"reflect"
	"testing"
)

func TestLexTokenShape(t *testing.T) {
	src := "NAME Hello World\ntp 92\ntime 4/4\nel {\n    tn std\n}\n"
	tokens, err := Lex(src)
	if err != nil {
		t.Fatalf("Lex() returned error: %v", err)
	}

	got := tokenTypes(tokens)
	want := []TokenType{
		TokenName, TokenIdent, TokenIdent, TokenNewline,
		TokenTempo, TokenInt, TokenNewline,
		TokenTime, TokenInt, TokenSlash, TokenInt, TokenNewline,
		TokenIdent, TokenLBrace, TokenNewline,
		TokenIdent, TokenIdent, TokenNewline,
		TokenRBrace, TokenNewline,
		TokenEOF,
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("token type mismatch\n got: %v\nwant: %v", got, want)
	}
}

func TestLexNumericLiterals(t *testing.T) {
	src := "drv g 0.6 t 1 m 0\n"
	tokens, err := Lex(src)
	if err != nil {
		t.Fatalf("Lex() returned error: %v", err)
	}

	got := tokenTypes(tokens)
	want := []TokenType{
		TokenIdent, TokenIdent, TokenFloat, TokenIdent, TokenInt, TokenIdent, TokenInt, TokenNewline, TokenEOF,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("token type mismatch\n got: %v\nwant: %v", got, want)
	}
}

func TestLexLineAndColumn(t *testing.T) {
	src := "tp 92\nq: s2,0\n"
	tokens, err := Lex(src)
	if err != nil {
		t.Fatalf("Lex() returned error: %v", err)
	}

	expectTokenPos(t, tokens[0], TokenTempo, "tp", 1, 1)
	expectTokenPos(t, tokens[1], TokenInt, "92", 1, 4)
	expectTokenPos(t, tokens[3], TokenIdent, "q", 2, 1)
	expectTokenPos(t, tokens[4], TokenColon, ":", 2, 2)
	expectTokenPos(t, tokens[5], TokenIdent, "s2", 2, 4)
	expectTokenPos(t, tokens[6], TokenComma, ",", 2, 6)
	expectTokenPos(t, tokens[7], TokenInt, "0", 2, 7)
}

func TestLexIllegalCharacter(t *testing.T) {
	_, err := Lex("tempo 92\n@\n")
	if err == nil {
		t.Fatal("expected lexical error, got nil")
	}
}

func TestLexSharpsAndFlatsInTuning(t *testing.T) {
	src := "tn F# Bb D#\n"
	tokens, err := Lex(src)
	if err != nil {
		t.Fatalf("Lex() returned error: %v", err)
	}

	got := tokenTypes(tokens)
	want := []TokenType{TokenIdent, TokenIdent, TokenIdent, TokenIdent, TokenNewline, TokenEOF}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("token type mismatch\n got: %v\nwant: %v", got, want)
	}
	if tokens[1].Literal != "F#" || tokens[2].Literal != "Bb" || tokens[3].Literal != "D#" {
		t.Fatalf("unexpected tuning token literals: %q %q %q", tokens[1].Literal, tokens[2].Literal, tokens[3].Literal)
	}
}

func tokenTypes(tokens []Token) []TokenType {
	out := make([]TokenType, 0, len(tokens))
	for _, tok := range tokens {
		out = append(out, tok.Type)
	}
	return out
}

func expectTokenPos(t *testing.T, tok Token, wantType TokenType, wantLit string, wantLine, wantCol int) {
	t.Helper()
	if tok.Type != wantType || tok.Literal != wantLit || tok.Line != wantLine || tok.Column != wantCol {
		t.Fatalf("unexpected token %+v; want type=%s lit=%q at %d:%d", tok, wantType, wantLit, wantLine, wantCol)
	}
}
