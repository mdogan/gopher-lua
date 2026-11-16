package lua

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestStringGmatch_FindsMatchesOneAtATime(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.SetMemoryLimit(32 << 20)

	// 16M empty matches used to be found, and kept, before the first one
	// was returned.
	start := time.Now()
	err := L.DoString(`
		local s = string.rep("a", 16 * 1024 * 1024)
		for m in s:gmatch("") do break end
		local n = 0
		for m in s:gmatch("a") do n = n + 1 if n == 3 then break end end
		assert(n == 3)
	`)
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("gmatch took %v", d)
	}
}

func TestStringGmatch_Semantics(t *testing.T) {
	L := NewState()
	defer L.Close()
	err := L.DoString(`
		local function all(s, p)
			local out = {}
			for a, b in s:gmatch(p) do
				out[#out + 1] = b == nil and tostring(a) or tostring(a) .. "=" .. tostring(b)
			end
			return table.concat(out, ",")
		end
		local function check(want, got) assert(got == want, got) end
		check("hello,world,from,Lua", all("hello world from Lua", "%a+"))
		check("from=world,to=Lua", all("from=world, to=Lua", "(%w+)=(%w+)"))
		check(",,,", all("abc", "x*"))
		check("1,2,3,4", all("abc", "()"))
		check("a,,c,", all("abc", "[ac]*"))
		check("a", all("aaa", "^a"))
		check("", all("", "a"))
		check("", all("", "^a"))
		check("", all("bbb", "a"))

		-- gmatch returns one function, which keeps returning nil after the
		-- last match.
		assert(select("#", ("ab"):gmatch("%a")) == 1)
		local it = ("ab"):gmatch("%a")
		check("a", it()) check("b", it())
		assert(it() == nil and it() == nil)

		-- The iterators are independent.
		local i1, i2 = ("abc"):gmatch("."), ("abc"):gmatch(".")
		check("a", i1()) check("b", i1()) check("a", i2()) check("c", i1())

		-- A bad pattern fails when gmatch is called.
		local ok, err = pcall(string.gmatch, "abc", "(a")
		assert(not ok and err:find("unfinished capture"), err)
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestStringGmatch_StopsOnDeadline(t *testing.T) {
	L := NewState()
	defer L.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	L.SetContext(ctx)

	start := time.Now()
	err := L.DoString(`
		local s = string.rep("a", 30)
		for m in s:gmatch(string.rep("a*", 30) .. "b") do end
	`)
	if err == nil || !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("expected a deadline error, got %v", err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("took %v", d)
	}
}
