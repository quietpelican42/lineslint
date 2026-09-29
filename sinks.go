package lineslint

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"
)

// dirty reports whether e still carries the line terminator. Only the forms
// listed here propagate it; every other expression is treated as clean, which
// is what makes unknown calls and unrecognized flows silent.
func (w *walker) dirty(e ast.Expr, st state) kind {
	info := w.c.pass.TypesInfo
	switch e := ast.Unparen(e).(type) {
	case *ast.Ident:
		if v, ok := info.ObjectOf(e).(*types.Var); ok {
			return st[v]
		}
	case *ast.BinaryExpr:
		// "prefix" + line ends with line.
		if e.Op == token.ADD {
			if w.dirty(e.Y, st) != kindClean {
				return kindTail
			}
		}
	case *ast.SliceExpr:
		// line[i:] keeps the tail. Any explicit upper bound (line[:len(line)-1])
		// is a cut this analyzer does not try to interpret.
		if e.High == nil && !e.Slice3 {
			if k := w.dirty(e.X, st); k != kindClean {
				return kindTail
			}
		}
	case *ast.CallExpr:
		return w.dirtyCall(e, st)
	}
	return kindClean
}

func (w *walker) dirtyCall(call *ast.CallExpr, st state) kind {
	info := w.c.pass.TypesInfo
	// string(line) and []byte(line).
	if tv, ok := info.Types[call.Fun]; ok && tv.IsType() {
		if len(call.Args) == 1 && isStringOrBytes(tv.Type) {
			return w.dirty(call.Args[0], st)
		}
		return kindClean
	}
	fn := w.c.callee(call)
	if fn == nil {
		return kindClean
	}
	if p := fn.Pkg().Path(); p != "strings" && p != "bytes" {
		return kindClean
	}
	if len(call.Args) == 0 {
		return kindClean
	}
	k := w.dirty(call.Args[0], st)
	if k == kindClean {
		return kindClean
	}
	switch fn.Name() {
	case "ToLower", "ToUpper", "ToTitle", "Clone":
		return k
	case "TrimPrefix", "TrimLeft", "TrimLeftFunc":
		return kindTail
	case "TrimSuffix":
		if len(call.Args) == 2 {
			if s, ok := w.constString(call.Args[1]); ok && !strings.HasSuffix(s, "\n") {
				return kindTail
			}
		}
	case "Trim", "TrimRight":
		if len(call.Args) == 2 {
			if s, ok := w.constString(call.Args[1]); ok && !strings.Contains(s, "\n") {
				return kindTail
			}
		}
	}
	// TrimSpace, TrimFunc, TrimRightFunc, CutSuffix, Fields, Split and the
	// rest either remove the terminator or cannot be classified.
	return kindClean
}

func isStringOrBytes(t types.Type) bool {
	switch u := t.Underlying().(type) {
	case *types.Basic:
		return u.Kind() == types.String
	case *types.Slice:
		b, ok := u.Elem().Underlying().(*types.Basic)
		return ok && b.Kind() == types.Byte
	}
	return false
}

func (w *walker) checkCompare(b *ast.BinaryExpr, st state) {
	if b.Op != token.EQL && b.Op != token.NEQ {
		return
	}
	for _, pair := range [][2]ast.Expr{{b.X, b.Y}, {b.Y, b.X}} {
		k := w.dirty(pair[0], st)
		if k == kindClean {
			continue
		}
		lit, ok := w.constString(pair[1])
		if !ok || !w.matchesNeverOrRarely(lit, k) {
			continue
		}
		verb := "never equals"
		if b.Op == token.NEQ {
			verb = "is never equal to"
		}
		if lit == "" {
			w.c.report(b, "%s is never empty: strings.Lines and bytes.Lines yield a blank line as %q, so comparing with \"\" cannot detect it",
				w.desc(pair[0]), "\n")
			return
		}
		w.c.report(b, "%s%s, so it %s %q (%s)", w.desc(pair[0]), mustEndNL, verb, lit, fixHint)
		return
	}
}

// pathSink describes which arguments of a function are checked.
type pathSink struct {
	what string
	args []int // indexes to check; nil means every argument
	from int   // when args is nil, the first argument to check
}

var sinkFuncs = map[string]pathSink{
	"os.Stat":      {what: "a file path", args: []int{0}},
	"os.Lstat":     {what: "a file path", args: []int{0}},
	"os.Open":      {what: "a file path", args: []int{0}},
	"os.OpenFile":  {what: "a file path", args: []int{0}},
	"os.ReadFile":  {what: "a file path", args: []int{0}},
	"os.WriteFile": {what: "a file path", args: []int{0}},
	"os.Create":    {what: "a file path", args: []int{0}},
	"os.ReadDir":   {what: "a file path", args: []int{0}},
	"os.Remove":    {what: "a file path", args: []int{0}},
	"os.RemoveAll": {what: "a file path", args: []int{0}},
	"os.Mkdir":     {what: "a file path", args: []int{0}},
	"os.MkdirAll":  {what: "a file path", args: []int{0}},
	"os.Chdir":     {what: "a file path", args: []int{0}},
	"os.Chmod":     {what: "a file path", args: []int{0}},
	"os.Readlink":  {what: "a file path", args: []int{0}},
	"os.Truncate":  {what: "a file path", args: []int{0}},
	"os.Rename":    {what: "a file path", args: []int{0, 1}},
	"os.Symlink":   {what: "a file path", args: []int{0, 1}},

	"path/filepath.Join":         {what: "a file path"},
	"path/filepath.Base":         {what: "a file path"},
	"path/filepath.Dir":          {what: "a file path"},
	"path/filepath.Ext":          {what: "a file path"},
	"path/filepath.Abs":          {what: "a file path"},
	"path/filepath.Clean":        {what: "a file path"},
	"path/filepath.Rel":          {what: "a file path"},
	"path/filepath.Glob":         {what: "a file path"},
	"path/filepath.EvalSymlinks": {what: "a file path"},
	"path/filepath.WalkDir":      {what: "a file path", args: []int{0}},
	"path/filepath.Walk":         {what: "a file path", args: []int{0}},

	"os/exec.Command":        {what: "a command name or argument"},
	"os/exec.CommandContext": {what: "a command name or argument", from: 1},
	"os/exec.LookPath":       {what: "a command name", args: []int{0}},

	"strconv.Atoi":       {what: "a number to parse", args: []int{0}},
	"strconv.ParseInt":   {what: "a number to parse", args: []int{0}},
	"strconv.ParseUint":  {what: "a number to parse", args: []int{0}},
	"strconv.ParseFloat": {what: "a number to parse", args: []int{0}},
	"strconv.ParseBool":  {what: "a boolean to parse", args: []int{0}},

	"net.ParseIP":             {what: "an address to parse", args: []int{0}},
	"net.ParseCIDR":           {what: "an address to parse", args: []int{0}},
	"net/netip.ParseAddr":     {what: "an address to parse", args: []int{0}},
	"net/url.Parse":           {what: "a URL to parse", args: []int{0}},
	"net/url.ParseRequestURI": {what: "a URL to parse", args: []int{0}},
}

func (w *walker) checkCall(call *ast.CallExpr, st state) {
	if w.isBuiltin(call, "delete") && len(call.Args) == 2 {
		if w.dirty(call.Args[1], st) != kindClean {
			w.c.report(call, "%s%s, so this map key never matches the same text without the terminator (%s)",
				w.desc(call.Args[1]), mustEndNL, fixHint)
		}
		return
	}
	fn := w.c.callee(call)
	if fn == nil {
		return
	}
	full := fn.Pkg().Path() + "." + fn.Name()
	switch full {
	case "strings.HasSuffix", "bytes.HasSuffix", "strings.EqualFold", "bytes.Equal", "bytes.EqualFold":
		w.checkSuffixOrEqual(call, fn, st)
		return
	}
	sink, ok := sinkFuncs[full]
	if !ok {
		return
	}
	check := func(i int) bool {
		if i >= len(call.Args) {
			return false
		}
		if w.dirty(call.Args[i], st) == kindClean {
			return false
		}
		w.c.report(call, "%s%s, so %s cannot use it as %s (%s)",
			w.desc(call.Args[i]), mustEndNL, funcName(fn), sink.what, fixHint)
		return true
	}
	if sink.args != nil {
		for _, i := range sink.args {
			if check(i) {
				return
			}
		}
		return
	}
	for i := sink.from; i < len(call.Args); i++ {
		if check(i) {
			return
		}
	}
}

func funcName(fn *types.Func) string {
	p := fn.Pkg().Path()
	return p[strings.LastIndex(p, "/")+1:] + "." + fn.Name()
}

// checkSuffixOrEqual handles HasSuffix(line, "x"), EqualFold and bytes.Equal.
func (w *walker) checkSuffixOrEqual(call *ast.CallExpr, fn *types.Func, st state) {
	if len(call.Args) != 2 {
		return
	}
	isSuffix := fn.Name() == "HasSuffix"
	pairs := [][2]ast.Expr{{call.Args[0], call.Args[1]}}
	if !isSuffix {
		pairs = append(pairs, [2]ast.Expr{call.Args[1], call.Args[0]})
	}
	for _, pair := range pairs {
		k := w.dirty(pair[0], st)
		if k == kindClean {
			continue
		}
		lit, ok := w.constString(pair[1])
		if !ok || lit == "" && isSuffix || !w.matchesNeverOrRarely(lit, k) {
			continue
		}
		w.c.report(call, "%s%s, so %s with %q is false for every terminated line (%s)",
			w.desc(pair[0]), mustEndNL, funcName(fn), lit, fixHint)
		return
	}
}

func (w *walker) checkIndex(ix *ast.IndexExpr, st state) {
	tv, ok := w.c.pass.TypesInfo.Types[ix.X]
	if !ok {
		return
	}
	if _, isMap := tv.Type.Underlying().(*types.Map); !isMap {
		return
	}
	if w.dirty(ix.Index, st) != kindClean {
		w.c.report(ix, "%s%s, so this map key differs from the same text without the terminator (%s)",
			w.desc(ix.Index), mustEndNL, fixHint)
	}
}

func (w *walker) checkLiteral(cl *ast.CompositeLit, st state) {
	tv, ok := w.c.pass.TypesInfo.Types[cl]
	if !ok {
		return
	}
	if _, isMap := tv.Type.Underlying().(*types.Map); !isMap {
		return
	}
	for _, el := range cl.Elts {
		kv, ok := el.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if w.dirty(kv.Key, st) != kindClean {
			w.c.report(kv.Key, "%s%s, so this map key differs from the same text without the terminator (%s)",
				w.desc(kv.Key), mustEndNL, fixHint)
		}
	}
}
