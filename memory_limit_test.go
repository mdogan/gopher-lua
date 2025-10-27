package lua

import (
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
		t.Fatalf("Expected memory limit error, got nil: %d", L.allocatedBytes)
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
