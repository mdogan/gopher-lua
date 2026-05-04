package lua

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestLStateIsClosed(t *testing.T) {
	L := NewState()
	L.Close()
	errorIfNotEqual(t, true, L.IsClosed())
}

func TestCallStackOverflowWhenFixed(t *testing.T) {
	L := NewState(Options{
		CallStackSize: 3,
	})
	defer L.Close()

	// expect fixed stack implementation by default (for backwards compatibility)
	stack := L.stack
	if _, ok := stack.(*fixedCallFrameStack); !ok {
		t.Errorf("expected fixed callframe stack by default")
	}

	errorIfScriptNotFail(t, L, `
    local function recurse(count)
      if count > 0 then
        recurse(count - 1)
      end
    end
    local function c()
      print(_printregs())
      recurse(9)
    end
    c()
    `, "stack overflow")
}

func TestCallStackOverflowWhenAutoGrow(t *testing.T) {
	L := NewState(Options{
		CallStackSize:       3,
		MinimizeStackMemory: true,
	})
	defer L.Close()

	// expect auto growing stack implementation when MinimizeStackMemory is set
	stack := L.stack
	if _, ok := stack.(*autoGrowingCallFrameStack); !ok {
		t.Errorf("expected fixed callframe stack by default")
	}

	errorIfScriptNotFail(t, L, `
    local function recurse(count)
      if count > 0 then
        recurse(count - 1)
      end
    end
    local function c()
      print(_printregs())
      recurse(9)
    end
    c()
    `, "stack overflow")
}

func TestSkipOpenLibs(t *testing.T) {
	L := NewState(Options{SkipOpenLibs: true})
	defer L.Close()
	errorIfScriptNotFail(t, L, `print("")`,
		"attempt to call a non-function object")
	L2 := NewState()
	defer L2.Close()
	errorIfScriptFail(t, L2, `print("")`)
}

func TestReadOnlyTable(t *testing.T) {
	L := NewState()
	defer L.Close()

	tbl := L.NewTable()
	tbl.RawSetString("x", LNumber(1))
	tbl.RawSetInt(1, LNumber(3))
	tbl.RawSetInt(2, LNumber(2))
	L.SetGlobal("readonly", tbl)
	L.SetTableReadOnly(tbl, true)
	errorIfNotEqual(t, true, L.IsTableReadOnly(tbl))

	errorIfScriptNotFail(t, L, `readonly.x = 2`, readonlyTableError)
	errorIfScriptNotFail(t, L, `readonly.y = 2`, readonlyTableError)
	errorIfScriptNotFail(t, L, `rawset(readonly, "x", 2)`, readonlyTableError)
	errorIfScriptNotFail(t, L, `table.insert(readonly, 4)`, readonlyTableError)
	errorIfScriptNotFail(t, L, `table.remove(readonly)`, readonlyTableError)
	errorIfScriptNotFail(t, L, `table.sort(readonly)`, readonlyTableError)
	errorIfScriptNotFail(t, L, `setmetatable(readonly, {})`, readonlyTableError)
	errorIfScriptNotFail(t, L, `debug.setmetatable(readonly, {})`, readonlyTableError)

	errorIfGFuncNotFail(t, L, func(L *LState) int {
		L.RawSet(tbl, LString("x"), LNumber(2))
		return 0
	}, readonlyTableError)
	errorIfGFuncNotFail(t, L, func(L *LState) int {
		L.RawSetInt(tbl, 1, LNumber(2))
		return 0
	}, readonlyTableError)
	errorIfGFuncNotFail(t, L, func(L *LState) int {
		tbl.RawSetString("x", LNumber(2))
		return 0
	}, readonlyTableError)
	errorIfGFuncNotFail(t, L, func(L *LState) int {
		tbl.RawSetInt(1, LNumber(2))
		return 0
	}, readonlyTableError)
	errorIfGFuncNotFail(t, L, func(L *LState) int {
		tbl.RawSetH(LString("x"), LNumber(2))
		return 0
	}, readonlyTableError)

	L.WithTableReadOnlyBypass(func() {
		L.RawSet(tbl, LString("x"), LNumber(2))
		tbl.RawSetString("y", LNumber(3))
		tbl.Insert(1, LNumber(4))
		tbl.Remove(1)
		L.SetMetatable(tbl, L.NewTable())
	})
	errorIfNotEqual(t, LNumber(2), tbl.RawGetString("x"))
	errorIfNotEqual(t, LNumber(3), tbl.RawGetString("y"))

	errorIfGFuncNotFail(t, L, func(L *LState) int {
		tbl.RawSetString("z", LNumber(4))
		return 0
	}, readonlyTableError)

	errorIfScriptFail(t, L, `
		local t = {}
		t.x = 1
		rawset(t, "y", 2)
		table.insert(t, 3)
		table.remove(t)
		table.sort({3, 2, 1})
		setmetatable(t, {__index = {ok = true}})
		assert(t.x == 1 and t.y == 2 and t.ok)
	`)
}

func TestReadOnlyTableSetList(t *testing.T) {
	L := NewState()
	defer L.Close()

	tbl := L.NewTable()
	L.SetGlobal("readonly", tbl)
	L.SetTableReadOnly(tbl, true)

	proto := &FunctionProto{
		SourceName:       "setlist-readonly-test",
		NumUsedRegisters: 3,
		Code: []uint32{
			opCreateABx(OP_GETGLOBAL, 0, 0),
			opCreateABx(OP_LOADK, 1, 0),
			opCreateABx(OP_LOADK, 2, 1),
			opCreateABC(OP_SETLIST, 0, 2, 1),
			opCreateABC(OP_RETURN, 0, 1, 0),
		},
		Constants:          []LValue{LNumber(1), LNumber(2)},
		DbgSourcePositions: []int{1, 1, 1, 1, 1},
		stringConstants:    []string{"readonly"},
	}

	L.Push(L.NewFunctionFromProto(proto))
	err := L.PCall(0, 0, nil)
	errorIfNil(t, err)
	errorIfFalse(t, strings.Contains(err.Error(), readonlyTableError), "expected readonly error, got %v", err)
	errorIfNotEqual(t, LNil, tbl.RawGetInt(1))
	errorIfNotEqual(t, LNil, tbl.RawGetInt(2))
}

// Readonly must trump __newindex: writes that would otherwise be redirected
// into the readonly table by a metatable chain still error out.
func TestReadOnlyTableNewIndexBypass(t *testing.T) {
	L := NewState()
	defer L.Close()

	tbl := L.NewTable()
	tbl.RawSetString("existing", LNumber(1))
	L.SetGlobal("readonly", tbl)
	L.SetTableReadOnly(tbl, true)

	// Writing a missing key should still hit the readonly check, not fall
	// through to a __newindex hook even if one were attached.
	errorIfScriptNotFail(t, L, `readonly.missing = 1`, readonlyTableError)

	// A writable proxy whose __newindex points at the readonly table must
	// not let scripts smuggle writes through. setField walks the metatable
	// chain and re-checks readonly on each step.
	errorIfScriptNotFail(t, L, `
		local proxy = setmetatable({}, {__newindex = readonly})
		proxy.x = 99
	`, readonlyTableError)

	// __index reads through readonly are fine; only writes are gated.
	errorIfScriptFail(t, L, `
		local view = setmetatable({}, {__index = readonly})
		assert(view.existing == 1)
	`)
}

// The bypass uses a counter so nested scopes compose correctly: leaving the
// inner scope must not re-enable enforcement while the outer scope is active.
func TestReadOnlyTableNestedBypass(t *testing.T) {
	L := NewState()
	defer L.Close()

	tbl := L.NewTable()
	L.SetTableReadOnly(tbl, true)

	L.WithTableReadOnlyBypass(func() {
		tbl.RawSetString("a", LNumber(1))
		L.WithTableReadOnlyBypass(func() {
			tbl.RawSetString("b", LNumber(2))
		})
		// Inner scope returned; outer bypass must still apply.
		tbl.RawSetString("c", LNumber(3))
	})
	errorIfNotEqual(t, LNumber(1), tbl.RawGetString("a"))
	errorIfNotEqual(t, LNumber(2), tbl.RawGetString("b"))
	errorIfNotEqual(t, LNumber(3), tbl.RawGetString("c"))

	// Outer scope returned; enforcement is back on.
	errorIfGFuncNotFail(t, L, func(L *LState) int {
		tbl.RawSetString("d", LNumber(4))
		return 0
	}, readonlyTableError)
}

// Read paths must remain unaffected by the readonly flag — pairs/ipairs/#/
// rawget/table.concat/table.maxn are all canaries against future regressions.
func TestReadOnlyTableReadOps(t *testing.T) {
	L := NewState()
	defer L.Close()

	tbl := L.NewTable()
	tbl.RawSetInt(1, LString("a"))
	tbl.RawSetInt(2, LString("b"))
	tbl.RawSetInt(3, LString("c"))
	tbl.RawSetString("name", LString("redis"))
	L.SetGlobal("readonly", tbl)
	L.SetTableReadOnly(tbl, true)

	errorIfScriptFail(t, L, `
		assert(#readonly == 3)
		assert(rawget(readonly, "name") == "redis")
		assert(table.concat(readonly, ",") == "a,b,c")
		assert(table.maxn(readonly) == 3)

		local seen = 0
		for i, v in ipairs(readonly) do seen = seen + 1 end
		assert(seen == 3)

		local keys = 0
		for k, v in pairs(readonly) do keys = keys + 1 end
		assert(keys == 4)

		assert(getmetatable(readonly) == nil)
	`)
}

// A bypass on the parent LState must not leak into a child coroutine: scripts
// that try to write to a readonly table from inside a coroutine still fail,
// even if the embedder wrapped the resume in WithTableReadOnlyBypass.
func TestReadOnlyTableCoroutineNoLeak(t *testing.T) {
	L := NewState()
	defer L.Close()

	tbl := L.NewTable()
	L.SetGlobal("readonly", tbl)
	L.SetTableReadOnly(tbl, true)

	// Sanity: without bypass, the coroutine errors as expected.
	errorIfScriptFail(t, L, `
		local co = coroutine.create(function()
			readonly.x = 1
		end)
		local ok, err = coroutine.resume(co)
		assert(not ok, "coroutine should have failed")
		assert(string.find(err, "readonly"), "expected readonly error, got "..tostring(err))
	`)

	// With bypass on the parent, a coroutine still cannot write — its
	// executing LState has its own bypass counter at zero.
	L.WithTableReadOnlyBypass(func() {
		errorIfScriptFail(t, L, `
			local co = coroutine.create(function()
				readonly.x = 1
			end)
			local ok, err = coroutine.resume(co)
			assert(not ok, "coroutine should have failed even under parent bypass")
			assert(string.find(err, "readonly"), "expected readonly error, got "..tostring(err))
		`)
	})

	L.WithTableReadOnlyBypass(func() {
		errorIfScriptFail(t, L, `
			local ops = {
				function() rawset(readonly, "raw", 1) end,
				function() table.insert(readonly, 1) end,
				function() table.remove(readonly) end,
				function() table.sort(readonly) end,
			}
			for _, op in ipairs(ops) do
				local co = coroutine.create(op)
				local ok, err = coroutine.resume(co)
				assert(not ok, "coroutine should have failed even under parent bypass")
				assert(string.find(err, "readonly"), "expected readonly error, got "..tostring(err))
			end
		`)
	})

	errorIfNotEqual(t, LNil, tbl.RawGetString("x"))
	errorIfNotEqual(t, LNil, tbl.RawGetString("raw"))
	errorIfNotEqual(t, LNil, tbl.RawGetInt(1))
}

func TestGetAndReplace(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.Push(LString("a"))
	L.Replace(1, LString("b"))
	L.Replace(0, LString("c"))
	errorIfNotEqual(t, LNil, L.Get(0))
	errorIfNotEqual(t, LNil, L.Get(-10))
	errorIfNotEqual(t, L.Env, L.Get(EnvironIndex))
	errorIfNotEqual(t, LString("b"), L.Get(1))
	L.Push(LString("c"))
	L.Push(LString("d"))
	L.Replace(-2, LString("e"))
	errorIfNotEqual(t, LString("e"), L.Get(-2))
	registry := L.NewTable()
	L.Replace(RegistryIndex, registry)
	L.G.Registry = registry
	errorIfGFuncNotFail(t, L, func(L *LState) int {
		L.Replace(RegistryIndex, LNil)
		return 0
	}, "registry must be a table")
	errorIfGFuncFail(t, L, func(L *LState) int {
		env := L.NewTable()
		L.Replace(EnvironIndex, env)
		errorIfNotEqual(t, env, L.Get(EnvironIndex))
		return 0
	})
	errorIfGFuncNotFail(t, L, func(L *LState) int {
		L.Replace(EnvironIndex, LNil)
		return 0
	}, "environment must be a table")
	errorIfGFuncFail(t, L, func(L *LState) int {
		gbl := L.NewTable()
		L.Replace(GlobalsIndex, gbl)
		errorIfNotEqual(t, gbl, L.G.Global)
		return 0
	})
	errorIfGFuncNotFail(t, L, func(L *LState) int {
		L.Replace(GlobalsIndex, LNil)
		return 0
	}, "_G must be a table")

	L2 := NewState()
	defer L2.Close()
	clo := L2.NewClosure(func(L2 *LState) int {
		L2.Replace(UpvalueIndex(1), LNumber(3))
		errorIfNotEqual(t, LNumber(3), L2.Get(UpvalueIndex(1)))
		return 0
	}, LNumber(1), LNumber(2))
	L2.SetGlobal("clo", clo)
	errorIfScriptFail(t, L2, `clo()`)
}

func TestRemove(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.Push(LString("a"))
	L.Push(LString("b"))
	L.Push(LString("c"))

	L.Remove(4)
	errorIfNotEqual(t, LString("a"), L.Get(1))
	errorIfNotEqual(t, LString("b"), L.Get(2))
	errorIfNotEqual(t, LString("c"), L.Get(3))
	errorIfNotEqual(t, 3, L.GetTop())

	L.Remove(3)
	errorIfNotEqual(t, LString("a"), L.Get(1))
	errorIfNotEqual(t, LString("b"), L.Get(2))
	errorIfNotEqual(t, LNil, L.Get(3))
	errorIfNotEqual(t, 2, L.GetTop())
	L.Push(LString("c"))

	L.Remove(-10)
	errorIfNotEqual(t, LString("a"), L.Get(1))
	errorIfNotEqual(t, LString("b"), L.Get(2))
	errorIfNotEqual(t, LString("c"), L.Get(3))
	errorIfNotEqual(t, 3, L.GetTop())

	L.Remove(2)
	errorIfNotEqual(t, LString("a"), L.Get(1))
	errorIfNotEqual(t, LString("c"), L.Get(2))
	errorIfNotEqual(t, LNil, L.Get(3))
	errorIfNotEqual(t, 2, L.GetTop())
}

func TestToInt(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.Push(LNumber(10))
	L.Push(LString("99.9"))
	L.Push(L.NewTable())
	errorIfNotEqual(t, 10, L.ToInt(1))
	errorIfNotEqual(t, 99, L.ToInt(2))
	errorIfNotEqual(t, 0, L.ToInt(3))
}

func TestToInt64(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.Push(LNumber(10))
	L.Push(LString("99.9"))
	L.Push(L.NewTable())
	errorIfNotEqual(t, int64(10), L.ToInt64(1))
	errorIfNotEqual(t, int64(99), L.ToInt64(2))
	errorIfNotEqual(t, int64(0), L.ToInt64(3))
}

func TestToNumber(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.Push(LNumber(10))
	L.Push(LString("99.9"))
	L.Push(L.NewTable())
	errorIfNotEqual(t, LNumber(10), L.ToNumber(1))
	errorIfNotEqual(t, LNumber(99.9), L.ToNumber(2))
	errorIfNotEqual(t, LNumber(0), L.ToNumber(3))
}

func TestToString(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.Push(LNumber(10))
	L.Push(LString("99.9"))
	L.Push(L.NewTable())
	errorIfNotEqual(t, "10", L.ToString(1))
	errorIfNotEqual(t, "99.9", L.ToString(2))
	errorIfNotEqual(t, "", L.ToString(3))
}

func TestToTable(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.Push(LNumber(10))
	L.Push(LString("99.9"))
	L.Push(L.NewTable())
	errorIfFalse(t, L.ToTable(1) == nil, "index 1 must be nil")
	errorIfFalse(t, L.ToTable(2) == nil, "index 2 must be nil")
	errorIfNotEqual(t, L.Get(3), L.ToTable(3))
}

func TestToFunction(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.Push(LNumber(10))
	L.Push(LString("99.9"))
	L.Push(L.NewFunction(func(L *LState) int { return 0 }))
	errorIfFalse(t, L.ToFunction(1) == nil, "index 1 must be nil")
	errorIfFalse(t, L.ToFunction(2) == nil, "index 2 must be nil")
	errorIfNotEqual(t, L.Get(3), L.ToFunction(3))
}

func TestToUserData(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.Push(LNumber(10))
	L.Push(LString("99.9"))
	L.Push(L.NewUserData())
	errorIfFalse(t, L.ToUserData(1) == nil, "index 1 must be nil")
	errorIfFalse(t, L.ToUserData(2) == nil, "index 2 must be nil")
	errorIfNotEqual(t, L.Get(3), L.ToUserData(3))
}

func TestToChannel(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.Push(LNumber(10))
	L.Push(LString("99.9"))
	var ch chan LValue
	L.Push(LChannel(ch))
	errorIfFalse(t, L.ToChannel(1) == nil, "index 1 must be nil")
	errorIfFalse(t, L.ToChannel(2) == nil, "index 2 must be nil")
	errorIfNotEqual(t, ch, L.ToChannel(3))
}

func TestObjLen(t *testing.T) {
	L := NewState()
	defer L.Close()
	errorIfNotEqual(t, 3, L.ObjLen(LString("abc")))
	tbl := L.NewTable()
	tbl.Append(LTrue)
	tbl.Append(LTrue)
	errorIfNotEqual(t, 2, L.ObjLen(tbl))
	mt := L.NewTable()
	L.SetField(mt, "__len", L.NewFunction(func(L *LState) int {
		tbl := L.CheckTable(1)
		L.Push(LNumber(tbl.Len() + 1))
		return 1
	}))
	L.SetMetatable(tbl, mt)
	errorIfNotEqual(t, 3, L.ObjLen(tbl))
	errorIfNotEqual(t, 0, L.ObjLen(LNumber(10)))
}

func TestConcat(t *testing.T) {
	L := NewState()
	defer L.Close()
	errorIfNotEqual(t, "a1c", L.Concat(LString("a"), LNumber(1), LString("c")))
}

func TestPCall(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.Register("f1", func(L *LState) int {
		panic("panic!")
	})
	errorIfScriptNotFail(t, L, `f1()`, "panic!")
	L.Push(L.GetGlobal("f1"))
	err := L.PCall(0, 0, L.NewFunction(func(L *LState) int {
		L.Push(LString("by handler"))
		return 1
	}))
	errorIfFalse(t, strings.Contains(err.Error(), "by handler"), "")

	L.Push(L.GetGlobal("f1"))
	err = L.PCall(0, 0, L.NewFunction(func(L *LState) int {
		L.RaiseError("error!")
		return 1
	}))
	errorIfFalse(t, strings.Contains(err.Error(), "error!"), "")

	L.Push(L.GetGlobal("f1"))
	err = L.PCall(0, 0, L.NewFunction(func(L *LState) int {
		panic("panicc!")
	}))
	errorIfFalse(t, strings.Contains(err.Error(), "panicc!"), "")

	// Issue #452, expected to be revert back to previous call stack after any error.
	currentFrame, currentTop, currentSp := L.currentFrame, L.GetTop(), L.stack.Sp()
	L.Push(L.GetGlobal("f1"))
	err = L.PCall(0, 0, nil)
	errorIfFalse(t, err != nil, "")
	errorIfFalse(t, L.currentFrame == currentFrame, "")
	errorIfFalse(t, L.GetTop() == currentTop, "")
	errorIfFalse(t, L.stack.Sp() == currentSp, "")

	currentFrame, currentTop, currentSp = L.currentFrame, L.GetTop(), L.stack.Sp()
	L.Push(L.GetGlobal("f1"))
	err = L.PCall(0, 0, L.NewFunction(func(L *LState) int {
		L.RaiseError("error!")
		return 1
	}))
	errorIfFalse(t, err != nil, "")
	errorIfFalse(t, L.currentFrame == currentFrame, "")
	errorIfFalse(t, L.GetTop() == currentTop, "")
	errorIfFalse(t, L.stack.Sp() == currentSp, "")
}

func TestCoroutineApi1(t *testing.T) {
	L := NewState()
	defer L.Close()
	co, _ := L.NewThread()
	errorIfScriptFail(t, L, `
      function coro(v)
        assert(v == 10)
        local ret1, ret2 = coroutine.yield(1,2,3)
        assert(ret1 == 11)
        assert(ret2 == 12)
        coroutine.yield(4)
        return 5
      end
    `)
	fn := L.GetGlobal("coro").(*LFunction)
	st, err, values := L.Resume(co, fn, LNumber(10))
	errorIfNotEqual(t, ResumeYield, st)
	errorIfNotNil(t, err)
	errorIfNotEqual(t, 3, len(values))
	errorIfNotEqual(t, LNumber(1), values[0].(LNumber))
	errorIfNotEqual(t, LNumber(2), values[1].(LNumber))
	errorIfNotEqual(t, LNumber(3), values[2].(LNumber))

	st, err, values = L.Resume(co, fn, LNumber(11), LNumber(12))
	errorIfNotEqual(t, ResumeYield, st)
	errorIfNotNil(t, err)
	errorIfNotEqual(t, 1, len(values))
	errorIfNotEqual(t, LNumber(4), values[0].(LNumber))

	st, err, values = L.Resume(co, fn)
	errorIfNotEqual(t, ResumeOK, st)
	errorIfNotNil(t, err)
	errorIfNotEqual(t, 1, len(values))
	errorIfNotEqual(t, LNumber(5), values[0].(LNumber))

	L.Register("myyield", func(L *LState) int {
		return L.Yield(L.ToNumber(1))
	})
	errorIfScriptFail(t, L, `
      function coro_error()
        coroutine.yield(1,2,3)
        myyield(4)
        assert(false, "--failed--")
      end
    `)
	fn = L.GetGlobal("coro_error").(*LFunction)
	co, _ = L.NewThread()
	st, err, values = L.Resume(co, fn)
	errorIfNotEqual(t, ResumeYield, st)
	errorIfNotNil(t, err)
	errorIfNotEqual(t, 3, len(values))
	errorIfNotEqual(t, LNumber(1), values[0].(LNumber))
	errorIfNotEqual(t, LNumber(2), values[1].(LNumber))
	errorIfNotEqual(t, LNumber(3), values[2].(LNumber))

	st, err, values = L.Resume(co, fn)
	errorIfNotEqual(t, ResumeYield, st)
	errorIfNotNil(t, err)
	errorIfNotEqual(t, 1, len(values))
	errorIfNotEqual(t, LNumber(4), values[0].(LNumber))

	st, err, values = L.Resume(co, fn)
	errorIfNotEqual(t, ResumeError, st)
	errorIfNil(t, err)
	errorIfFalse(t, strings.Contains(err.Error(), "--failed--"), "error message must be '--failed--'")
	st, err, values = L.Resume(co, fn)
	errorIfNotEqual(t, ResumeError, st)
	errorIfNil(t, err)
	errorIfFalse(t, strings.Contains(err.Error(), "can not resume a dead thread"), "can not resume a dead thread")

}

func TestContextTimeout(t *testing.T) {
	L := NewState()
	defer L.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	L.SetContext(ctx)
	errorIfNotEqual(t, ctx, L.Context())
	err := L.DoString(`
	  local clock = os.clock
      function sleep(n)  -- seconds
        local t0 = clock()
        while clock() - t0 <= n do end
      end
	  sleep(3)
	`)
	errorIfNil(t, err)
	errorIfFalse(t, strings.Contains(err.Error(), "context deadline exceeded"), "execution must be canceled")

	oldctx := L.RemoveContext()
	errorIfNotEqual(t, ctx, oldctx)
	errorIfNotNil(t, L.ctx)
}

func TestContextCancel(t *testing.T) {
	L := NewState()
	defer L.Close()
	ctx, cancel := context.WithCancel(context.Background())
	errch := make(chan error, 1)
	L.SetContext(ctx)
	go func() {
		errch <- L.DoString(`
	    local clock = os.clock
        function sleep(n)  -- seconds
          local t0 = clock()
          while clock() - t0 <= n do end
        end
	    sleep(3)
	  `)
	}()
	time.Sleep(1 * time.Second)
	cancel()
	err := <-errch
	errorIfNil(t, err)
	errorIfFalse(t, strings.Contains(err.Error(), "context canceled"), "execution must be canceled")
}

func TestContextWithCroutine(t *testing.T) {
	L := NewState()
	defer L.Close()
	ctx, cancel := context.WithCancel(context.Background())
	L.SetContext(ctx)
	defer cancel()
	L.DoString(`
	    function coro()
		  local i = 0
		  while true do
		    coroutine.yield(i)
			i = i+1
		  end
		  return i
	    end
	`)
	co, cocancel := L.NewThread()
	defer cocancel()
	fn := L.GetGlobal("coro").(*LFunction)
	_, err, values := L.Resume(co, fn)
	errorIfNotNil(t, err)
	errorIfNotEqual(t, LNumber(0), values[0])
	// cancel the parent context
	cancel()
	_, err, values = L.Resume(co, fn)
	errorIfNil(t, err)
	errorIfFalse(t, strings.Contains(err.Error(), "context canceled"), "coroutine execution must be canceled when the parent context is canceled")

}

func TestPCallAfterFail(t *testing.T) {
	L := NewState()
	defer L.Close()
	errFn := L.NewFunction(func(L *LState) int {
		L.RaiseError("error!")
		return 0
	})
	changeError := L.NewFunction(func(L *LState) int {
		L.Push(errFn)
		err := L.PCall(0, 0, nil)
		if err != nil {
			L.RaiseError("A New Error")
		}
		return 0
	})
	L.Push(changeError)
	err := L.PCall(0, 0, nil)
	errorIfFalse(t, strings.Contains(err.Error(), "A New Error"), "error not propogated correctly")
}

func TestRegistryFixedOverflow(t *testing.T) {
	state := NewState()
	defer state.Close()
	reg := state.reg
	expectedPanic := false
	// should be non auto grow by default
	errorIfFalse(t, reg.maxSize == 0, "state should default to non-auto growing implementation")
	// fill the stack and check we get a panic
	test := LString("test")
	for i := 0; i < len(reg.array); i++ {
		reg.Push(test)
	}
	defer func() {
		rcv := recover()
		if rcv != nil {
			if expectedPanic {
				errorIfFalse(t, rcv.(error).Error() != "registry overflow", "expected registry overflow exception, got "+rcv.(error).Error())
			} else {
				t.Errorf("did not expect registry overflow")
			}
		} else if expectedPanic {
			t.Errorf("expected registry overflow exception, but didn't get panic")
		}
	}()
	expectedPanic = true
	reg.Push(test)
}

func TestRegistryAutoGrow(t *testing.T) {
	state := NewState(Options{RegistryMaxSize: 300, RegistrySize: 200, RegistryGrowStep: 25})
	defer state.Close()
	expectedPanic := false
	defer func() {
		rcv := recover()
		if rcv != nil {
			if expectedPanic {
				errorIfFalse(t, rcv.(error).Error() != "registry overflow", "expected registry overflow exception, got "+rcv.(error).Error())
			} else {
				t.Errorf("did not expect registry overflow")
			}
		} else if expectedPanic {
			t.Errorf("expected registry overflow exception, but didn't get panic")
		}
	}()
	reg := state.reg
	test := LString("test")
	for i := 0; i < 300; i++ {
		reg.Push(test)
	}
	expectedPanic = true
	reg.Push(test)
}

// This test exposed a panic caused by accessing an unassigned var in the lua registry.
// The panic was caused by initCallFrame. It was calling resize() on the registry after it had written some values
// directly to the reg's array, but crucially, before it had updated "top". This meant when the resize occurred, the
// values beyond top where not copied, and were lost, leading to a later uninitialised value being found in the registry.
func TestUninitializedVarAccess(t *testing.T) {
	L := NewState(Options{
		RegistrySize:    128,
		RegistryMaxSize: 256,
	})
	defer L.Close()
	// This test needs to trigger a resize when the local vars are allocated, so we need it to
	// be 128 for the padding amount in the test function to work. If it's larger, we will need
	// more padding to force the error.
	errorIfNotEqual(t, cap(L.reg.array), 128)
	ctx, cancel := context.WithCancel(context.Background())
	L.SetContext(ctx)
	defer cancel()
	errorIfScriptFail(t, L, `
		local function test(arg1, arg2, arg3)
			-- padding to cause a registry resize when the local vars for this func are reserved
			local a0,b0,c0,d0,e0,f0,g0,h0,i0,j0,k0,l0,m0,n0,o0,p0,q0,r0,s0,t0,u0,v0,w0,x0,y0,z0
			local a1,b1,c1,d1,e1,f1,g1,h1,i1,j1,k1,l1,m1,n1,o1,p1,q1,r1,s1,t1,u1,v1,w1,x1,y1,z1
			local a2,b2,c2,d2,e2,f2,g2,h2,i2,j2,k2,l2,m2,n2,o2,p2,q2,r2,s2,t2,u2,v2,w2,x2,y2,z2
			local a3,b3,c3,d3,e3,f3,g3,h3,i3,j3,k3,l3,m3,n3,o3,p3,q3,r3,s3,t3,u3,v3,w3,x3,y3,z3
			local a4,b4,c4,d4,e4,f4,g4,h4,i4,j4,k4,l4,m4,n4,o4,p4,q4,r4,s4,t4,u4,v4,w4,x4,y4,z4
			if arg3 == nil then
				return 1
			end
			return 0
		end

		test(1,2)
	`)
}

func BenchmarkCallFrameStackPushPopAutoGrow(t *testing.B) {
	stack := newAutoGrowingCallFrameStack(256)

	t.ResetTimer()

	const Iterations = 256
	for j := 0; j < t.N; j++ {
		for i := 0; i < Iterations; i++ {
			stack.Push(callFrame{})
		}
		for i := 0; i < Iterations; i++ {
			stack.Pop()
		}
	}
}

func BenchmarkCallFrameStackPushPopFixed(t *testing.B) {
	stack := newFixedCallFrameStack(256)

	t.ResetTimer()

	const Iterations = 256
	for j := 0; j < t.N; j++ {
		for i := 0; i < Iterations; i++ {
			stack.Push(callFrame{})
		}
		for i := 0; i < Iterations; i++ {
			stack.Pop()
		}
	}
}

// this test will intentionally not incur stack growth in order to bench the performance when no allocations happen
func BenchmarkCallFrameStackPushPopShallowAutoGrow(t *testing.B) {
	stack := newAutoGrowingCallFrameStack(256)

	t.ResetTimer()

	const Iterations = 8
	for j := 0; j < t.N; j++ {
		for i := 0; i < Iterations; i++ {
			stack.Push(callFrame{})
		}
		for i := 0; i < Iterations; i++ {
			stack.Pop()
		}
	}
}

func BenchmarkCallFrameStackPushPopShallowFixed(t *testing.B) {
	stack := newFixedCallFrameStack(256)

	t.ResetTimer()

	const Iterations = 8
	for j := 0; j < t.N; j++ {
		for i := 0; i < Iterations; i++ {
			stack.Push(callFrame{})
		}
		for i := 0; i < Iterations; i++ {
			stack.Pop()
		}
	}
}

func BenchmarkCallFrameStackPushPopFixedNoInterface(t *testing.B) {
	stack := newFixedCallFrameStack(256).(*fixedCallFrameStack)

	t.ResetTimer()

	const Iterations = 256
	for j := 0; j < t.N; j++ {
		for i := 0; i < Iterations; i++ {
			stack.Push(callFrame{})
		}
		for i := 0; i < Iterations; i++ {
			stack.Pop()
		}
	}
}

func BenchmarkCallFrameStackUnwindAutoGrow(t *testing.B) {
	stack := newAutoGrowingCallFrameStack(256)

	t.ResetTimer()

	const Iterations = 256
	for j := 0; j < t.N; j++ {
		for i := 0; i < Iterations; i++ {
			stack.Push(callFrame{})
		}
		stack.SetSp(0)
	}
}

func BenchmarkCallFrameStackUnwindFixed(t *testing.B) {
	stack := newFixedCallFrameStack(256)

	t.ResetTimer()

	const Iterations = 256
	for j := 0; j < t.N; j++ {
		for i := 0; i < Iterations; i++ {
			stack.Push(callFrame{})
		}
		stack.SetSp(0)
	}
}

func BenchmarkCallFrameStackUnwindFixedNoInterface(t *testing.B) {
	stack := newFixedCallFrameStack(256).(*fixedCallFrameStack)

	t.ResetTimer()

	const Iterations = 256
	for j := 0; j < t.N; j++ {
		for i := 0; i < Iterations; i++ {
			stack.Push(callFrame{})
		}
		stack.SetSp(0)
	}
}

type registryTestHandler int

func (registryTestHandler) registryOverflow() {
	panic("registry overflow")
}

// test pushing and popping from the registry
func BenchmarkRegistryPushPopAutoGrow(t *testing.B) {
	al := newAllocator(32)
	sz := 256 * 20
	reg := newRegistry(registryTestHandler(0), sz/2, 64, sz, al)
	value := LString("test")

	t.ResetTimer()

	for j := 0; j < t.N; j++ {
		for i := 0; i < sz; i++ {
			reg.Push(value)
		}
		for i := 0; i < sz; i++ {
			reg.Pop()
		}
	}
}

func BenchmarkRegistryPushPopFixed(t *testing.B) {
	al := newAllocator(32)
	sz := 256 * 20
	reg := newRegistry(registryTestHandler(0), sz, 0, sz, al)
	value := LString("test")

	t.ResetTimer()

	for j := 0; j < t.N; j++ {
		for i := 0; i < sz; i++ {
			reg.Push(value)
		}
		for i := 0; i < sz; i++ {
			reg.Pop()
		}
	}
}

func BenchmarkRegistrySetTop(t *testing.B) {
	al := newAllocator(32)
	sz := 256 * 20
	reg := newRegistry(registryTestHandler(0), sz, 32, sz*2, al)

	t.ResetTimer()

	for j := 0; j < t.N; j++ {
		reg.SetTop(sz)
		reg.SetTop(0)
	}
}
