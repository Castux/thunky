package syntax

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Castux/thunky/internal/source"
)

var (
	reWhitespace = regexp.MustCompile(`^\s+`)
	// A comment runs to the end of the line, or to the end of the file when the
	// last line has no terminator (a file — or an editor buffer — may end on a
	// comment with no trailing newline).
	reComment    = regexp.MustCompile(`^--[^\n\r]*(\r?\n|$)`)
	reIdentifier = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*`)
	reString     = regexp.MustCompile(`^('[^']*'|"[^"]*")`)
	reNumber     = regexp.MustCompile(`^\d+(\.\d+)?`)
)

type Token struct {
	Kind  string
	Value string
	Pos   source.SourcePos
}

// Number returns a number token's value. LexContent validates every literal it
// admits, so this cannot fail on a token the lexer produced.
func (t Token) Number() float64 {
	v, err := strconv.ParseFloat(t.Value, 64)
	if err != nil {
		panic("Token.Number: not a valid number literal: " + t.Value)
	}
	return v
}

var keywords = map[string]bool{
	"let":    true,
	"in":     true,
	"import": true,
	"module": true,
}

var symbols = []string{
	"->", "<*", "*>",
	">", "<", ".", "=", ",", ";",
	"(", ")", "{", "}", "[", "]",
}

func Lex(path string) []Token {
	text, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not read file: %s\n", path)
		return nil
	}
	return LexContent(path, string(text))
}

func LexContent(path string, content string) []Token {
	file := &source.Source{
		Path: path,
		Text: content,
	}

	var tokens []Token
	head := 0
	src := content

lexLoop:
	for head < len(src) {

		// 1. Consume whitespace
		if match := reWhitespace.FindString(src[head:]); len(match) > 0 {
			head += len(match)
			continue lexLoop
		}

		// 2. Consume comments
		if match := reComment.FindString(src[head:]); len(match) > 0 {
			head += len(match)
			continue lexLoop
		}

		// 3. Keywords and identifiers
		if match := reIdentifier.FindString(src[head:]); len(match) > 0 {
			pos := source.SourcePos{File: file, Start: head, Length: len(match)}
			token := Token{Value: match, Pos: pos}

			if keywords[match] {
				token.Kind = match
			} else {
				token.Kind = "identifier"
			}

			tokens = append(tokens, token)
			head += len(match)
			continue lexLoop
		}

		// 4. Symbols (Check for multi-character symbols first)
		for _, symbol := range symbols {
			if strings.HasPrefix(src[head:], symbol) {
				pos := source.SourcePos{File: file, Start: head, Length: len(symbol)}
				token := Token{Kind: symbol, Value: symbol, Pos: pos}

				tokens = append(tokens, token)
				head += len(symbol)
				continue lexLoop
			}
		}

		// 5. String literals
		if match := reString.FindString(src[head:]); len(match) > 0 {
			pos := source.SourcePos{File: file, Start: head, Length: len(match)}
			token := Token{Kind: "string", Value: match[1 : len(match)-1], Pos: pos}

			tokens = append(tokens, token)
			head += len(match)
			continue lexLoop
		}

		// 6. Number literals
		if match := reNumber.FindString(src[head:]); len(match) > 0 {
			pos := source.SourcePos{File: file, Start: head, Length: len(match)}

			// The literal is well-formed by construction (reNumber admits only
			// digits and one dot), so the only way ParseFloat can fail is
			// ErrRange — a literal too large for a float64. Catch it here, where
			// there is a position to report: Token.Number() has none, and its
			// assertion panic is not the parser's "expect" sentinel, so
			// Recover() would re-raise it as a Go stack dump.
			if _, err := strconv.ParseFloat(match, 64); err != nil {
				source.Log("number literal out of range for a 64-bit float", pos, source.SeverityError)
				return nil
			}

			token := Token{Kind: "number", Value: match, Pos: pos}

			tokens = append(tokens, token)
			head += len(match)
			continue lexLoop
		}

		// 7. Fail state. A quote here means reString found no closing one:
		// naming that beats reporting the quote as an unexpected character.
		if c := src[head]; c == '\'' || c == '"' {
			pos := source.SourcePos{File: file, Start: head, Length: 1}
			source.Log("unterminated string literal", pos, source.SeverityError)
			return nil
		}

		// Report the offending character as a whole rune: slicing one byte
		// would cut a multi-byte character into mojibake.
		r, size := utf8.DecodeRuneInString(src[head:])
		pos := source.SourcePos{File: file, Start: head, Length: size}
		if r == utf8.RuneError && size == 1 {
			source.Log(fmt.Sprintf("invalid UTF-8 byte 0x%02x", src[head]), pos, source.SeverityError)
		} else {
			source.Log(fmt.Sprintf("unexpected character '%c'", r), pos, source.SeverityError)
		}
		return nil
	}

	// Append "eof" token at the end of the file.
	eofLoc := source.SourcePos{File: file, Start: head, Length: 0}
	tokens = append(tokens, Token{Kind: "eof", Pos: eofLoc})

	return tokens
}
