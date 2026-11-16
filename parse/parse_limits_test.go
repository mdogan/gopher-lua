package parse

import (
	"strings"
	"testing"
)

func TestParse_TooManySyntaxLevels(t *testing.T) {
	for name, src := range map[string]string{
		// 5M nested unary operators used to grow the parser stack to
		// gigabytes before the compiler overflowed the Go stack.
		"unary":    "return " + strings.Repeat("# ", 5_000_000) + "'x'",
		"not":      "return " + strings.Repeat("not ", 2000) + "1",
		"parens":   "return " + strings.Repeat("(", 2000) + "1" + strings.Repeat(")", 2000),
		"concat":   "return " + strings.Repeat("a..", 2000) + "a",
		"blocks":   strings.Repeat("do ", 2000) + strings.Repeat("end ", 2000),
		"function": strings.Repeat("local f = function() ", 2000) + strings.Repeat("end ", 2000),
		"tables":   "return " + strings.Repeat("{", 2000) + strings.Repeat("}", 2000),
	} {
		_, err := Parse(strings.NewReader(src), name)
		if err == nil || !strings.Contains(err.Error(), "chunk has too many syntax levels") {
			t.Errorf("%s: expected too many syntax levels, got %v", name, err)
		}
	}
}

func TestParse_LongFlatCode(t *testing.T) {
	// Long lists and left associative chains reduce as they are parsed,
	// so they do not count as nesting.
	for name, src := range map[string]string{
		"statements": strings.Repeat("local a = 1\n", 100_000),
		"table":      "return {" + strings.Repeat("1,", 100_000) + "1}",
		"arguments":  "return f(" + strings.Repeat("1,", 100_000) + "1)",
		"sum":        "return " + strings.Repeat("a+", 100_000) + "a",
		"or chain":   "return " + strings.Repeat("a == 1 or ", 100_000) + "a",
		"elseifs":    "if x then " + strings.Repeat("elseif x then ", 10_000) + "end",
		// Nesting that real code uses is far below the limit.
		"nesting": strings.Repeat("do ", 100) + "return " + strings.Repeat("(", 100) + "1" +
			strings.Repeat(")", 100) + strings.Repeat(" end", 100),
	} {
		if _, err := Parse(strings.NewReader(src), name); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}
