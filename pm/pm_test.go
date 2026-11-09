package pm

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// slowPattern backtracks for a very long time on a string of 'a's with no 'b'.
const slowPattern = "a*a*a*a*a*a*a*a*a*a*b"

func TestFindContextStopsOnDeadline(t *testing.T) {
	src := []byte(strings.Repeat("a", 40))
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	matches, err := FindContext(ctx, slowPattern, src, 0, 1)
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got err=%v matches=%d", err, len(matches))
	}
	if elapsed > 2*time.Second {
		t.Fatalf("matching took %v after the deadline", elapsed)
	}
}

func TestFindContextStopsOnCancel(t *testing.T) {
	src := []byte(strings.Repeat("x", 2000))
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)

	_, err := FindContext(ctx, ".-.-.-y", src, 0, -1)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestFindContextStopsDuringLongScans(t *testing.T) {
	// %b, back-references and character sets can do a lot of work inside
	// one VM instruction.
	cases := map[string]struct {
		pat string
		src []byte
	}{
		"brace":   {"%b()", []byte(strings.Repeat("(", 8<<20))},
		"backref": {"(%a+)%1b", []byte(strings.Repeat("a", 64<<10))},
		// A large set is checked entry by entry for every input byte.
		"set": {"[" + strings.Repeat("a", 1<<20) + "]", []byte(strings.Repeat("b", 1024))},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer cancel()

			start := time.Now()
			_, err := FindContext(ctx, c.pat, c.src, 0, 1)
			elapsed := time.Since(start)

			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("expected context.DeadlineExceeded, got %v", err)
			}
			if elapsed > 500*time.Millisecond {
				t.Fatalf("matching took %v with a 10ms deadline", elapsed)
			}
		})
	}
}

func TestFindContextAlreadyCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	matches, err := FindContext(ctx, "a", []byte("a"), 0, 1)
	if !errors.Is(err, context.Canceled) || matches != nil {
		t.Fatalf("expected context.Canceled and no matches, got err=%v matches=%d", err, len(matches))
	}
}

func TestFindContextSameResultsAsFind(t *testing.T) {
	cases := []struct {
		pat, src string
		limit    int
	}{
		{"(%w+)=(%w+)", "a=1, b=2, c=3", -1},
		{"^(.-),(.-)$", "left,right", 1},
		{"%bxy", "axxyyb", -1},
		{"()a()", "banana", -1},
		{"(a)%1", "xaay", 1},
		{"", "abc", -1},
	}
	for _, c := range cases {
		want, werr := Find(c.pat, []byte(c.src), 0, c.limit)
		got, gerr := FindContext(context.Background(), c.pat, []byte(c.src), 0, c.limit)
		if werr != nil || gerr != nil {
			t.Fatalf("%q: unexpected errors %v, %v", c.pat, werr, gerr)
		}
		if len(want) != len(got) {
			t.Fatalf("%q: %d matches vs %d", c.pat, len(want), len(got))
		}
		for i := range want {
			if want[i].CaptureLength() != got[i].CaptureLength() {
				t.Fatalf("%q: match %d capture length differs", c.pat, i)
			}
			for j := 0; j < want[i].CaptureLength(); j++ {
				if want[i].Capture(j) != got[i].Capture(j) {
					t.Fatalf("%q: match %d capture %d differs", c.pat, i, j)
				}
			}
		}
	}
}

func TestFindContextPatternErrorStillReported(t *testing.T) {
	_, err := FindContext(context.Background(), "(abc", []byte("abc"), 0, 1)
	var perr *Error
	if !errors.As(err, &perr) {
		t.Fatalf("expected *pm.Error, got %v", err)
	}
}

func BenchmarkFindNoContext(b *testing.B) {
	src := []byte(strings.Repeat("key=value;", 100))
	for i := 0; i < b.N; i++ {
		Find("(%w+)=(%w+)", src, 0, -1)
	}
}

func BenchmarkFindWithContext(b *testing.B) {
	src := []byte(strings.Repeat("key=value;", 100))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for i := 0; i < b.N; i++ {
		FindContext(ctx, "(%w+)=(%w+)", src, 0, -1)
	}
}
