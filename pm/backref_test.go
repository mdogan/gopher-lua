package pm

import "testing"

// The expected results are from Lua 5.1.
func TestFind_BackReferences(t *testing.T) {
	for _, c := range []struct {
		p, src string
		offset int
		want   string // the match, "" for no match, or the error
	}{
		{"(ab)%1", "abab", 0, "abab"},
		{"(a*)b%1", "aabaa", 0, "aabaa"},
		// A position capture is never equal to the text.
		{"()ab%1", "abab", 0, ""},
		// A capture that is not closed or not started yet is an error.
		{"(a(b%1))", "ab", 0, "invalid capture index"},
		{"%1(a)", "aa", 0, "invalid capture index"},
		{"(a)%2", "aa", 0, "invalid capture index"},
		// ... but only when the matcher reaches it.
		{"z(a%1)", "xy", 0, ""},
		// These ran into Go slice panics.
		{"[a-c]+[a-c]()%1?$", "ab1bb2aba", 3, ""},
		{"[^a]*(((%s*%%?%1%bab)%d(%a-[^a]*[ab]?))%bab-)%d", "b22cc", 0, "invalid capture index"},
	} {
		mds, err := Find(c.p, []byte(c.src), c.offset, 1)
		got := ""
		switch {
		case err != nil:
			got = err.Error()
		case len(mds) == 1:
			got = c.src[mds[0].Capture(0):mds[0].Capture(1)]
		}
		if got != c.want {
			t.Errorf("%q on %q: got %q, want %q", c.p, c.src, got, c.want)
		}
	}
}
