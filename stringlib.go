package lua

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/yuin/gopher-lua/pm"
)

const emptyLString LString = LString("")

func OpenString(L *LState) int {
	var mod *LTable
	// _, ok := L.G.builtinMts[int(LTString)]
	// if !ok {
	mod = L.RegisterModule(StringLibName, strFuncs).(*LTable)
	gmatch := L.NewClosure(strGmatch, L.NewFunction(strGmatchIter))
	mod.RawSetString("gmatch", gmatch)
	mod.RawSetString("gfind", gmatch)
	mod.RawSetString("__index", mod)
	L.G.builtinMts[int(LTString)] = mod
	// }
	L.Push(mod)
	return 1
}

var strFuncs = map[string]LGFunction{
	"byte":    strByte,
	"char":    strChar,
	"dump":    strDump,
	"find":    strFind,
	"format":  strFormat,
	"gsub":    strGsub,
	"len":     strLen,
	"lower":   strLower,
	"match":   strMatch,
	"rep":     strRep,
	"reverse": strReverse,
	"sub":     strSub,
	"upper":   strUpper,
}

func strByte(L *LState) int {
	str := L.CheckString(1)
	start := L.OptInt(2, 1) - 1
	end := L.OptInt(3, -1)
	l := len(str)
	if start < 0 {
		start = l + start + 1
	}
	if end < 0 {
		end = l + end + 1
	}

	if L.GetTop() == 2 {
		if start < 0 || start >= l {
			return 0
		}
		L.Push(LNumber(str[start]))
		return 1
	}

	start = intMax(start, 0)
	end = intMin(end, l)
	if end < 0 || end <= start || start >= l {
		return 0
	}

	for i := start; i < end; i++ {
		L.Push(LNumber(str[i]))
	}
	return end - start
}

func strChar(L *LState) int {
	top := L.GetTop()
	bytes := make([]byte, L.GetTop())
	for i := 1; i <= top; i++ {
		bytes[i-1] = uint8(L.CheckInt(i))
	}
	result := string(bytes)
	L.TrackAlloc(int64(len(result)))
	L.Push(LString(result))
	return 1
}

func strDump(L *LState) int {
	L.RaiseError("GopherLua does not support the string.dump")
	return 0
}

func strFind(L *LState) int {
	str := L.CheckString(1)
	pattern := L.CheckString(2)
	if len(pattern) == 0 {
		L.Push(LNumber(1))
		L.Push(LNumber(0))
		return 2
	}
	init := luaIndex2StringIndex(str, L.OptInt(3, 1), true)
	plain := false
	if L.GetTop() == 4 {
		plain = LVAsBool(L.Get(4))
	}

	if plain {
		pos := strings.Index(str[init:], pattern)
		if pos < 0 {
			L.Push(LNil)
			return 1
		}
		L.Push(LNumber(init+pos) + 1)
		L.Push(LNumber(init + pos + len(pattern)))
		return 2
	}

	mds, err := pm.FindContext(L.ctx, pattern, unsafeFastStringToReadOnlyBytes(str), init, 1)
	if err != nil {
		L.RaiseError(err.Error())
	}
	if len(mds) == 0 {
		L.Push(LNil)
		return 1
	}
	md := mds[0]
	L.Push(LNumber(md.Capture(0) + 1))
	L.Push(LNumber(md.Capture(1)))
	for i := 2; i < md.CaptureLength(); i += 2 {
		if md.IsPosCapture(i) {
			L.Push(LNumber(md.Capture(i)))
		} else {
			capture := str[md.Capture(i):md.Capture(i+1)]
			L.TrackAlloc(int64(len(capture)))
			L.Push(LString(capture))
		}
	}
	return md.CaptureLength()/2 + 1
}

func strFormat(L *LState) int {
	str := L.CheckString(1)
	args := make([]interface{}, L.GetTop()-1)
	top := L.GetTop()
	for i := 2; i <= top; i++ {
		args[i-2] = formatArg(L.Get(i))
	}
	npat := strings.Count(str, "%") - strings.Count(str, "%%")
	args = args[:intMin(npat, len(args))]
	// fmt builds the whole result before it can be counted, and a large
	// width or a large argument used many times makes it far larger than
	// the input. Check an upper bound of its size first.
	L.checkAlloc(formatSizeBound(str, args))
	result := fmt.Sprintf(str, args...)
	L.TrackAlloc(int64(len(result)))
	L.Push(LString(result))
	return 1
}

// formatArg returns the value string.format passes to fmt for v. A value
// other than a string, number, boolean or nil is passed as its string, as
// tostring shows it: for verbs other than %s and %v, fmt printed the fields
// of the Go struct behind it instead, with every string held in a table.
func formatArg(v LValue) LValue {
	switch v.(type) {
	case LString, LNumber, LBool, *LNilType:
		return v
	default:
		return LString(v.String())
	}
}

// formatOverhead bounds what fmt adds around a value, such as
// "%!d(string=" and ")" for a verb that does not fit the type.
const formatOverhead = 64

// formatSizeBound returns an upper bound of len(fmt.Sprintf(format, args...))
// for the args formatArg returns.
//
// It walks the format the way fmt's doPrintf does (Go 1.26), including for
// malformed formats, so that it charges every verb to the argument fmt
// prints with it.
func formatSizeBound(format string, args []interface{}) int64 {
	size, reordered := walkFormatSize(format, args, nil)
	if reordered {
		// An index such as %[1]s lets many verbs print one argument, and go
		// back to an argument that was used. Lua has no such syntax, so
		// instead of following the indexes, count the largest argument for
		// each verb.
		size, _ = walkFormatSize(format, args, newFormatArgMax(args))
	}
	return size
}

// walkFormatSize returns formatSizeBound for a format without argument
// indexes, and reports whether the format has one. Like fmt's reordered, that
// is a '[' where fmt looks for an index, whether it parses or not; a '[' in
// the text around the verbs is not one. The walk is the same either way, so
// the report is exact. With largest, each verb counts the largest argument
// instead of the next one.
func walkFormatSize(format string, args []interface{}, largest *formatArgMax) (size int64, reordered bool) {
	skipIndex := func(i int) (int, bool) {
		newi, ok := skipFormatArgIndex(format, i)
		if newi > i {
			reordered = true
		}
		return newi, ok
	}

	argNum := 0
	end := len(format)
	for i := 0; i < end; {
		lasti := i
		for i < end && format[i] != '%' {
			i++
		}
		size += int64(i - lasti)
		if i >= end {
			break
		}
		i++

		sharp := false
		simple := false
	flags:
		for ; i < end; i++ {
			switch c := format[i]; c {
			case '#':
				sharp = true
			case '0', '+', '-', ' ':
			default:
				if 'a' <= c && c <= 'z' && argNum < len(args) {
					// A verb right after the flags.
					size += formatOverhead
					if largest != nil {
						size += largest.bound(rune(c), sharp)
					} else {
						size += formatArgBound(args[argNum], rune(c), sharp)
					}
					argNum++
					i++
					simple = true
				}
				break flags
			}
		}
		if simple {
			continue
		}

		var afterIndex bool
		var width, prec, stars int
		i, afterIndex = skipIndex(i)
		if i < end && format[i] == '*' {
			// The width comes from the next argument. Lua numbers are
			// float64, never an int, so fmt prints %!(BADWIDTH) instead, but
			// still uses the argument up.
			i++
			stars++
			if argNum < len(args) {
				argNum++
			}
			afterIndex = false
		} else {
			width, _, i = parseFormatNum(format, i, end)
		}
		if i+1 < end && format[i] == '.' {
			i++
			i, afterIndex = skipIndex(i)
			if i < end && format[i] == '*' {
				i++
				stars++
				if argNum < len(args) {
					argNum++
				}
				afterIndex = false
			} else {
				prec, _, i = parseFormatNum(format, i, end)
			}
		}
		if !afterIndex {
			i, _ = skipIndex(i)
		}
		// Each '*' prints %!(BADWIDTH) or %!(BADPREC).
		size += int64(stars) * int64(len("%!(BADWIDTH)"))
		if i >= end {
			// Prints %!(NOVERB).
			size += formatOverhead
			break
		}
		verb, n := utf8.DecodeRuneInString(format[i:])
		i += n
		if verb == '%' {
			// Prints one '%', ignores the width and precision, and uses no
			// argument.
			size++
			continue
		}
		size += int64(width) + int64(prec) + formatOverhead
		switch {
		case largest != nil:
			// An index can go back to an earlier argument, so argNum may be
			// past the one fmt prints.
			size += largest.bound(verb, sharp)
			argNum++
		case argNum >= len(args):
			// Prints %!verb(MISSING).
		default:
			size += formatArgBound(args[argNum], verb, sharp)
			argNum++
		}
	}
	// Arguments no verb used are printed as %!(EXTRA type=value, ...).
	for _, arg := range args[min(argNum, len(args)):] {
		size += formatArgBound(arg, 'v', false) + formatOverhead
	}
	return size, reordered
}

// formatArgMax holds the largest formatArgBound of a list of arguments, so
// that a verb finds it without going through the list.
type formatArgMax struct {
	// raw is for the verbs that print a string as it is, other for the rest.
	raw, other int64
}

func newFormatArgMax(args []interface{}) *formatArgMax {
	m := &formatArgMax{}
	for _, arg := range args {
		// formatArgBound depends on the verb only through formatPrintsRaw,
		// so 's' and 'x' stand for every verb.
		m.raw = max(m.raw, formatArgBound(arg, 's', false))
		m.other = max(m.other, formatArgBound(arg, 'x', false))
	}
	return m
}

func (m *formatArgMax) bound(verb rune, sharp bool) int64 {
	if formatPrintsRaw(verb, sharp) {
		return m.raw
	}
	return m.other
}

// formatPrintsRaw reports whether fmt prints a string as it is for verb: %s,
// and %v without the '#' flag.
func formatPrintsRaw(verb rune, sharp bool) bool {
	return verb == 's' || verb == 'v' && !sharp
}

// parseFormatNum parses a number at format[start:end] like fmt's parsenum:
// it checks for overflow before adding each digit, so it accepts up to
// 10,000,009. On overflow it returns end, which skips the rest of the format,
// and ok is false; ok is also false when there is no digit.
func parseFormatNum(format string, start, end int) (n int, ok bool, newi int) {
	if start >= end {
		return 0, false, end
	}
	for newi = start; newi < end && '0' <= format[newi] && format[newi] <= '9'; newi++ {
		if n > 1e6 {
			return 0, false, end
		}
		n = n*10 + int(format[newi]-'0')
		ok = true
	}
	return n, ok, newi
}

// skipFormatArgIndex skips an argument index such as [3] at format[i:] like
// fmt's argNumber, and reports whether it parsed as a number, which fmt
// reports even when the index is out of range. Without a closing bracket,
// only the '[' is skipped.
func skipFormatArgIndex(format string, i int) (int, bool) {
	if i >= len(format) || format[i] != '[' {
		return i, false
	}
	if len(format)-i < 3 {
		return i + 1, false
	}
	for j := i + 1; j < len(format); j++ {
		if format[j] == ']' {
			_, ok, newi := parseFormatNum(format, i+1, j)
			return j + 1, ok && newi == j
		}
	}
	return i + 1, false
}

// formatArgBound returns an upper bound of how long fmt prints arg for verb,
// not counting the width and precision. sharp is the '#' flag: %#v prints a
// string in Go syntax, quoted and escaped like %q. It depends on verb and
// sharp only through formatPrintsRaw.
func formatArgBound(arg interface{}, verb rune, sharp bool) int64 {
	switch v := arg.(type) {
	case LString:
		n := int64(len(v))
		if formatPrintsRaw(verb, sharp) {
			return n
		}
		// %q escapes a byte as \xNN, and "% #x" prints it as "0xNN ".
		return 5*n + 2
	case LNumber:
		// %f of 1e308 has 309 digits before the precision.
		return 400
	default:
		return formatOverhead
	}
}

// gsubCheckInterval is how many matches gsub replaces between two context
// checks. Matching checks the context itself, but replacing does not.
const gsubCheckInterval = 1024

func strGsub(L *LState) int {
	str := L.CheckString(1)
	pat := L.CheckString(2)
	L.CheckTypes(3, LTString, LTTable, LTFunction)
	repl := L.CheckAny(3)
	limit := L.OptInt(4, -1)

	// Build the result in one pass, while matching, and count it as it
	// grows. Collecting the matches first held one MatchData per match,
	// and replacing them one by one copied the whole string per match.
	var b strings.Builder
	last := 0
	n := 0
	appendPart := func(part string) {
		L.TrackAlloc(int64(len(part)))
		b.WriteString(part)
	}
	err := pm.FindEachContext(L.ctx, pat, unsafeFastStringToReadOnlyBytes(str), 0, limit, func(match *pm.MatchData) bool {
		n++
		if n%gsubCheckInterval == 0 && L.ctx != nil {
			if err := L.ctx.Err(); err != nil {
				L.RaiseError(err.Error())
			}
		}
		start, end := match.Capture(0), match.Capture(1)
		var replacement string
		var ok bool
		switch lv := repl.(type) {
		case LString:
			appendPart(str[last:start])
			strGsubStr(L, str, string(lv), match, appendPart)
			last = end
			return true
		case *LTable:
			replacement, ok = strGsubTable(L, str, lv, match)
		case *LFunction:
			replacement, ok = strGsubFunc(L, str, lv, match)
		}
		appendPart(str[last:start])
		if ok {
			appendPart(replacement)
		} else {
			appendPart(str[start:end])
		}
		last = end
		return true
	})
	if err != nil {
		L.RaiseError(err.Error())
	}
	if n == 0 {
		L.SetTop(1)
		L.Push(LNumber(0))
		return 2
	}
	appendPart(str[last:])
	L.Push(LString(b.String()))
	L.Push(LNumber(n))
	return 2
}

func checkCaptureIndex(L *LState, m *pm.MatchData, idx int) {
	if idx <= 2 {
		return
	}
	if idx >= m.CaptureLength() {
		L.RaiseError("invalid capture index")
	}
}

func capturedString(L *LState, m *pm.MatchData, str string, idx int) string {
	checkCaptureIndex(L, m, idx)
	if idx >= m.CaptureLength() && idx == 2 {
		idx = 0
	}
	if m.IsPosCapture(idx) {
		return fmt.Sprint(m.Capture(idx))
	} else {
		return str[m.Capture(idx):m.Capture(idx+1)]
	}

}

// strGsubStr writes repl, with %0-%9 replaced by the captures of match, to
// appendPart one piece at a time, so each piece is counted before it is
// written. Building the whole replacement first let one match grow without a
// limit: string.rep("%0", 5000) repeats a 1 MB match into 5 GB.
//
// "%%" is "%", and a '%' before any other byte, or at the end, stays as it is.
func strGsubStr(L *LState, str string, repl string, match *pm.MatchData, appendPart func(string)) {
	last := 0 // where the text not written yet starts
	for i := 0; i < len(repl)-1; i++ {
		if repl[i] != '%' {
			continue
		}
		switch c := repl[i+1]; {
		case c == '%':
			appendPart(repl[last : i+1])
			last = i + 2
		case c >= '0' && c <= '9':
			appendPart(repl[last:i])
			appendPart(capturedString(L, match, str, 2*int(c-'0')))
			last = i + 2
		}
		i++
	}
	appendPart(repl[last:])
}

// strGsubTable returns the value repl holds for the first capture of match,
// and false when it is false or nil.
func strGsubTable(L *LState, str string, repl *LTable, match *pm.MatchData) (string, bool) {
	idx := 0
	if match.CaptureLength() > 2 { // has captures
		idx = 2
	}
	var value LValue
	if match.IsPosCapture(idx) {
		value = L.GetTable(repl, LNumber(match.Capture(idx)))
	} else {
		value = L.GetField(repl, str[match.Capture(idx):match.Capture(idx+1)])
	}
	if LVIsFalse(value) {
		return "", false
	}
	return LVAsString(value), true
}

// strGsubFunc returns what repl returns for the captures of match, and false
// when it returns false or nil.
func strGsubFunc(L *LState, str string, repl *LFunction, match *pm.MatchData) (string, bool) {
	L.Push(repl)
	nargs := 0
	if match.CaptureLength() > 2 { // has captures
		for i := 2; i < match.CaptureLength(); i += 2 {
			if match.IsPosCapture(i) {
				L.Push(LNumber(match.Capture(i)))
			} else {
				L.Push(LString(capturedString(L, match, str, i)))
			}
			nargs++
		}
	} else {
		L.Push(LString(capturedString(L, match, str, 0)))
		nargs++
	}
	L.Call(nargs, 1)
	ret := L.reg.Pop()
	if LVIsFalse(ret) {
		return "", false
	}
	return LVAsString(ret), true
}

type strMatchData struct {
	str     string
	pos     int
	matches []*pm.MatchData
}

func strGmatchIter(L *LState) int {
	md := L.CheckUserData(1).Value.(*strMatchData)
	str := md.str
	matches := md.matches
	idx := md.pos
	md.pos += 1
	if idx == len(matches) {
		return 0
	}
	L.Push(L.Get(1))
	match := matches[idx]
	if match.CaptureLength() == 2 {
		capture := str[match.Capture(0):match.Capture(1)]
		L.TrackAlloc(int64(len(capture)))
		L.Push(LString(capture))
		return 1
	}

	for i := 2; i < match.CaptureLength(); i += 2 {
		if match.IsPosCapture(i) {
			L.Push(LNumber(match.Capture(i)))
		} else {
			capture := str[match.Capture(i):match.Capture(i+1)]
			L.TrackAlloc(int64(len(capture)))
			L.Push(LString(capture))
		}
	}
	return match.CaptureLength()/2 - 1
}

func strGmatch(L *LState) int {
	str := L.CheckString(1)
	pattern := L.CheckString(2)
	mds, err := pm.FindContext(L.ctx, pattern, []byte(str), 0, -1)
	if err != nil {
		L.RaiseError(err.Error())
	}
	L.Push(L.Get(UpvalueIndex(1)))
	ud := L.NewUserData()
	ud.Value = &strMatchData{str, 0, mds}
	L.Push(ud)
	return 2
}

func strLen(L *LState) int {
	str := L.CheckString(1)
	L.Push(LNumber(len(str)))
	return 1
}

func strLower(L *LState) int {
	str := L.CheckString(1)
	result := strings.ToLower(str)
	L.TrackAlloc(int64(len(result)))
	L.Push(LString(result))
	return 1
}

func strMatch(L *LState) int {
	str := L.CheckString(1)
	pattern := L.CheckString(2)
	offset := L.OptInt(3, 1)
	l := len(str)
	if offset < 0 {
		offset = l + offset + 1
	}
	offset--
	if offset < 0 {
		offset = 0
	}

	mds, err := pm.FindContext(L.ctx, pattern, unsafeFastStringToReadOnlyBytes(str), offset, 1)
	if err != nil {
		L.RaiseError(err.Error())
	}
	if len(mds) == 0 {
		L.Push(LNil)
		return 0
	}
	md := mds[0]
	nsubs := md.CaptureLength() / 2
	switch nsubs {
	case 1:
		capture := str[md.Capture(0):md.Capture(1)]
		L.TrackAlloc(int64(len(capture)))
		L.Push(LString(capture))
		return 1
	default:
		for i := 2; i < md.CaptureLength(); i += 2 {
			if md.IsPosCapture(i) {
				L.Push(LNumber(md.Capture(i)))
			} else {
				capture := str[md.Capture(i):md.Capture(i+1)]
				L.TrackAlloc(int64(len(capture)))
				L.Push(LString(capture))
			}
		}
		return nsubs - 1
	}
}

func strRep(L *LState) int {
	str := L.CheckString(1)
	n := L.CheckInt(2)
	if n < 0 {
		L.Push(emptyLString)
	} else {
		// len(str) * n can overflow into a negative size, which used to
		// lower the memory counter instead of raising.
		if len(str) > 0 && n > math.MaxInt/len(str) {
			L.RaiseError("resulting string too large")
		}
		L.TrackAlloc(int64(len(str) * n))
		result := strings.Repeat(str, n)
		L.Push(LString(result))
	}
	return 1
}

func strReverse(L *LState) int {
	str := L.CheckString(1)
	bts := []byte(str)
	out := make([]byte, len(bts))
	for i, j := 0, len(bts)-1; j >= 0; i, j = i+1, j-1 {
		out[i] = bts[j]
	}
	result := string(out)
	L.TrackAlloc(int64(len(result)))
	L.Push(LString(result))
	return 1
}

func strSub(L *LState) int {
	str := L.CheckString(1)
	start := luaIndex2StringIndex(str, L.CheckInt(2), true)
	end := luaIndex2StringIndex(str, L.OptInt(3, -1), false)
	l := len(str)
	if start >= l || end < start {
		L.Push(emptyLString)
	} else {
		result := str[start:end]
		L.TrackAlloc(int64(len(result)))
		L.Push(LString(result))
	}
	return 1
}

func strUpper(L *LState) int {
	str := L.CheckString(1)
	result := strings.ToUpper(str)
	L.TrackAlloc(int64(len(result)))
	L.Push(LString(result))
	return 1
}

func luaIndex2StringIndex(str string, i int, start bool) int {
	if start && i != 0 {
		i -= 1
	}
	l := len(str)
	if i < 0 {
		i = l + i + 1
	}
	i = intMax(0, i)
	if !start && i > l {
		i = l
	}
	return i
}

//
