package lua

import "testing"

// The expected results are from Lua 5.1.
func TestStringGsub_Count(t *testing.T) {
	L := NewState()
	defer L.Close()
	err := L.DoString(`
		local function check(want, wantN, got, n)
			assert(got == want and n == wantN, tostring(got) .. " " .. tostring(n))
		end
		check("xxx", 3, string.gsub("aaa", "a", "x"))
		check("xxa", 2, string.gsub("aaa", "a", "x", 2))
		check("-a-bc", 2, string.gsub("abc", "", "-", 2.7))
		check("-a-b-c-", 4, string.gsub("abc", "", "-"))
		-- A count of 0 or less replaces nothing; it replaced every match
		-- when the first position matched.
		check("aaa", 0, string.gsub("aaa", "a", "x", 0))
		check("aaa", 0, string.gsub("aaa", "a", "x", -1))
		-- The pattern is not used then.
		check("abc", 0, string.gsub("abc", "(", "x", 0))
		assert(not pcall(string.gsub, "abc", "(", "x", 1))
	`)
	if err != nil {
		t.Fatal(err)
	}
}
