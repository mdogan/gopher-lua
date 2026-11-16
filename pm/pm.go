// Lua pattern match functions for Go
package pm

import (
	"context"
	"fmt"
)

const (
	EOS      = -1
	_UNKNOWN = -2
	// maxRecursionLevel is how deep the matcher can recurse. It recurses once
	// per capture boundary and once per repeat or optional item it is trying,
	// so the depth depends on the pattern only, not on the input. A level is
	// about 450 bytes of stack. C Lua 5.2 and later stop at 200 (MAXCCALLS);
	// Lua 5.1 has no limit, so allow more.
	maxRecursionLevel = 1000
	// maxCaptures is how many captures a pattern can have, Lua 5.1's
	// LUA_MAXCAPTURES. It also bounds how deep parsePattern and
	// compilePattern recurse, once per nested capture.
	maxCaptures = 32
	// ctxCheckInterval is how many matcher steps run between two context
	// checks. A step is one VM instruction, one byte scanned by %b or a
	// back-reference, or one entry of a character set checked against a byte.
	ctxCheckInterval = 1024
)

/* Error {{{ */

type Error struct {
	Pos     int
	Message string
}

func newError(pos int, message string, args ...interface{}) *Error {
	if len(args) == 0 {
		return &Error{pos, message}
	}
	return &Error{pos, fmt.Sprintf(message, args...)}
}

func (e *Error) Error() string {
	switch e.Pos {
	case EOS:
		return fmt.Sprintf("%s at EOS", e.Message)
	case _UNKNOWN:
		return fmt.Sprintf("%s", e.Message)
	default:
		return fmt.Sprintf("%s at %d", e.Message, e.Pos)
	}
}

/* }}} */

/* MatchData {{{ */

type MatchData struct {
	// captured positions
	// layout
	// xxxx xxxx xxxx xxx0 : caputured positions
	// xxxx xxxx xxxx xxx1 : position captured positions
	captures []uint32
}

func newMatchState() *MatchData { return &MatchData{[]uint32{}} }

func (st *MatchData) addPosCapture(s, pos int) {
	for s+1 >= len(st.captures) {
		st.captures = append(st.captures, 0)
	}
	st.captures[s] = (uint32(pos) << 1) | 1
	st.captures[s+1] = (uint32(pos) << 1) | 1
}

func (st *MatchData) setCapture(s, pos int) uint32 {
	for s >= len(st.captures) {
		st.captures = append(st.captures, 0)
	}
	v := st.captures[s]
	st.captures[s] = (uint32(pos) << 1)
	return v
}

func (st *MatchData) restoreCapture(s int, pos uint32) { st.captures[s] = pos }

func (st *MatchData) CaptureLength() int { return len(st.captures) }

func (st *MatchData) IsPosCapture(idx int) bool { return (st.captures[idx] & 1) == 1 }

func (st *MatchData) Capture(idx int) int { return int(st.captures[idx] >> 1) }

/* }}} */

/* scanner {{{ */

type scannerState struct {
	Pos     int
	started bool
}

type scanner struct {
	src   []byte
	State scannerState
	saved scannerState
	// captures is how many captures were parsed so far.
	captures int
}

func newScanner(src []byte) *scanner {
	return &scanner{
		src: src,
		State: scannerState{
			Pos:     0,
			started: false,
		},
		saved: scannerState{},
	}
}

func (sc *scanner) Length() int { return len(sc.src) }

func (sc *scanner) Next() int {
	if !sc.State.started {
		sc.State.started = true
		if len(sc.src) == 0 {
			sc.State.Pos = EOS
		}
	} else {
		sc.State.Pos = sc.NextPos()
	}
	if sc.State.Pos == EOS {
		return EOS
	}
	return int(sc.src[sc.State.Pos])
}

func (sc *scanner) CurrentPos() int {
	return sc.State.Pos
}

func (sc *scanner) NextPos() int {
	if sc.State.Pos == EOS || sc.State.Pos >= len(sc.src)-1 {
		return EOS
	}
	if !sc.State.started {
		return 0
	} else {
		return sc.State.Pos + 1
	}
}

func (sc *scanner) Peek() int {
	cureof := sc.State.Pos == EOS
	ch := sc.Next()
	if !cureof {
		if sc.State.Pos == EOS {
			sc.State.Pos = len(sc.src) - 1
		} else {
			sc.State.Pos--
			if sc.State.Pos < 0 {
				sc.State.Pos = 0
				sc.State.started = false
			}
		}
	}
	return ch
}

func (sc *scanner) Save() { sc.saved = sc.State }

func (sc *scanner) Restore() { sc.State = sc.saved }

/* }}} */

/* bytecode {{{ */

type opCode int

const (
	opChar opCode = iota
	opMatch
	opTailMatch
	opJmp
	opSplit
	opSave
	opPSave
	opBrace
	opNumber
	// opRepeat matches Class repeated, then the rest of the pattern.
	// Operand1 is the fewest repeats (0 for '*', 1 for '+'). Operand2 is 1
	// when it is lazy ('-'): it tries the fewest repeats first instead of
	// the most.
	opRepeat
)

type inst struct {
	OpCode   opCode
	Class    class
	Operand1 int
	Operand2 int
}

/* }}} */

/* classes {{{ */

type class interface {
	Matches(ch int) bool
}

type dotClass struct{}

func (pn *dotClass) Matches(ch int) bool { return true }

type charClass struct {
	Ch int
}

func (pn *charClass) Matches(ch int) bool { return pn.Ch == ch }

type singleClass struct {
	Class int
}

func (pn *singleClass) Matches(ch int) bool {
	ret := false
	switch pn.Class {
	case 'a', 'A':
		ret = 'A' <= ch && ch <= 'Z' || 'a' <= ch && ch <= 'z'
	case 'c', 'C':
		ret = (0x00 <= ch && ch <= 0x1F) || ch == 0x7F
	case 'd', 'D':
		ret = '0' <= ch && ch <= '9'
	case 'l', 'L':
		ret = 'a' <= ch && ch <= 'z'
	case 'p', 'P':
		ret = (0x21 <= ch && ch <= 0x2f) || (0x3a <= ch && ch <= 0x40) || (0x5b <= ch && ch <= 0x60) || (0x7b <= ch && ch <= 0x7e)
	case 's', 'S':
		switch ch {
		case ' ', '\f', '\n', '\r', '\t', '\v':
			ret = true
		}
	case 'u', 'U':
		ret = 'A' <= ch && ch <= 'Z'
	case 'w', 'W':
		ret = '0' <= ch && ch <= '9' || 'A' <= ch && ch <= 'Z' || 'a' <= ch && ch <= 'z'
	case 'x', 'X':
		ret = '0' <= ch && ch <= '9' || 'a' <= ch && ch <= 'f' || 'A' <= ch && ch <= 'F'
	case 'z', 'Z':
		ret = ch == 0
	default:
		return ch == pn.Class
	}
	if 'A' <= pn.Class && pn.Class <= 'Z' {
		return !ret
	}
	return ret
}

type setClass struct {
	IsNot   bool
	Classes []class
}

func (pn *setClass) Matches(ch int) bool {
	for _, class := range pn.Classes {
		if class.Matches(ch) {
			return !pn.IsNot
		}
	}
	return pn.IsNot
}

type rangeClass struct {
	Begin class
	End   class
}

func (pn *rangeClass) Matches(ch int) bool {
	switch begin := pn.Begin.(type) {
	case *charClass:
		end, ok := pn.End.(*charClass)
		if !ok {
			return false
		}
		return begin.Ch <= ch && ch <= end.Ch
	}
	return false
}

// }}}

// patterns {{{

type pattern interface{}

type singlePattern struct {
	Class class
}

type seqPattern struct {
	MustHead bool
	MustTail bool
	Patterns []pattern
}

type repeatPattern struct {
	Type  int
	Class class
}

type posCapPattern struct{}

type capPattern struct {
	Pattern pattern
}

type numberPattern struct {
	N int
}

type bracePattern struct {
	Begin int
	End   int
}

// }}}

/* parse {{{ */

func parseClass(sc *scanner, allowset bool) class {
	ch := sc.Next()
	switch ch {
	case '%':
		return &singleClass{sc.Next()}
	case '.':
		if allowset {
			return &dotClass{}
		}
		return &charClass{ch}
	case '[':
		if allowset {
			return parseClassSet(sc)
		}
		return &charClass{ch}
	//case '^' '$', '(', ')', ']', '*', '+', '-', '?':
	//	panic(newError(sc.CurrentPos(), "invalid %c", ch))
	case EOS:
		panic(newError(sc.CurrentPos(), "unexpected EOS"))
	default:
		return &charClass{ch}
	}
}

func parseClassSet(sc *scanner) class {
	set := &setClass{false, []class{}}
	if sc.Peek() == '^' {
		set.IsNot = true
		sc.Next()
	}
	isrange := false
	for {
		ch := sc.Peek()
		switch ch {
		// case '[':
		// 	panic(newError(sc.CurrentPos(), "'[' can not be nested"))
		case EOS:
			panic(newError(sc.CurrentPos(), "unexpected EOS"))
		case ']':
			if len(set.Classes) > 0 {
				sc.Next()
				goto exit
			}
			fallthrough
		case '-':
			if len(set.Classes) > 0 {
				sc.Next()
				isrange = true
				continue
			}
			fallthrough
		default:
			set.Classes = append(set.Classes, parseClass(sc, false))
		}
		if isrange {
			begin := set.Classes[len(set.Classes)-2]
			end := set.Classes[len(set.Classes)-1]
			set.Classes = set.Classes[0 : len(set.Classes)-2]
			set.Classes = append(set.Classes, &rangeClass{begin, end})
			isrange = false
		}
	}
exit:
	if isrange {
		set.Classes = append(set.Classes, &charClass{'-'})
	}

	return set
}

func parsePattern(sc *scanner, toplevel bool) *seqPattern {
	pat := &seqPattern{}
	if toplevel {
		if sc.Peek() == '^' {
			sc.Next()
			pat.MustHead = true
		}
	}
	for {
		ch := sc.Peek()
		switch ch {
		case '%':
			sc.Save()
			sc.Next()
			switch sc.Peek() {
			case '0':
				panic(newError(sc.CurrentPos(), "invalid capture index"))
			case '1', '2', '3', '4', '5', '6', '7', '8', '9':
				pat.Patterns = append(pat.Patterns, &numberPattern{sc.Next() - 48})
			case 'b':
				sc.Next()
				pat.Patterns = append(pat.Patterns, &bracePattern{sc.Next(), sc.Next()})
			default:
				sc.Restore()
				pat.Patterns = append(pat.Patterns, &singlePattern{parseClass(sc, true)})
			}
		case '.', '[', ']':
			pat.Patterns = append(pat.Patterns, &singlePattern{parseClass(sc, true)})
		//case ']':
		//	panic(newError(sc.CurrentPos(), "invalid ']'"))
		case ')':
			if toplevel {
				panic(newError(sc.CurrentPos(), "invalid ')'"))
			}
			return pat
		case '(':
			sc.captures++
			if sc.captures > maxCaptures {
				panic(newError(sc.CurrentPos(), "too many captures"))
			}
			sc.Next()
			if sc.Peek() == ')' {
				sc.Next()
				pat.Patterns = append(pat.Patterns, &posCapPattern{})
			} else {
				ret := &capPattern{parsePattern(sc, false)}
				if sc.Peek() != ')' {
					panic(newError(sc.CurrentPos(), "unfinished capture"))
				}
				sc.Next()
				pat.Patterns = append(pat.Patterns, ret)
			}
		case '*', '+', '-', '?':
			sc.Next()
			if len(pat.Patterns) > 0 {
				spat, ok := pat.Patterns[len(pat.Patterns)-1].(*singlePattern)
				if ok {
					pat.Patterns = pat.Patterns[0 : len(pat.Patterns)-1]
					pat.Patterns = append(pat.Patterns, &repeatPattern{ch, spat.Class})
					continue
				}
			}
			pat.Patterns = append(pat.Patterns, &singlePattern{&charClass{ch}})
		case '$':
			if toplevel && (sc.NextPos() == sc.Length()-1 || sc.NextPos() == EOS) {
				pat.MustTail = true
			} else {
				pat.Patterns = append(pat.Patterns, &singlePattern{&charClass{ch}})
			}
			sc.Next()
		case EOS:
			sc.Next()
			goto exit
		default:
			sc.Next()
			pat.Patterns = append(pat.Patterns, &singlePattern{&charClass{ch}})
		}
	}
exit:
	return pat
}

type iptr struct {
	insts   []inst
	capture int
	// kinds holds the kind of each capture started so far, by its first
	// slot.
	kinds map[int]captureKind
}

type captureKind int

const (
	captureOpen captureKind = iota
	captureClosed
	capturePosition
)

// What an opNumber instruction does, in its Operand2. A pattern has no
// alternatives and no repeated groups, so whether capture N is started,
// closed or a position capture at %N is known when the pattern is compiled.
const (
	// backrefMatch matches the text of the capture.
	backrefMatch = iota
	// backrefInvalid raises "invalid capture index": the capture is not
	// started or not closed yet.
	backrefInvalid
	// backrefPosition never matches: the capture is a position capture. In
	// Lua 5.1 its length is (size_t)CAP_POSITION, longer than any input.
	backrefPosition
)

func compilePattern(p pattern, ps ...*iptr) []inst {
	var ptr *iptr
	toplevel := false
	if len(ps) == 0 {
		toplevel = true
		ptr = &iptr{[]inst{inst{opSave, nil, 0, -1}}, 2, map[int]captureKind{}}
	} else {
		ptr = ps[0]
	}
	switch pat := p.(type) {
	case *singlePattern:
		ptr.insts = append(ptr.insts, inst{opChar, pat.Class, -1, -1})
	case *seqPattern:
		for _, cp := range pat.Patterns {
			compilePattern(cp, ptr)
		}
	case *repeatPattern:
		idx := len(ptr.insts)
		switch pat.Type {
		case '*':
			ptr.insts = append(ptr.insts, inst{opRepeat, pat.Class, 0, 0})
		case '+':
			ptr.insts = append(ptr.insts, inst{opRepeat, pat.Class, 1, 0})
		case '-':
			ptr.insts = append(ptr.insts, inst{opRepeat, pat.Class, 0, 1})
		case '?':
			ptr.insts = append(ptr.insts,
				inst{opSplit, nil, idx + 1, idx + 2},
				inst{opChar, pat.Class, -1, -1})
		}
	case *posCapPattern:
		ptr.kinds[ptr.capture] = capturePosition
		ptr.insts = append(ptr.insts, inst{opPSave, nil, ptr.capture, -1})
		ptr.capture += 2
	case *capPattern:
		c0, c1 := ptr.capture, ptr.capture+1
		ptr.capture += 2
		ptr.kinds[c0] = captureOpen
		ptr.insts = append(ptr.insts, inst{opSave, nil, c0, -1})
		compilePattern(pat.Pattern, ptr)
		ptr.insts = append(ptr.insts, inst{opSave, nil, c1, -1})
		ptr.kinds[c0] = captureClosed
	case *bracePattern:
		ptr.insts = append(ptr.insts, inst{opBrace, nil, pat.Begin, pat.End})
	case *numberPattern:
		backref := backrefInvalid
		switch kind, ok := ptr.kinds[pat.N*2]; {
		case ok && kind == captureClosed:
			backref = backrefMatch
		case ok && kind == capturePosition:
			backref = backrefPosition
		}
		ptr.insts = append(ptr.insts, inst{opNumber, nil, pat.N, backref})
	}
	if toplevel {
		if p.(*seqPattern).MustTail {
			ptr.insts = append(ptr.insts, inst{opSave, nil, 1, -1}, inst{opTailMatch, nil, -1, -1})
		}
		ptr.insts = append(ptr.insts, inst{opSave, nil, 1, -1}, inst{opMatch, nil, -1, -1})
	}
	return ptr.insts
}

/* }}} parse */

/* VM {{{ */

// ctxDone is the panic value used to stop matching when the context is done.
type ctxDone struct{ err error }

type matcher struct {
	src   []byte
	insts []inst
	ctx   context.Context // nil when there is no context to check
	steps int
}

// step counts n units of matching work and, once ctxCheckInterval steps
// have run since the last check, stops matching with a ctxDone panic if the
// context is done.
func (vm *matcher) step(n int) {
	if vm.ctx == nil {
		return
	}
	vm.steps += n
	if vm.steps >= ctxCheckInterval {
		vm.checkContext()
	}
}

// checkContext is kept out of line so that step stays small enough to inline.
//
//go:noinline
func (vm *matcher) checkContext() {
	vm.steps = 0
	if err := vm.ctx.Err(); err != nil {
		panic(ctxDone{err})
	}
}

// matches reports whether ch is in class c, and counts the work as steps.
func (vm *matcher) matches(c class, ch byte) bool {
	vm.step(1)
	if set, ok := c.(*setClass); ok && vm.ctx != nil {
		// Matches checks the set entries one by one.
		vm.step(len(set.Classes))
	}
	return c.Matches(int(ch))
}

// Simple recursive virtual machine based on the
// "Regular Expression Matching: the Virtual Machine Approach" (https://swtch.com/~rsc/regexp/regexp2.html)
func (vm *matcher) recursiveVM(pc, sp, recLevel int, m *MatchData) (bool, int, *MatchData) {
	recLevel++
	if recLevel > maxRecursionLevel {
		panic(newError(_UNKNOWN, "pattern too complex"))
	}
	src, insts := vm.src, vm.insts
redo:
	vm.step(1)
	inst := insts[pc]
	switch inst.OpCode {
	case opChar:
		if sp >= len(src) || !vm.matches(inst.Class, src[sp]) {
			return false, sp, m
		}
		pc++
		sp++
		goto redo
	case opRepeat:
		// The repeated item is one character class, so the repeats are tried
		// in a loop, as C Lua does (max_expand and min_expand), instead of
		// recursing once per repeated byte. Only the rest of the pattern is
		// matched recursively, once per try.
		minimum := sp + inst.Operand1
		if inst.Operand2 == 0 {
			// Greedy: find the most repeats, then try the rest of the
			// pattern after each count, from the most to the fewest.
			end := sp
			for end < len(src) && vm.matches(inst.Class, src[end]) {
				end++
			}
			for ; end > minimum; end-- {
				if ok, nsp, _ := vm.recursiveVM(pc+1, end, recLevel, m); ok {
					return true, nsp, m
				}
			}
			if end < minimum {
				return false, sp, m
			}
			sp = end
			pc++
			goto redo
		}
		// Lazy: try the rest of the pattern after the fewest repeats, then
		// after one more repeat at a time.
		for ; sp < minimum; sp++ {
			if sp >= len(src) || !vm.matches(inst.Class, src[sp]) {
				return false, sp, m
			}
		}
		for {
			if ok, nsp, _ := vm.recursiveVM(pc+1, sp, recLevel, m); ok {
				return true, nsp, m
			}
			if sp >= len(src) || !vm.matches(inst.Class, src[sp]) {
				return false, sp, m
			}
			sp++
		}
	case opMatch:
		return true, sp, m
	case opTailMatch:
		return sp >= len(src), sp, m
	case opJmp:
		pc = inst.Operand1
		goto redo
	case opSplit:
		if ok, nsp, _ := vm.recursiveVM(inst.Operand1, sp, recLevel, m); ok {
			return true, nsp, m
		}
		pc = inst.Operand2
		goto redo
	case opSave:
		s := m.setCapture(inst.Operand1, sp)
		if ok, nsp, _ := vm.recursiveVM(pc+1, sp, recLevel, m); ok {
			return true, nsp, m
		}
		m.restoreCapture(inst.Operand1, s)
		return false, sp, m
	case opPSave:
		m.addPosCapture(inst.Operand1, sp+1)
		pc++
		goto redo
	case opBrace:
		if sp >= len(src) || int(src[sp]) != inst.Operand1 {
			return false, sp, m
		}
		count := 1
		for sp = sp + 1; sp < len(src); sp++ {
			vm.step(1)
			if int(src[sp]) == inst.Operand2 {
				count--
			}
			if count == 0 {
				pc++
				sp++
				goto redo
			}
			if int(src[sp]) == inst.Operand1 {
				count++
			}
		}
		return false, sp, m
	case opNumber:
		switch inst.Operand2 {
		case backrefInvalid:
			panic(newError(_UNKNOWN, "invalid capture index"))
		case backrefPosition:
			return false, sp, m
		}
		idx := inst.Operand1 * 2
		capture := src[m.Capture(idx):m.Capture(idx+1)]
		for i := 0; i < len(capture); i++ {
			vm.step(1)
			if i+sp >= len(src) || capture[i] != src[i+sp] {
				return false, sp, m
			}
		}
		pc++
		sp += len(capture)
		goto redo
	}
	panic("should not reach here")
}

/* }}} */

/* API {{{ */

// Find finds up to limit matches of pattern p in src, starting at offset.
// A negative limit means no limit.
func Find(p string, src []byte, offset, limit int) (matches []*MatchData, err error) {
	return FindContext(nil, p, src, offset, limit)
}

// FindContext is like Find, but stops when ctx is done and returns ctx.Err().
// Backtracking on some patterns takes exponential time, so callers that run
// untrusted patterns should pass a context with a deadline. ctx may be nil.
func FindContext(ctx context.Context, p string, src []byte, offset, limit int) (matches []*MatchData, err error) {
	matches = []*MatchData{}
	err = FindEachContext(ctx, p, src, offset, limit, func(md *MatchData) bool {
		matches = append(matches, md)
		return true
	})
	if err != nil {
		return nil, err
	}
	return matches, nil
}

// FindEachContext is like FindContext, but calls fn with each match as it is
// found instead of returning them all, and stops early when fn returns false.
// A panic in fn is not recovered.
func FindEachContext(ctx context.Context, p string, src []byte, offset, limit int, fn func(*MatchData) bool) (err error) {
	defer func() {
		if v := recover(); v != nil {
			switch perr := v.(type) {
			case *Error:
				err = perr
			case ctxDone:
				err = perr.err
			case foreignPanic:
				panic(perr.v)
			default:
				panic(v)
			}
		}
	}()
	if limit == 0 {
		return nil
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	pat := compile(p)
	vm := &matcher{src: src, insts: pat.insts, ctx: ctx}
	n := 0
	for sp := offset; sp <= len(src); {
		ok, nsp, ms := vm.recursiveVM(0, sp, 0, newMatchState())
		sp++
		if ok {
			if sp < nsp {
				sp = nsp
			}
			n++
			if !callMatchFunc(fn, ms) {
				break
			}
		}
		if n == limit || pat.mustHead {
			break
		}
	}
	return
}

// Pattern is a compiled pattern.
type Pattern struct {
	insts    []inst
	mustHead bool
}

// Compile compiles the pattern p.
func Compile(p string) (pat *Pattern, err error) {
	defer func() {
		if v := recover(); v != nil {
			perr, ok := v.(*Error)
			if !ok {
				panic(v)
			}
			err = perr
		}
	}()
	return compile(p), nil
}

func compile(p string) *Pattern {
	sp := parsePattern(newScanner([]byte(p)), true)
	return &Pattern{insts: compilePattern(sp), mustHead: sp.MustHead}
}

// Matcher finds the matches of a pattern in a string one at a time, in the
// same order and at the same places as FindEachContext.
type Matcher struct {
	vm       matcher
	mustHead bool
	// pos is where the next match is looked for.
	pos  int
	done bool
}

// NewMatcher returns a Matcher for the matches of p in src, starting at
// offset. src must not change while the Matcher is used.
func (p *Pattern) NewMatcher(src []byte, offset int) *Matcher {
	return &Matcher{
		vm:       matcher{src: src, insts: p.insts},
		mustHead: p.mustHead,
		pos:      offset,
	}
}

// NextContext returns the next match, or nil when there are no more. It
// stops when ctx is done and returns ctx.Err(); ctx may be nil.
func (m *Matcher) NextContext(ctx context.Context) (md *MatchData, err error) {
	defer func() {
		if v := recover(); v != nil {
			m.done = true
			switch perr := v.(type) {
			case *Error:
				err = perr
			case ctxDone:
				err = perr.err
			default:
				panic(v)
			}
		}
	}()
	m.vm.ctx = ctx
	for !m.done && m.pos <= len(m.vm.src) {
		sp := m.pos
		ok, nsp, ms := m.vm.recursiveVM(0, sp, 0, newMatchState())
		m.pos = sp + 1
		if m.mustHead {
			// An anchored pattern is only tried at the offset.
			m.done = true
		}
		if ok {
			// The next match starts after this one, and at least one byte
			// later, so an empty match is not found again.
			if m.pos < nsp {
				m.pos = nsp
			}
			return ms, nil
		}
	}
	m.done = true
	return nil, nil
}

// callMatchFunc calls fn outside FindEachContext's recover: a panic in fn
// must reach fn's caller as it is, even if it is a *Error or ctxDone.
func callMatchFunc(fn func(*MatchData) bool, md *MatchData) (ok bool) {
	panicked := true
	defer func() {
		if panicked {
			panic(foreignPanic{recover()})
		}
	}()
	ok = fn(md)
	panicked = false
	return ok
}

// foreignPanic carries a panic from a FindEachContext callback through its
// recover.
type foreignPanic struct{ v interface{} }

/* }}} */
