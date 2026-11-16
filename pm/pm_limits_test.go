package pm

import (
	"strings"
	"testing"
)

func TestFind_TooManyCaptures(t *testing.T) {
	src := []byte(strings.Repeat("a", 40))

	// 32 captures, nested or side by side, are allowed.
	for _, p := range []string{
		strings.Repeat("(", 32) + "a" + strings.Repeat(")", 32),
		strings.Repeat("(a)", 32),
		strings.Repeat("()", 32),
	} {
		mds, err := Find(p, src, 0, 1)
		if err != nil || len(mds) != 1 {
			t.Fatalf("pattern %q: matches %d, err %v", p, len(mds), err)
		}
	}

	// One more fails, and a deep nesting fails before it recurses deeply:
	// ten million levels used to be a fatal stack overflow.
	for _, p := range []string{
		strings.Repeat("(", 33) + "a" + strings.Repeat(")", 33),
		strings.Repeat("(a)", 33),
		strings.Repeat("()", 33),
		strings.Repeat("(", 10_000_000),
	} {
		_, err := Find(p, src, 0, 1)
		if err == nil || !strings.Contains(err.Error(), "too many captures") {
			t.Fatalf("pattern of %d bytes: expected too many captures, got %v", len(p), err)
		}
	}
}

func TestFind_RepeatsDoNotRecursePerByte(t *testing.T) {
	// A repeat used to recurse once per matched byte, about 280 bytes of
	// stack each, up to a million levels.
	src := []byte(" " + strings.Repeat("a b", 2_000_000) + " ")
	for _, p := range []string{"a*", "^%s*(.-)%s*$", "^ [ab ]+ $", "^[^x]-$"} {
		mds, err := Find(p, src, 0, 1)
		if err != nil || len(mds) != 1 {
			t.Fatalf("pattern %q: matches %d, err %v", p, len(mds), err)
		}
	}
	mds, err := Find("^%s*(.-)%s*$", src, 0, 1)
	if err != nil || mds[0].Capture(2) != 1 || mds[0].Capture(3) != len(src)-1 {
		t.Fatalf("trim: %v %v", mds, err)
	}
}

func TestFind_RepeatSemantics(t *testing.T) {
	for _, c := range []struct {
		p, src  string
		capture string // capture 1, or the whole match without captures
	}{
		{"a*", "aaab", "aaa"},
		{"a-", "aaab", ""},
		{"a-b", "aaab", "aaab"},
		{"(a*)a", "aaa", "aa"},
		{"(a-)a", "aaa", ""},
		{"(a+)", "baab", "aa"},
		{"(a+)$", "aa", "aa"},
		{"(%d+)%.(%d-)0*$", "12.3400", "12"},
		{"<(.-)>", "<a><b>", "a"},
		{"<(.*)>", "<a><b>", "a><b"},
		{"(x*)", "", ""},
		{"[%a_][%w_]*", "  foo_1 = 2", "foo_1"},
	} {
		mds, err := Find(c.p, []byte(c.src), 0, 1)
		if err != nil || len(mds) != 1 {
			t.Fatalf("pattern %q on %q: matches %d, err %v", c.p, c.src, len(mds), err)
		}
		i := 0
		if mds[0].CaptureLength() > 2 {
			i = 2
		}
		if got := c.src[mds[0].Capture(i):mds[0].Capture(i+1)]; got != c.capture {
			t.Errorf("pattern %q on %q: got %q, want %q", c.p, c.src, got, c.capture)
		}
	}
	if mds, err := Find("a+", []byte("bbb"), 0, 1); err != nil || len(mds) != 0 {
		t.Fatalf("a+ on bbb: %v %v", mds, err)
	}
}

func TestFind_PatternTooComplex(t *testing.T) {
	// The depth now grows with the number of repeat items in the pattern.
	src := []byte(strings.Repeat("a", 2000))
	if _, err := Find(strings.Repeat("a?", 900), src, 0, 1); err != nil {
		t.Fatal(err)
	}
	_, err := Find(strings.Repeat("a?", 2000), src, 0, 1)
	if err == nil || err.Error() != "pattern too complex" {
		t.Fatalf("expected pattern too complex, got %v", err)
	}
}

func TestMatcher_SameAsFind(t *testing.T) {
	for _, c := range []struct {
		p, src string
		offset int
	}{
		{"a", "banana", 0},
		{"", "abc", 0},
		{"a*", "baaac", 0},
		{"(a)(n?)", "banana", 2},
		{"()a", "banana", 0},
		{"^a", "aaa", 0},
		{"^a", "aaa", 1},
		{"x", "abc", 0},
		{"", "", 0},
		{"a", "aaa", 3},
		{"a", "aaa", 4},
	} {
		want, err := Find(c.p, []byte(c.src), c.offset, -1)
		if err != nil {
			t.Fatal(err)
		}
		pat, err := Compile(c.p)
		if err != nil {
			t.Fatal(err)
		}
		m := pat.NewMatcher([]byte(c.src), c.offset)
		for i := 0; ; i++ {
			md, err := m.NextContext(nil)
			if err != nil {
				t.Fatal(err)
			}
			if md == nil {
				if i != len(want) {
					t.Fatalf("%q on %q: %d matches, want %d", c.p, c.src, i, len(want))
				}
				break
			}
			if i >= len(want) || md.Capture(0) != want[i].Capture(0) || md.Capture(1) != want[i].Capture(1) {
				t.Fatalf("%q on %q: match %d is [%d, %d]", c.p, c.src, i, md.Capture(0), md.Capture(1))
			}
		}
		// It stays done.
		if md, err := m.NextContext(nil); md != nil || err != nil {
			t.Fatalf("%q on %q: match after the end: %v %v", c.p, c.src, md, err)
		}
	}
	if _, err := Compile("(a"); err == nil || !strings.Contains(err.Error(), "unfinished capture") {
		t.Fatalf("expected unfinished capture, got %v", err)
	}
}

func TestFind_LimitZero(t *testing.T) {
	// A limit of 0 used to find every match when the first try matched.
	mds, err := Find("a", []byte("aaa"), 0, 0)
	if err != nil || len(mds) != 0 {
		t.Fatalf("got %d matches, err %v", len(mds), err)
	}
}
