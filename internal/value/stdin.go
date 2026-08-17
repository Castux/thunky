package value

import (
	"bufio"
	"io"
	"os"
	"unicode"

	"github.com/Castux/thunky/internal/source"
)

var (
	stdinReader  *bufio.Reader
	stdinStream  Value
	bstdinStream Value
)

func getStdinReader() *bufio.Reader {
	if stdinReader == nil {
		stdinReader = bufio.NewReader(os.Stdin)
	}
	return stdinReader
}

// makeInputStream builds the head of a shared, lazy cons-list backed by standard
// input. The head is a thunk that, when forced, calls readCell to read one item:
// readCell returns the item as a number, or ok = false at end of input. A read
// item becomes a cons cell [item, tail] whose tail is another such thunk, so the
// stream is produced one cell at a time and each cell, once forced, is memoized.
func makeInputStream(readCell func() (float64, bool)) Value {
	var thunk *Thunk
	thunk = &Thunk{
		Read: func() Value {
			num, ok := readCell()
			if !ok {
				return EmptyTuple
			}
			tail := makeInputStream(readCell)
			return ConsValue(NumberValue(num), tail)
		},
	}
	return ThunkValue(thunk)
}

// StdinCodePoints returns stdin: the standard input decoded as a lazy list of
// Unicode code points. A byte sequence that is not valid UTF-8 is a runtime error.
func StdinCodePoints() Value {
	if stdinStream.Ref == nil {
		stdinStream = makeInputStream(func() (float64, bool) {
			r, size, err := getStdinReader().ReadRune()
			if err == io.EOF {
				return 0, false
			}
			if err != nil {
				panic(&source.RuntimeError{Message: "error reading standard input: " + err.Error()})
			}
			// ReadRune yields U+FFFD with size 1 for invalid UTF-8 bytes.
			if r == unicode.ReplacementChar && size == 1 {
				panic(&source.RuntimeError{Message: "invalid UTF-8 byte on standard input"})
			}
			return float64(r), true
		})
	}
	return stdinStream
}

// StdinBytes returns bstdin: the standard input as a lazy list of raw byte values.
func StdinBytes() Value {
	if bstdinStream.Ref == nil {
		bstdinStream = makeInputStream(func() (float64, bool) {
			b, err := getStdinReader().ReadByte()
			if err == io.EOF {
				return 0, false
			}
			if err != nil {
				panic(&source.RuntimeError{Message: "error reading standard input: " + err.Error()})
			}
			return float64(b), true
		})
	}
	return bstdinStream
}
