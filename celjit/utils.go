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
		if line == "" {
			continue
		}
		indentLevel := 0
		for _, c := range line {
			if c != '\t' {
				break
			}
			indentLevel++
		}
		commonIndentLevel = min(commonIndentLevel, indentLevel)
	}

	var b strings.Builder
	first := true
	for line := range strings.SplitSeq(format, "\n") {
		if !first {
			b.WriteRune('\n')
		} else {
			first = false
		}

		if line == "" {
			continue
		}
		trimmedLine := line[commonIndentLevel:]
		if trimmedLine == "" {
			continue
		}

		for range level {
			b.WriteRune('\t')
		}
		b.WriteString(line[commonIndentLevel:])
	}
	newFormat := b.String()

	return fmt.Fprintf(w, newFormat, a...)
}

func repeat[T any](format string, vals []T, mapper func(int, T) []any) string {
	var builder strings.Builder
	for i, val := range vals {
		if i > 0 {
			builder.WriteString(", ")
		}
		fmt.Fprintf(&builder, format, mapper(i, val)...)
	}
	return builder.String()
}

func repeatInt(format string, n int, mapper func(int) []any) string {
	var builder strings.Builder
	for i := range n {
		if i > 0 {
			builder.WriteString(", ")
		}
		fmt.Fprintf(&builder, format, mapper(i)...)
	}
	return builder.String()
}
