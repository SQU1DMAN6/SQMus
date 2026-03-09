package lexer

import "fmt"

// TokenType identifies lexical token categories.
type TokenType string

const (
	TokenIllegal  TokenType = "ILLEGAL"
	TokenEOF      TokenType = "EOF"
	TokenNewline  TokenType = "NEWLINE"
	TokenIdent    TokenType = "IDENT"
	TokenInt      TokenType = "INT"
	TokenFloat    TokenType = "FLOAT"
	TokenName     TokenType = "NAME"
	TokenTempo    TokenType = "TEMPO"
	TokenTime     TokenType = "TIME"
	TokenSection  TokenType = "SECTION"
	TokenBar      TokenType = "BAR"
	TokenLBrace   TokenType = "LBRACE"
	TokenRBrace   TokenType = "RBRACE"
	TokenLBracket TokenType = "LBRACKET"
	TokenRBracket TokenType = "RBRACKET"
	TokenComma    TokenType = "COMMA"
	TokenColon    TokenType = "COLON"
	TokenSlash    TokenType = "SLASH"
)

var keywords = map[string]TokenType{
	"NAME":    TokenName,
	"tempo":   TokenTempo,
	"tp":      TokenTempo,
	"time":    TokenTime,
	"Section": TokenSection,
	"b":       TokenBar,
}

// Token holds one lexical token with source position.
type Token struct {
	Type    TokenType
	Literal string
	Line    int
	Column  int
}

// LexError is returned for lexical failures.
type LexError struct {
	Line   int
	Column int
	Msg    string
}

func (e *LexError) Error() string {
	return fmt.Sprintf("lex error at %d:%d: %s", e.Line, e.Column, e.Msg)
}

// Lex tokenizes a SQMus source string.
func Lex(input string) ([]Token, error) {
	tokens := make([]Token, 0, len(input)/2)
	line := 1
	col := 1
	i := 0

	for i < len(input) {
		ch := input[i]

		switch ch {
		case ' ', '\t', '\r':
			i++
			col++
			continue
		case '\n':
			tokens = append(tokens, Token{Type: TokenNewline, Literal: "\\n", Line: line, Column: col})
			i++
			line++
			col = 1
			continue
		case '{':
			tokens = append(tokens, Token{Type: TokenLBrace, Literal: "{", Line: line, Column: col})
			i++
			col++
			continue
		case '}':
			tokens = append(tokens, Token{Type: TokenRBrace, Literal: "}", Line: line, Column: col})
			i++
			col++
			continue
		case '[':
			tokens = append(tokens, Token{Type: TokenLBracket, Literal: "[", Line: line, Column: col})
			i++
			col++
			continue
		case ']':
			tokens = append(tokens, Token{Type: TokenRBracket, Literal: "]", Line: line, Column: col})
			i++
			col++
			continue
		case ',':
			tokens = append(tokens, Token{Type: TokenComma, Literal: ",", Line: line, Column: col})
			i++
			col++
			continue
		case ':':
			tokens = append(tokens, Token{Type: TokenColon, Literal: ":", Line: line, Column: col})
			i++
			col++
			continue
		case '/':
			tokens = append(tokens, Token{Type: TokenSlash, Literal: "/", Line: line, Column: col})
			i++
			col++
			continue
		}

		if isDigit(ch) {
			start := i
			startCol := col

			for i < len(input) && isDigit(input[i]) {
				i++
				col++
			}

			typeID := TokenInt
			if i < len(input) && input[i] == '.' {
				if i+1 >= len(input) || !isDigit(input[i+1]) {
					return nil, &LexError{Line: line, Column: col, Msg: "malformed float literal"}
				}
				typeID = TokenFloat
				i++
				col++
				for i < len(input) && isDigit(input[i]) {
					i++
					col++
				}
			}

			tokens = append(tokens, Token{Type: typeID, Literal: input[start:i], Line: line, Column: startCol})
			continue
		}

		if isIdentStart(ch) {
			start := i
			startCol := col
			for i < len(input) && isIdentPart(input[i]) {
				i++
				col++
			}
			lit := input[start:i]
			typeID := TokenIdent
			if kw, ok := keywords[lit]; ok {
				typeID = kw
			}
			tokens = append(tokens, Token{Type: typeID, Literal: lit, Line: line, Column: startCol})
			continue
		}

		return nil, &LexError{Line: line, Column: col, Msg: fmt.Sprintf("illegal character %q", ch)}
	}

	tokens = append(tokens, Token{Type: TokenEOF, Literal: "", Line: line, Column: col})
	return tokens, nil
}

func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

func isIdentStart(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
}

func isIdentPart(ch byte) bool {
	return isIdentStart(ch) || isDigit(ch)
}
