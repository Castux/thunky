package source

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// reLineBreak splits lines for diagnostic rendering in Log.
var reLineBreak = regexp.MustCompile(`\n\r?`)

// A Source is one loaded source file: its path and full text. Spans (SourcePos)
// point into it.
type Source struct {
	Path string
	Text string
}

// A SourcePos is a span within a source file: a start offset and length into
// File.Text. The zero value (nil File) means "no known location".
type SourcePos struct {
	File   *Source
	Start  int
	Length int
}

type Severity string

const (
	SeverityError Severity = "error"
	SeverityInfo  Severity = "info"
)

var colors = map[Severity]int{
	SeverityError: 31, // Red
	SeverityInfo:  34, // Blue
}

// colorEnabled is true when stderr is an interactive terminal and NO_COLOR is unset.
var colorEnabled = func() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	fi, err := os.Stderr.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}()

func colorText(txt string, color int) string {
	if !colorEnabled {
		return txt
	}
	return fmt.Sprintf("\x1b[%d;1m%s\x1b[0m", color, txt)
}

func toSpace(r rune) rune {
	if !unicode.IsSpace(r) {
		return ' '
	}
	return r
}

// LineCol returns the 1-based line and column of a span's start, and the byte
// offset at which that line begins.
//
// The column is counted in characters, not bytes. Source lines hold arbitrary
// UTF-8 — string literals, comments, and in this language even the file
// extension — and every caller of this is printing a position for a human to
// find, so a byte count would send them to the wrong place.
func (loc SourcePos) LineCol() (line, column, lineStart int) {
	if loc.File == nil {
		return 0, 0, 0
	}
	text := loc.File.Text
	breaks := reLineBreak.FindAllStringIndex(text[:loc.Start], -1)
	lineStart = 0
	if len(breaks) > 0 {
		lineStart = breaks[len(breaks)-1][0] + 1
	}
	return len(breaks) + 1, utf8.RuneCountInString(text[lineStart:loc.Start]) + 1, lineStart
}

// Log prints a located diagnostic: the path/line/column, the message, the
// offending source line, and an underline of the span, all colored by severity.
//
// Diagnostics go to stderr, where the color detection already looks, so that a
// program's own output (show/write/peek, on stdout) can be redirected on its own
// without ANSI escapes and compiler noise landing in the file.
func Log(msg string, loc SourcePos, severity Severity) {
	// The zero SourcePos has no file. Nothing reaches Log with one today, but
	// reporting the message unlocated beats dereferencing nil.
	if loc.File == nil {
		fmt.Fprintln(os.Stderr, msg)
		return
	}

	text := loc.File.Text

	lineNumber, column, lineStart := loc.LineCol()

	lineEnd := loc.Start
	nextBreak := reLineBreak.FindStringIndex(text[loc.Start:])
	if len(nextBreak) != 0 {
		lineEnd += nextBreak[0]
	}

	// A SourcePos is a byte span, so slicing the line needs byte offsets, but the
	// caret run is drawn in characters — for the same reason LineCol counts the
	// column in characters.
	line := text[lineStart:lineEnd]
	byteCol := loc.Start - lineStart
	byteEnd := min(len(line), byteCol+loc.Length)
	width := utf8.RuneCountInString(line[byteCol:byteEnd])

	fmt.Fprintf(os.Stderr, "%s:%d:%d: %s\n", loc.File.Path, lineNumber, column, msg)

	coloredLine := line[:byteCol] +
		colorText(line[byteCol:byteEnd], colors[severity]) +
		line[byteEnd:]

	underline := strings.Map(toSpace, line[:byteCol]) +
		colorText(strings.Repeat("^", width), colors[severity])

	fmt.Fprintln(os.Stderr, coloredLine)
	fmt.Fprintln(os.Stderr, underline)
}

// To returns the span from the start of a to the end of b. Both must be in the
// same file.
func (a SourcePos) To(b SourcePos) SourcePos {
	if a.File != b.File {
		panic("Cannot merge SourcePos from different files")
	}

	return SourcePos{
		File:   a.File,
		Start:  a.Start,
		Length: b.Start + b.Length - a.Start,
	}
}
