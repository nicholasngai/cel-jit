package celjit

import (
	"fmt"
	"io"
	"math"
	"strings"
)

func freindentf(w io.Writer, format string, a ...any) (int, error) {
	return freindentfLevel(w, 1, format, a...)
}

func freindentfLevel(w io.Writer, level int, format string, a ...any) (int, error) {
	// Trim off leading newline if present.
	if len(format) > 0 && format[0] == '\n' {
		format = format[1:]
	}

	commonIndentLevel := math.MaxInt
	for line := range strings.SplitSeq(format, "\n") {
		indentLevel := 0
		for _, c := range line {
			if c != '\t' {
				break
			}
		}
		commonIndentLevel = min(commonIndentLevel, indentLevel)
	}

	var b strings.Builder
	for line := range strings.SplitSeq(format, "\n") {
		for range level {
			b.WriteRune('\t')
		}
		fmt.Fprintf(&b, "%s\n", line[commonIndentLevel:])
	}
	newFormat := b.String()

	return fmt.Fprintf(w, newFormat, a...)
}
