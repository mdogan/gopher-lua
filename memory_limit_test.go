package lua

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"testing"
)

func TestMemoryLimit_TableAllocation(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Set a 10KB limit
	L.ResetMemoryUsage()
	L.SetMemoryLimit(10 * 1024)

	err := L.DoString(`
		local t = {}
		for i = 1, 100 do
			t[i] = i
		end
		return #t
	`)

	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}

	allocated := L.GetAllocatedBytes()
	if allocated == 0 {
		t.Error("Expected non-zero memory allocation")
	}

	// Create tables until we hit the limit
	err = L.DoString(`
		local tables = {}
		for i = 1, 1000 do
			tables[i] = {}
			for j = 1, 100 do
				tables[i][j] = j
			end
		end
	`)

	if err == nil {
		t.Fatal("Expected memory limit error, got nil")
	}

	if !strings.Contains(err.Error(), "memory limit exceeded") {
		t.Errorf("Expected 'memory limit exceeded' error, got: %v", err)
	}
}

func TestMemoryLimit_TableInsert(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetMemoryLimit(20 * 1024) // 20KB limit
	L.ResetMemoryUsage()

	script := `
		local t = {}
		for i = 1, 200 do
			table.insert(t, i)
		end
		return #t
	`

	if err := L.DoString(script); err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}

	allocated := L.GetAllocatedBytes()
	if allocated == 0 {
		t.Error("Expected non-zero memory allocation for table.insert")
	}
}

func TestMemoryLimit_ArrayGrowth(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetMemoryLimit(50 * 1024) // 50KB limit
	L.ResetMemoryUsage()

	script := `
		local t = {}
		for i = 1, 500 do
			t[i] = "value" .. i
		end
		return #t
	`

	if err := L.DoString(script); err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}

	allocated := L.GetAllocatedBytes()
	if allocated == 0 {
		t.Error("Expected non-zero memory allocation for array growth")
	}
}

func TestMemoryLimit_HashTable(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetMemoryLimit(30 * 1024) // 30KB limit
	L.ResetMemoryUsage()

	script := `
		local t = {}
		for i = 1, 100 do
			t["key" .. i] = "value" .. i
		end
		local count = 0
		for k, v in pairs(t) do
			count = count + 1
		end
		return count
	`

	if err := L.DoString(script); err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}

	allocated := L.GetAllocatedBytes()
	if allocated == 0 {
		t.Error("Expected non-zero memory allocation for hash table")
	}
}

func TestMemoryLimit_NestedTables(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetMemoryLimit(100 * 1024) // 100KB limit
	L.ResetMemoryUsage()

	script := `
		local t = {}
		for i = 1, 50 do
			t[i] = {}
			for j = 1, 50 do
				t[i][j] = i * j
			end
		end
		return #t
	`

	if err := L.DoString(script); err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}

	allocated := L.GetAllocatedBytes()
	if allocated == 0 {
		t.Error("Expected non-zero memory allocation for nested tables")
	}
}

func TestMemoryLimit_TableConcat(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetMemoryLimit(200 * 1024) // 200KB limit
	L.ResetMemoryUsage()

	script := `
		local t = {}
		for i = 1, 1000 do
			t[i] = "item" .. i
		end
		local result = table.concat(t, ",")
		return #result
	`

	if err := L.DoString(script); err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}

	allocated := L.GetAllocatedBytes()
	if allocated == 0 {
		t.Error("Expected non-zero memory allocation for table.concat")
	}
}

func TestMemoryLimit_StringConcat(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.ResetMemoryUsage()
	L.SetMemoryLimit(5 * 1024)

	// Concatenate strings until we hit the limit
	err := L.DoString(`
		local str = "a"
		for i = 1, 20 do
			str = str .. str  -- doubles each time
		end
	`)

	if err == nil {
		t.Fatalf("Expected memory limit error, got nil: %d", L.GetAllocatedBytes())
	}

	if !strings.Contains(err.Error(), "memory limit exceeded") {
		t.Errorf("Expected 'memory limit exceeded' error, got: %v", err)
	}
}

func TestMemoryLimit_StringRepeat(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.ResetMemoryUsage()
	L.SetMemoryLimit(5 * 1024)

	err := L.DoString(`
		local s = string.rep("x", 100)
		return #s
	`)

	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}

	allocated := L.GetAllocatedBytes()
	if allocated == 0 {
		t.Error("Expected non-zero memory allocation for string.rep")
	}

	// Concatenate strings until we hit the limit
	err = L.DoString(`
		local str = string.rep("a", 1024 * 10)
		return str
	`)

	if err == nil {
		t.Fatal("Expected memory limit error, got nil")
	}

	if !strings.Contains(err.Error(), "memory limit exceeded") {
		t.Errorf("Expected 'memory limit exceeded' error, got: %v", err)
	}
}

func TestMemoryLimit_GetUsage(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.ResetMemoryUsage()
	L.SetMemoryLimit(100 * 1024) // 100KB

	initialUsage := L.GetAllocatedBytes()
	if initialUsage != 0 {
		t.Errorf("Expected initial usage to be 0 after reset, got %d", initialUsage)
	}

	// Allocate some memory
	err := L.DoString(`
		local t = {}
		for i = 1, 100 do
			t[i] = i
		end
	`)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	usageAfter := L.GetAllocatedBytes()
	if usageAfter <= 0 {
		t.Errorf("Expected usage after allocation to be > 0, got %d", usageAfter)
	}
}

func TestMemoryLimit_ResetUsage(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.ResetMemoryUsage()
	L.SetMemoryLimit(100 * 1024) // 100KB

	// Allocate some memory
	err := L.DoString(`
		local t = {}
		for i = 1, 50 do
			t[i] = i
		end
	`)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	usageBefore := L.GetAllocatedBytes()
	if usageBefore <= 0 {
		t.Errorf("Expected usage to be > 0 before reset, got %d", usageBefore)
	}

	L.ResetMemoryUsage()

	usageAfter := L.GetAllocatedBytes()
	if usageAfter != 0 {
		t.Errorf("Expected usage to be 0 after reset, got %d", usageAfter)
	}

	// Should be able to allocate again after reset
	err = L.DoString(`
		local t = {}
		for i = 1, 50 do
			t[i] = i
		end
	`)

	if err != nil {
		t.Fatalf("Unexpected error after reset: %v", err)
	}
}

func TestMemoryLimit_NoLimit(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Should not fail even with large allocations
	err := L.DoString(`
		local t = {}
		for i = 1, 1000 do
			t[i] = {}
		end
	`)

	if err != nil {
		t.Fatalf("Unexpected error with no limit: %v", err)
	}

	usage := L.GetAllocatedBytes()
	if usage <= 0 {
		t.Errorf("Expected usage to be tracked even without limit, got %d", usage)
	}
}

func TestMemoryLimit_DisableLimit(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Reset usage after initialization, then set a very low limit
	L.ResetMemoryUsage()
	L.SetMemoryLimit(100)

	// Should fail quickly when adding values to table
	err := L.DoString(`
		local t = {}
		for i = 1, 10 do
			t[i] = i
		end
	`)
	if err == nil {
		t.Fatal("Expected memory limit error with low limit")
	}

	// Disable limit by setting to 0
	L.SetMemoryLimit(0)
	L.ResetMemoryUsage()

	// Should now succeed
	err = L.DoString(`
		local t = {}
		for i = 1, 100 do
			t[i] = i
		end
	`)

	if err != nil {
		t.Fatalf("Unexpected error after disabling limit: %v", err)
	}
}

func TestMemoryLimit_StringUpper(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.ResetMemoryUsage()
	L.SetMemoryLimit(5 * 1024) // 5KB limit

	err := L.DoString(`
		local s = string.rep("a", 1024)
		for i = 1, 10 do
			s = string.upper(s)  -- Each creates a copy
		end
	`)

	if err == nil {
		t.Fatal("Expected memory limit error, got nil")
	}

	if !strings.Contains(err.Error(), "memory limit exceeded") {
		t.Errorf("Expected 'memory limit exceeded' error, got: %v", err)
	}
}

func TestMemoryLimit_StringLower(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.ResetMemoryUsage()
	L.SetMemoryLimit(5 * 1024) // 5KB limit

	err := L.DoString(`
		local s = string.rep("A", 1024)
		for i = 1, 10 do
			s = string.lower(s)  -- Each creates a copy
		end
	`)

	if err == nil {
		t.Fatal("Expected memory limit error, got nil")
	}

	if !strings.Contains(err.Error(), "memory limit exceeded") {
		t.Errorf("Expected 'memory limit exceeded' error, got: %v", err)
	}
}

func TestMemoryLimit_StringReverse(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.ResetMemoryUsage()
	L.SetMemoryLimit(5 * 1024) // 5KB limit

	err := L.DoString(`
		local s = string.rep("abc", 300)
		for i = 1, 10 do
			s = string.reverse(s)  -- Each creates a copy
		end
	`)

	if err == nil {
		t.Fatal("Expected memory limit error, got nil")
	}

	if !strings.Contains(err.Error(), "memory limit exceeded") {
		t.Errorf("Expected 'memory limit exceeded' error, got: %v", err)
	}
}

func TestMemoryLimit_StringChar(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.ResetMemoryUsage()
	L.SetMemoryLimit(2 * 1024) // 2KB limit

	err := L.DoString(`
		local results = {}
		for i = 1, 1000 do
			results[i] = string.char(65, 66, 67, 68, 69, 70)  -- Creates small strings
		end
	`)

	if err == nil {
		t.Fatal("Expected memory limit error, got nil")
	}

	if !strings.Contains(err.Error(), "memory limit exceeded") {
		t.Errorf("Expected 'memory limit exceeded' error, got: %v", err)
	}
}

func TestMemoryLimit_StringFormat(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.ResetMemoryUsage()
	L.SetMemoryLimit(5 * 1024) // 5KB limit

	err := L.DoString(`
		local t = {}
		for i = 1, 10 do
			t[i] = string.format("Item %03d: value=%d, squared=%d", i, i*10, i*i)
		end
		return #t
	`)

	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}

	allocated := L.GetAllocatedBytes()
	if allocated == 0 {
		t.Error("Expected non-zero memory allocation for string.format")
	}

	err = L.DoString(`
		local results = {}
		for i = 1, 1000 do
			results[i] = string.format("%1000s", "x")  -- Creates 1000-byte strings
		end
	`)

	if err == nil {
		t.Fatal("Expected memory limit error, got nil")
	}

	if !strings.Contains(err.Error(), "memory limit exceeded") {
		t.Errorf("Expected 'memory limit exceeded' error, got: %v", err)
	}
}

func TestMemoryLimit_StringSub(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.ResetMemoryUsage()
	L.SetMemoryLimit(3 * 1024) // 3KB limit

	err := L.DoString(`
		local base = string.rep("x", 500)
		local results = {}
		for i = 1, 100 do
			results[i] = string.sub(base, 1, 400)  -- Creates 400-byte substrings
		end
	`)

	if err == nil {
		t.Fatal("Expected memory limit error, got nil")
	}

	if !strings.Contains(err.Error(), "memory limit exceeded") {
		t.Errorf("Expected 'memory limit exceeded' error, got: %v", err)
	}
}

func TestMemoryLimit_StringGsub(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.ResetMemoryUsage()
	L.SetMemoryLimit(3 * 1024)

	err := L.DoString(`
		local s = string.rep("a", 1000)
		s = string.gsub(s, "a", "bbb")  -- Triples size
	`)

	if err == nil {
		t.Fatal("Expected memory limit error, got nil")
	}

	if !strings.Contains(err.Error(), "memory limit exceeded") {
		t.Errorf("Expected 'memory limit exceeded' error, got: %v", err)
	}
}

func TestMemoryLimit_StringMatch(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.ResetMemoryUsage()
	L.SetMemoryLimit(3 * 1024) // 3KB limit

	err := L.DoString(`
		local base = string.rep("test123", 100)
		local results = {}
		for i = 1, 1000 do
			local cap1, cap2 = string.match(base, "(test)(%d+)")
			results[i] = cap1 .. cap2  -- Captured strings
		end
	`)

	if err == nil {
		t.Fatal("Expected memory limit error, got nil")
	}

	if !strings.Contains(err.Error(), "memory limit exceeded") {
		t.Errorf("Expected 'memory limit exceeded' error, got: %v", err)
	}
}

func TestMemoryLimit_StringFind(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.ResetMemoryUsage()
	L.SetMemoryLimit(3 * 1024) // 3KB limit

	err := L.DoString(`
		local base = string.rep("hello world ", 50)
		local results = {}
		for i = 1, 1000 do
			local _, _, cap = string.find(base, "(hello)")
			results[i] = cap  -- Captured strings
		end
	`)

	if err == nil {
		t.Fatal("Expected memory limit error, got nil")
	}

	if !strings.Contains(err.Error(), "memory limit exceeded") {
		t.Errorf("Expected 'memory limit exceeded' error, got: %v", err)
	}
}

func TestMemoryLimit_StringOperationsTracking(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.ResetMemoryUsage()
	L.SetMemoryLimit(100 * 1024) // 100KB limit

	err := L.DoString(`
		-- Test that memory is being tracked for various string operations
		local s1 = string.upper("test")
		local s2 = string.lower("TEST")
		local s3 = string.reverse("hello")
		local s4 = string.char(65, 66, 67)
		local s5 = string.format("test %s", "value")
		local s6 = string.sub("hello world", 1, 5)
		local s7 = string.gsub("aaa", "a", "b")
		local s8 = string.match("test123", "(%d+)")
		local _, _, s9 = string.find("test", "(test)")
	`)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	usage := L.GetAllocatedBytes()
	if usage <= 0 {
		t.Errorf("Expected memory usage to be tracked for string operations, got %d", usage)
	}
}

func TestMemoryLimit_TableGrowth(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.ResetMemoryUsage()
	L.SetMemoryLimit(10 * 1024) // 10KB limit

	err := L.DoString(`
		local t = {}
		for i = 1, 10000 do
			t[i] = i
		end
	`)

	if err == nil {
		t.Fatal("Expected memory limit error for large table, got nil")
	}

	if !strings.Contains(err.Error(), "memory limit exceeded") {
		t.Errorf("Expected 'memory limit exceeded' error, got: %v", err)
	}
}

func TestMemoryLimit_Closures(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetMemoryLimit(50 * 1024) // 50KB limit
	L.ResetMemoryUsage()

	script := `
		local functions = {}
		for i = 1, 100 do
			local x = i
			functions[i] = function()
				return x * 2
			end
		end
		return functions[50]()
	`

	if err := L.DoString(script); err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}

	allocated := L.GetAllocatedBytes()
	if allocated == 0 {
		t.Error("Expected non-zero memory allocation for closures")
	}
}

func TestMemoryLimit_MixedOperations(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetMemoryLimit(500 * 1024) // 500KB limit
	L.ResetMemoryUsage()

	script := `
		-- Mix of array, hash, nested tables, strings, and functions
		local data = {}

		-- Array part
		for i = 1, 50 do
			data[i] = i * 2
		end

		-- Hash part
		for i = 1, 50 do
			data["key" .. i] = "value" .. i
		end

		-- Nested tables
		data.nested = {}
		for i = 1, 20 do
			data.nested[i] = { x = i, y = i * 2, name = "item" .. i }
		end

		-- Functions
		data.functions = {}
		for i = 1, 30 do
			local val = i
			data.functions[i] = function() return val * 3 end
		end

		-- String operations
		data.strings = {}
		for i = 1, 50 do
			data.strings[i] = string.rep("x", 100) .. i
		end

		return data.functions[10]()
	`

	if err := L.DoString(script); err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}

	result := L.Get(-1)
	if num, ok := result.(LNumber); !ok || num != 30 {
		t.Errorf("Expected function result 30, got %v", result)
	}
	L.Pop(1)

	allocated := L.GetAllocatedBytes()
	if allocated == 0 {
		t.Error("Expected non-zero memory allocation for mixed operations")
	}
}

func TestMemoryLimit_LargeArraySparse(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetMemoryLimit(500 * 1024)
	L.ResetMemoryUsage()

	script := `
		local t = {}
		-- Sparse array - only set specific indices
		for i = 1, 100 do
			t[i * 100] = i
		end
		return t[5000]
	`

	if err := L.DoString(script); err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}

	allocated := L.GetAllocatedBytes()
	if allocated == 0 {
		t.Error("Expected non-zero memory allocation for sparse array")
	}
}

func TestMemoryLimit_RecursiveTable(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.SetMemoryLimit(60 * 1024) // 60KB limit (increased for nested tables)
	L.ResetMemoryUsage()

	script := `
		local t = {}
		t.self = t  -- Recursive reference

		for i = 1, 50 do
			t[i] = { parent = t, value = i }
		end

		return #t
	`

	if err := L.DoString(script); err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}

	allocated := L.GetAllocatedBytes()
	if allocated == 0 {
		t.Error("Expected non-zero memory allocation for recursive tables")
	}
}

func TestTryCreateTable_WithinLimit(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.ResetMemoryUsage()
	L.SetMemoryLimit(10 * 1024)

	tb, err := L.TryCreateTable(8, 0)
	if err != nil {
		t.Fatalf("Expected TryCreateTable to succeed within the limit, got: %v", err)
	}
	if tb == nil {
		t.Fatal("Expected a table")
	}
	if L.GetAllocatedBytes() == 0 {
		t.Error("Expected the successful allocation to be tracked")
	}

	// Filling up to the requested capacity must not grow the array, so no further
	// allocation can be charged and nothing can raise.
	allocated := L.GetAllocatedBytes()
	for i := 1; i <= 8; i++ {
		tb.RawSetInt(i, LNumber(i))
	}
	if got := L.GetAllocatedBytes(); got != allocated {
		t.Errorf("Expected filling to capacity to charge nothing, allocated went %d -> %d", allocated, got)
	}
}

func TestTryCreateTable_OverLimit(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.ResetMemoryUsage()
	L.SetMemoryLimit(1024)

	tb, err := L.TryCreateTable(1024*1024, 0)
	if err == nil {
		t.Fatal("Expected TryCreateTable to return an error over the limit")
	}
	if tb != nil {
		t.Error("Expected no table on failure")
	}
	expected := fmt.Sprintf(
		"memory limit exceeded: %d bytes allocated, limit is %d bytes",
		ltableSize(1024*1024, 0), 1024,
	)
	if err.Error() != expected {
		t.Errorf("Expected %q, got: %v", expected, err)
	}
	if _, ok := err.(*ApiError); ok {
		t.Errorf("Expected a plain Go error, got an ApiError: %v", err)
	}
	if L.GetAllocatedBytes() != 0 {
		t.Error("Expected a rejected allocation not to be charged")
	}

	// The whole point: CreateTable raises here, and a raise with no protected frame
	// on the stack panics instead of unwinding into an error.
	func() {
		defer func() {
			if recover() == nil {
				t.Error("Expected CreateTable to panic over the limit")
			}
		}()
		L.CreateTable(1024*1024, 0)
	}()
}

func TestTryCreateTable_NoLimit(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.ResetMemoryUsage()
	L.SetMemoryLimit(0)

	if _, err := L.TryCreateTable(1024*1024, 0); err != nil {
		t.Fatalf("Expected TryCreateTable to succeed when no limit is set, got: %v", err)
	}
}

// Coroutines used to get a new state with no limit and their own counter, so
// a script could allocate without bound inside one.
func TestMemoryLimit_Coroutine(t *testing.T) {
	for _, tc := range []struct {
		name   string
		script string
	}{
		{"create and resume", `
			local co = coroutine.create(function()
				local t = {}
				for i = 1, 100 do t[i] = string.rep("x", 100000) .. i end
			end)
			local ok, err = coroutine.resume(co)
			assert(not ok)
			error(err)
		`},
		{"wrap", `
			local f = coroutine.wrap(function()
				local t = {}
				for i = 1, 100 do t[i] = string.rep("x", 100000) .. i end
			end)
			f()
		`},
		{"nested coroutines", `
			local f = coroutine.wrap(function()
				local g = coroutine.wrap(function()
					local t = {}
					for i = 1, 100 do t[i] = string.rep("x", 100000) .. i end
				end)
				g()
			end)
			f()
		`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			L := NewState()
			defer L.Close()
			L.SetMemoryLimit(1024 * 1024)

			err := L.DoString(tc.script)
			if err == nil || !strings.Contains(err.Error(), "memory limit exceeded") {
				t.Fatalf("expected a memory limit error, got %v", err)
			}
			if got := L.GetAllocatedBytes(); got > 2*1024*1024 {
				t.Fatalf("allocated %d bytes under a 1 MB limit", got)
			}
		})
	}
}

func TestMemoryLimit_CoroutineSharesCounter(t *testing.T) {
	L := NewState()
	defer L.Close()

	before := L.GetAllocatedBytes()
	err := L.DoString(`
		local co = coroutine.create(function()
			s = string.rep("x", 100000)
		end)
		assert(coroutine.resume(co))
	`)
	if err != nil {
		t.Fatal(err)
	}
	if got := L.GetAllocatedBytes() - before; got < 100000 {
		t.Fatalf("a coroutine's allocation is not counted: %d bytes", got)
	}
}

// len(str) * n overflowed into a negative size, which lowered the counter
// to near math.MinInt64 and turned the limit off for the rest of the script.
func TestMemoryLimit_StringRepeatOverflow(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.SetMemoryLimit(1024 * 1024)

	err := L.DoString(`
		local ok, err = pcall(string.rep, "aa", 2^62)
		assert(not ok)
		assert(string.find(err, "resulting string too large", 1, true), err)
		local t = {}
		for i = 1, 100 do t[i] = string.rep("x", 100000) .. i end
	`)
	if err == nil || !strings.Contains(err.Error(), "memory limit exceeded") {
		t.Fatalf("expected a memory limit error, got %v", err)
	}
	if got := L.GetAllocatedBytes(); got < 0 || got > 2*1024*1024 {
		t.Fatalf("allocated %d bytes under a 1 MB limit", got)
	}
}

func TestMemoryLimit_TrackAllocNegative(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.Push(L.NewFunction(func(L *LState) int {
		L.TrackAlloc(-1)
		return 0
	}))
	err := L.PCall(0, 0, nil)
	if err == nil || !strings.Contains(err.Error(), "memory limit exceeded") {
		t.Fatalf("expected a memory limit error, got %v", err)
	}
}

func TestMemoryLimit_TrackAllocSaturates(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.Push(L.NewFunction(func(L *LState) int {
		L.TrackAlloc(math.MaxInt64)
		L.TrackAlloc(math.MaxInt64)
		return 0
	}))
	if err := L.PCall(0, 0, nil); err != nil {
		t.Fatal(err)
	}
	if got := L.GetAllocatedBytes(); got != math.MaxInt64 {
		t.Fatalf("expected the counter to stop at MaxInt64, got %d", got)
	}
}

// formatSizeBound must never be below what fmt prints, or the check before
// formatting lets a larger result through.
func TestFormatSizeBound_IsUpperBound(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	verbs := "svqxXdifeEgGcboOUtTp%!"
	flags := "#0+- "
	values := []LValue{
		LString(""), LString("plain"), LString("\x00\x01\xff\xfe\a\n"),
		LString("ünïcödé   🙂"), LString(strings.Repeat("\x80", 50)),
		LString("12345"), LString("-1.5e10"),
		// Long enough that the overhead per verb does not hide escaping:
		// %#v and %q print each of these bytes as \x00 or \x80.
		LString(strings.Repeat("\x00", 1024)), LString(strings.Repeat("\x80", 1024)),
		LNumber(0), LNumber(-1), LNumber(1.5), LNumber(1e308), LNumber(-1e308),
		LNumber(5e-324), LNumber(math.Inf(1)), LNumber(math.NaN()), LNumber(1 << 62),
		LTrue, LFalse, LNil,
	}
	L := NewState()
	defer L.Close()
	values = append(values, formatArg(L.NewTable()), formatArg(L.NewFunction(func(*LState) int { return 0 })))

	for range 20000 {
		var b strings.Builder
		for range rng.Intn(4) + 1 {
			if rng.Intn(3) == 0 {
				b.WriteString("lit")
			}
			b.WriteByte('%')
			for range rng.Intn(3) {
				b.WriteByte(flags[rng.Intn(len(flags))])
			}
			switch rng.Intn(6) {
			case 0:
				fmt.Fprintf(&b, "%d", rng.Intn(3000))
			case 1:
				b.WriteString("*")
			case 2:
				fmt.Fprintf(&b, "[%d]", rng.Intn(4))
			}
			if rng.Intn(3) == 0 {
				fmt.Fprintf(&b, ".%d", rng.Intn(3000))
			}
			b.WriteByte(verbs[rng.Intn(len(verbs))])
		}
		format := b.String()
		args := make([]interface{}, rng.Intn(5))
		for i := range args {
			args[i] = values[rng.Intn(len(values))]
		}
		got := len(fmt.Sprintf(format, args...))
		if bound := formatSizeBound(format, args); int64(got) > bound {
			t.Fatalf("format %q with %v: printed %d bytes, bound %d", format, args, got, bound)
		}
	}
}

// Formats built from random pieces are mostly malformed, like "%1*s" or
// "%[2.*x", which fmt still prints: the bound must follow fmt's parser.
func TestFormatSizeBound_MalformedFormats(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	pieces := []string{
		"%", "%", "%", "#", "0", "+", "-", " ", "1", "7", "30", "*", ".", "[", "]",
		"[1]", "[2]", "[0]", "[9]", "[x]", "s", "v", "q", "x", "d", "f", "c", "ü", "lit",
	}
	values := []LValue{
		LString(""), LString("plain"), LString(strings.Repeat("\x00", 1024)),
		LString(strings.Repeat("x", 1024)), LNumber(1), LNumber(-1e308), LTrue, LNil,
	}
	for range 200000 {
		var b strings.Builder
		for range rng.Intn(12) + 1 {
			b.WriteString(pieces[rng.Intn(len(pieces))])
		}
		format := b.String()
		args := make([]interface{}, rng.Intn(4))
		for i := range args {
			args[i] = values[rng.Intn(len(values))]
		}
		got := len(fmt.Sprintf(format, args...))
		if bound := formatSizeBound(format, args); int64(got) > bound {
			t.Fatalf("format %q with %d args: printed %d bytes, bound %d", format, len(args), got, bound)
		}
	}
}

func TestFormatSizeBound_Cases(t *testing.T) {
	s := LString(strings.Repeat("x", 4096))
	for _, c := range []struct {
		format string
		args   []interface{}
	}{
		// '*' after a width is the verb, and prints the argument.
		{"%1*s", []interface{}{s}},
		{"%1.2*s", []interface{}{s}},
		// fmt accepts widths up to 10,000,009.
		{"%9999999s", []interface{}{s}},
		{"%10000009s", []interface{}{s}},
		{"%.9999999s", []interface{}{s}},
		// An overflowing width skips the rest of the format.
		{"%100000000s|tail", []interface{}{s}},
		// An index goes back to an argument that was used.
		{"%s%[1]s%[1]q", []interface{}{s}},
		{"%[1][2]s", []interface{}{s, s}},
		{"%[99999999]s%s", []interface{}{s}},
		{"%#v %#q %+q % #x", []interface{}{s, s, s, s}},
		{"%5.", []interface{}{s}},
		{"%ü", []interface{}{s}},
	} {
		got := len(fmt.Sprintf(c.format, c.args...))
		if bound := formatSizeBound(c.format, c.args); int64(got) > bound {
			t.Errorf("format %q: printed %d bytes, bound %d", c.format, got, bound)
		}
	}
}

func TestFormatSizeBound_Percent(t *testing.T) {
	// "%%" prints one byte, and a '%' verb ignores the width and precision.
	for _, c := range []struct {
		format string
		bound  int64
	}{
		{strings.Repeat("%%", 1000), 1000},
		{"%5%", 1},
		{"%-9999999.9999999%", 1},
		{"a%%b", 3},
	} {
		if got := formatSizeBound(c.format, nil); got != c.bound {
			t.Errorf("format %q: bound %d, want %d", c.format, got, c.bound)
		}
	}
	// A '*' still prints %!(BADWIDTH) and uses the argument up.
	args := []interface{}{LNumber(1)}
	if got, bound := len(fmt.Sprintf("%*%", args...)), formatSizeBound("%*%", args); int64(got) > bound {
		t.Errorf("%%*%%: printed %d bytes, bound %d", got, bound)
	}
}

// A '[' in the text around the verbs is not an argument index. It used to
// make every verb count the largest argument, as %x prints it: "[%s] %s"
// with a 1 MB string was counted as 10 MB.
func TestFormatSizeBound_LiteralBracket(t *testing.T) {
	s := LString(strings.Repeat("x", 1<<20))
	for _, c := range []struct {
		format string
		args   []interface{}
	}{
		{"[%s] %s", []interface{}{LString("info"), s}},
		{"[%s] [%s] [%s] %s", []interface{}{LString("a"), LString("b"), LString("c"), s}},
		{"%s[1]%s", []interface{}{LString("a"), s}},
		{"%s]%s[", []interface{}{LString("a"), s}},
	} {
		got := int64(len(fmt.Sprintf(c.format, c.args...)))
		bound := formatSizeBound(c.format, c.args)
		if bound < got || bound > got+1024 {
			t.Errorf("format %q: printed %d bytes, bound %d", c.format, got, bound)
		}
	}
	// An index still makes each verb count the largest argument: %[2]s
	// prints s again after the %s that used it.
	args := []interface{}{LString("a"), s}
	if bound := formatSizeBound("%s%s%[2]s", args); bound < 2*int64(len(s)) {
		t.Errorf("bound %d is below the 2 MB that is printed", bound)
	}
}

func TestMemoryLimit_StringFormatPercent(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.SetMemoryLimit(8 << 20)
	// 2 MB of format for 1 MB of result. Counting 64 bytes for each "%%"
	// failed it with an estimate of 66 MB.
	if err := L.DoString(`assert(#string.format(string.rep("%%", 1000000)) == 1000000)`); err != nil {
		t.Fatal(err)
	}
}

func TestMemoryLimit_StringFormatLiteralBracket(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.SetMemoryLimit(8 << 20)
	// 2 MB of result. Counting each of the 4 verbs as the 2 MB string
	// printed by %x failed it with an estimate of 40 MB.
	err := L.DoString(`
		local s = string.rep("x", 2 * 1024 * 1024)
		assert(#string.format("[%s] %s %s %s", "info", "a", "b", s) == #s + 11)
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestMemoryLimit_StringFormatCheckedFirst(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.SetMemoryLimit(8 * 1024 * 1024)

	for _, script := range []string{
		// 2000 widths of 1 MB: 2 GB from one call.
		`local args = {} for i = 1, 2000 do args[i] = "x" end
		 return string.format(string.rep("%999999s", 2000), unpack(args))`,
		// One 1 MB string used 100 times.
		`local s = string.rep("x", 1024 * 1024) local args = {}
		 for i = 1, 100 do args[i] = s end
		 return string.format(string.rep("%s", 100), unpack(args))`,
		// The same with one argument: %[1] prints it again for every verb.
		`local s = string.rep("x", 1024 * 1024)
		 return string.format(string.rep("%[1]s", 100), s)`,
		// %#v quotes and escapes: each NUL byte prints as \x00.
		`return string.format("%#v", string.rep("\0", 2 * 1024 * 1024))`,
		// After a width, '*' is the verb: %!*(lua.LString=...) prints the
		// 2 MB string, and the s after it is text.
		`return string.format("%1*s", string.rep("x", 2 * 1024 * 1024))`,
		// fmt applies widths up to 10,000,009.
		`return string.format("%9999999s", "x")`,
		// '*' uses up the 1 for the width, so %x prints the 3 MB string as
		// 6 MB of hex. (string.format passes one argument per '%', so the
		// second verb is needed to pass the string.)
		`return string.format("%*x%s", 1, string.rep("a", 3 * 1024 * 1024))`,
	} {
		before := L.GetAllocatedBytes()
		err := L.DoString(script)
		if err == nil || !strings.Contains(err.Error(), "memory limit exceeded") {
			t.Fatalf("expected a memory limit error, got %v", err)
		}
		// The format result was never counted: it was not built.
		if got := L.GetAllocatedBytes() - before; got > 4*1024*1024 {
			t.Fatalf("allocated %d bytes before failing", got)
		}
		L.ResetMemoryUsage()
	}
}

func TestStringFormat_ReferenceValues(t *testing.T) {
	L := NewState()
	defer L.Close()

	err := L.DoString(`
		local t = {string.rep("secret", 10)}
		assert(string.format("%s", t) == tostring(t))
		assert(string.format("%v", print) == tostring(print))
		-- fmt used to print the struct behind the table, strings included.
		local d = string.format("%d", t)
		assert(not string.find(d, "secret"), d)
		assert(string.format("%5.1f|%-3d|%q", 1.25, 7, "a\\n") == '  1.2|7  |"a\\\\n"')
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestMemoryLimit_ConcatCountedFirst(t *testing.T) {
	for _, script := range []string{
		`local s = string.rep("x", 1024 * 1024) local t = {}
		 for i = 1, 100 do t[i] = s end
		 return table.concat(t)`,
		`local s = string.rep("x", 1024 * 1024)
		 return s .. s .. s .. s .. s .. s .. s .. s .. s .. s .. s .. s`,
	} {
		L := NewState()
		L.SetMemoryLimit(8 * 1024 * 1024)
		err := L.DoString(script)
		if err == nil || !strings.Contains(err.Error(), "memory limit exceeded") {
			t.Fatalf("expected a memory limit error, got %v", err)
		}
		L.Close()
	}
}
