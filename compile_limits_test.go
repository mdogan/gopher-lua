package lua

import (
	"strings"
	"testing"
)

func TestCompile_TooManySyntaxLevels(t *testing.T) {
	// The parser does not count left associative chains as nesting, but
	// they compile recursively: a million terms overflowed the Go stack.
	for name, src := range map[string]string{
		"sum":      "return " + strings.Repeat("a+", 1_000_000) + "a",
		"constant": "return " + strings.Repeat("1+", 1_000_000) + "1",
		"or chain": "return " + strings.Repeat("a == 1 or ", 20_000) + "a",
		"and if":   "if " + strings.Repeat("a and ", 20_000) + "a then end",
	} {
		L := NewState()
		err := L.DoString(src)
		L.Close()
		if err == nil || !strings.Contains(err.Error(), "chunk has too many syntax levels") {
			t.Errorf("%s: expected too many syntax levels, got %v", name, err)
		}
	}
}

func TestCompile_LongChains(t *testing.T) {
	L := NewState()
	defer L.Close()

	// These compiled in quadratic time: each level walked the rest of the
	// chain again.
	err := L.DoString(`
		local function check(want, got) assert(got == want, tostring(got)) end
		check(10002, (loadstring("a = 2 return " .. string.rep("a + ", 5000) .. "a"))())
		check(10002, (loadstring("return " .. string.rep("2 + ", 5000) .. "2"))())
		check(true, (loadstring("a = 3 return " .. string.rep("a == 1 or ", 5000) .. "a == 3"))())
		check(string.rep("ab", 151), (loadstring("return " .. string.rep("'ab' .. ", 150) .. "'ab'"))())
	`)
	if err != nil {
		t.Fatal(err)
	}
}
