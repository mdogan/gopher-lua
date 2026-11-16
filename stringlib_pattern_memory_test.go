package lua

import (
	"strings"
	"testing"
)

func TestStringGmatch_CountsCompiledPattern(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.SetMemoryLimit(1 << 20)

	// Each iterator keeps its compiled pattern, about 200 KB for 4 KB of
	// pattern. 200 of them held 48 MB while 64 KB was counted.
	err := L.DoString(`
		local p = string.rep("a", 4096)
		its = {}
		for i = 1, 200 do its[i] = string.gmatch("x", p) end
	`)
	if err == nil || !strings.Contains(err.Error(), "memory limit exceeded") {
		t.Fatalf("expected the memory limit error, got %v", err)
	}
}

// The memory counter never goes down, so what gmatch counts on every call
// adds up in a loop. For "%a+" it counted 1368 bytes per call, 1024 of them
// a fixed part shared with CompileSize; the iterator holds about 600.
func TestStringGmatch_CountsLittlePerCall(t *testing.T) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`s = "key=value,other=thing"`); err != nil {
		t.Fatal(err)
	}
	L.ResetMemoryUsage()
	const calls = 10000
	if err := L.DoString(`for i = 1, 10000 do local it = s:gmatch("%a+") end`); err != nil {
		t.Fatal(err)
	}
	if perCall := L.GetAllocatedBytes() / calls; perCall > 800 {
		t.Fatalf("gmatch counted %d bytes per call", perCall)
	}
}

func TestStringPatterns_CheckCompileMemory(t *testing.T) {
	for _, call := range []string{
		`string.find("x", p)`,
		`string.match("x", p)`,
		`string.gsub("x", p, "")`,
		`string.gmatch("x", p)`,
	} {
		L := NewState()
		L.SetMemoryLimit(8 << 20)
		// Compiling 100 KB of pattern can allocate about 15 MB.
		err := L.DoString(`local p = string.rep("a", 100000) ` + call)
		if err == nil || !strings.Contains(err.Error(), "memory limit exceeded") {
			t.Errorf("%s: expected the memory limit error, got %v", call, err)
		}
		L.Close()
	}

	// Only gmatch keeps the compiled pattern, so the others do not count it:
	// they can compile a 10 KB pattern any number of times.
	for _, call := range []string{
		`string.find("x", p)`,
		`string.match("x", p)`,
		`string.gsub("x", p, "")`,
	} {
		L := NewState()
		L.SetMemoryLimit(8 << 20)
		if err := L.DoString(`local p = string.rep("a", 10000) for i = 1, 1000 do ` + call + ` end`); err != nil {
			t.Errorf("%s: %v", call, err)
		}
		L.Close()
	}
}
