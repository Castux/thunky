package source

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode"
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

// LineCol returns the 1-based line and column of a span's start, and the offset
// at which that line begins.
func (loc SourcePos) LineCol() (line, column, lineStart int) {
	if loc.File == nil {
		return 0, 0, 0
	}
	breaks := reLineBreak.FindAllStringIndex(loc.File.Text[:loc.Start], -1)
	lineStart = 0
	if len(breaks) > 0 {
		lineStart = breaks[len(breaks)-1][0] + 1
	}
	return len(breaks) + 1, loc.Start - lineStart + 1, lineStart
}

// Log prints a located diagnostic: the path/line/column, the message, the
// offending source line, and an underline of the span, all colored by severity.
func Log(msg string, loc SourcePos, severity Severity) {
	text := loc.File.Text

	lineNumber, columnNumber, lineStart := loc.LineCol()
	lineIndex, column := lineNumber-1, columnNumber-1

	lineEnd := loc.Start
	nextBreak := reLineBreak.FindStringIndex(text[loc.Start:])
	if len(nextBreak) != 0 {
		lineEnd += nextBreak[0]
	}

	fmt.Printf("%s:%d:%d: %s\n", loc.File.Path, lineIndex+1, column+1, msg)

	line := text[lineStart:lineEnd]
	colorEnd := min(len(line), column+loc.Length)
	coloredLine := line[:column] +
		colorText(line[column:colorEnd], colors[severity]) +
		line[colorEnd:]

	underline := strings.Map(toSpace, line[:column]) +
		colorText(strings.Repeat("^", colorEnd-column), colors[severity])

	fmt.Println(coloredLine)
	fmt.Println(underline)
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
