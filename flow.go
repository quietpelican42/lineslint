package lineslint

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strings"
)

// kind describes what is known about the value of a tracked variable.
type kind int

const (
	kindClean kind = iota // not tracked: newline removed or unclassified
	kindTail              // ends in a line terminator (unless it is an unterminated last line)
	kindRaw               // the unmodified line: non-empty, ends in a line terminator
)

// state maps each tracked variable to what is known about it.
type state map[*types.Var]kind

func (s state) clone() state {
	n := make(state, len(s))
	for k, v := range s {
		n[k] = v
	}
	return n
}

// intersect keeps a variable only when both paths leave it tracked.
func intersect(a, b state) state {
	n := state{}
	for k, ka := range a {
		if kb, ok := b[k]; ok {
			if ka != kb {
				ka = kindTail
			}
			n[k] = ka
		}
	}
	return n
}

// walker performs a forward, path-merging walk over a statement list.
// Paths merge by intersection, so a variable is reported only when it still
// carries the newline on every path that reaches the sink.
type walker struct {
	c *checker
}

func (w *walker) block(list []ast.Stmt, st state) (state, bool) {
	for _, s := range list {
		var term bool
		st, term = w.stmt(s, st)
		if term {
			return st, true
		}
	}
	return st, false
}

func (w *walker) stmt(s ast.Stmt, st state) (state, bool) {
	switch s := s.(type) {
	case nil, *ast.EmptyStmt:
		return st, false
	case *ast.BlockStmt:
		return w.block(s.List, st)
	case *ast.LabeledStmt:
		return w.stmt(s.Stmt, st)
	case *ast.ExprStmt:
		w.scan(s.X, st)
		if call, ok := ast.Unparen(s.X).(*ast.CallExpr); ok && w.isBuiltin(call, "panic") {
			return st, true
		}
		return st, false
	case *ast.SendStmt:
		w.scan(s.Chan, st)
		w.scan(s.Value, st)
		return st, false
	case *ast.IncDecStmt:
		w.scan(s.X, st)
		w.forget(s.X, st)
		return st, false
	case *ast.GoStmt:
		w.scan(s.Call, st)
		return st, false
	case *ast.DeferStmt:
		w.scan(s.Call, st)
		return st, false
	case *ast.ReturnStmt:
		for _, r := range s.Results {
			w.scan(r, st)
		}
		return st, true
	case *ast.BranchStmt:
		return st, s.Tok != token.FALLTHROUGH
	case *ast.AssignStmt:
		w.assign(s, st)
		return st, false
	case *ast.DeclStmt:
		w.decl(s, st)
		return st, false
	case *ast.IfStmt:
		return w.ifStmt(s, st)
	case *ast.ForStmt:
		st, _ = w.stmt(s.Init, st)
		w.scan(s.Cond, st)
		body, _ := w.block(s.Body.List, st.clone())
		w.stmt(s.Post, body)
		w.dropAssigned(s, st)
		return st, false
	case *ast.RangeStmt:
		return w.rangeStmt(s, st)
	case *ast.SwitchStmt:
		return w.switchStmt(s, st)
	case *ast.TypeSwitchStmt:
		st, _ = w.stmt(s.Init, st)
		st, _ = w.stmt(s.Assign, st)
		return w.clauses(s, s.Body.List, st, nil)
	case *ast.SelectStmt:
		return w.clauses(s, s.Body.List, st, nil)
	}
	return st, false
}

func (w *walker) ifStmt(s *ast.IfStmt, st state) (state, bool) {
	st, _ = w.stmt(s.Init, st)
	w.scan(s.Cond, st)
	thenSt, thenTerm := w.block(s.Body.List, st.clone())
	elseSt, elseTerm := st, false
	if s.Else != nil {
		elseSt, elseTerm = w.stmt(s.Else, st.clone())
	}
	switch {
	case thenTerm && elseTerm:
		return st, true
	case thenTerm:
		return elseSt, false
	case elseTerm:
		return thenSt, false
	}
	return intersect(thenSt, elseSt), false
}

func (w *walker) rangeStmt(s *ast.RangeStmt, st state) (state, bool) {
	w.scan(s.X, st)
	body := st.clone()
	for _, e := range []ast.Expr{s.Key, s.Value} {
		w.forget(e, body)
	}
	if src := w.c.source(s.X, 0); src != srcNone {
		target := s.Key
		if src == srcSlice {
			target = s.Value
		}
		if v := w.c.lineVar(target); v != nil {
			body[v] = kindRaw
		}
	}
	w.block(s.Body.List, body)
	w.dropAssigned(s, st)
	return st, false
}

func (w *walker) switchStmt(s *ast.SwitchStmt, st state) (state, bool) {
	st, _ = w.stmt(s.Init, st)
	if s.Tag != nil {
		w.scan(s.Tag, st)
		w.switchTag(s, st)
	}
	return w.clauses(s, s.Body.List, st, s.Tag)
}

// clauses walks switch, type switch and select clauses and merges their
// results. A construct that may exit through break has every variable it
// assigns dropped, since the state at the break is not tracked.
func (w *walker) clauses(n ast.Node, list []ast.Stmt, st state, tag ast.Expr) (state, bool) {
	var merged state
	first, hasDefault := true, false
	add := func(s state) {
		if first {
			merged, first = s, false
		} else {
			merged = intersect(merged, s)
		}
	}
	for _, cl := range list {
		var body []ast.Stmt
		cs := st.clone()
		switch cl := cl.(type) {
		case *ast.CaseClause:
			hasDefault = hasDefault || cl.List == nil
			for _, e := range cl.List {
				w.scan(e, st)
			}
			body = cl.Body
		case *ast.CommClause:
			hasDefault = hasDefault || cl.Comm == nil
			cs, _ = w.stmt(cl.Comm, cs)
			body = cl.Body
		}
		out, term := w.block(body, cs)
		if !term {
			add(out)
		}
	}
	if !hasDefault {
		add(st)
	}
	if first {
		merged = st
	}
	w.dropAssigned(n, merged)
	return merged, false
}

// switchTag reports "switch line { case "x": ... }".
func (w *walker) switchTag(s *ast.SwitchStmt, st state) {
	k := w.dirty(s.Tag, st)
	if k == kindClean {
		return
	}
	for _, cl := range s.Body.List {
		cc, ok := cl.(*ast.CaseClause)
		if !ok {
			continue
		}
		for _, e := range cc.List {
			if lit, ok := w.constString(e); ok && w.matchesNeverOrRarely(lit, k) {
				w.c.report(s.Tag, "%s%s, and cannot match case %q (%s)",
					w.desc(s.Tag), mustEndNL, lit, fixHint)
				return
			}
		}
	}
}

const (
	mustEndNL = ` still ends in "\n" (Lines keeps line terminators)`
	fixHint   = `trim it first`
)

func (w *walker) desc(e ast.Expr) string { return types.ExprString(e) }

// matchesNeverOrRarely reports whether comparing a dirty value with lit is
// wrong: lit does not end in a newline, or lit is "" and the value is the
// untouched (and therefore never empty) line.
func (w *walker) matchesNeverOrRarely(lit string, k kind) bool {
	if lit == "" {
		return k == kindRaw
	}
	return !strings.HasSuffix(lit, "\n")
}

// assign updates the tracked state for an assignment.
func (w *walker) assign(s *ast.AssignStmt, st state) {
	for _, r := range s.Rhs {
		w.scan(r, st)
	}
	for _, l := range s.Lhs {
		if _, ok := ast.Unparen(l).(*ast.Ident); !ok {
			w.scan(l, st)
		}
	}
	switch {
	case s.Tok == token.ASSIGN || s.Tok == token.DEFINE:
		w.bind(s.Lhs, s.Rhs, st)
	case s.Tok == token.ADD_ASSIGN && len(s.Lhs) == 1 && len(s.Rhs) == 1:
		// x += y ends with y.
		k := w.dirty(s.Rhs[0], st)
		w.set(s.Lhs[0], k, st)
	default:
		for _, l := range s.Lhs {
			w.forget(l, st)
		}
	}
}

func (w *walker) decl(s *ast.DeclStmt, st state) {
	gd, ok := s.Decl.(*ast.GenDecl)
	if !ok || gd.Tok != token.VAR {
		return
	}
	for _, sp := range gd.Specs {
		vs, ok := sp.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for _, v := range vs.Values {
			w.scan(v, st)
		}
		lhs := make([]ast.Expr, len(vs.Names))
		for i, n := range vs.Names {
			lhs[i] = n
		}
		if len(vs.Values) == 0 {
			for _, l := range lhs {
				w.forget(l, st)
			}
			continue
		}
		w.bind(lhs, vs.Values, st)
	}
}

// bind applies lhs = rhs to the state.
func (w *walker) bind(lhs, rhs []ast.Expr, st state) {
	if len(lhs) == len(rhs) {
		ks := make([]kind, len(rhs))
		for i, r := range rhs {
			ks[i] = w.dirty(r, st)
		}
		for i, l := range lhs {
			w.set(l, ks[i], st)
		}
		return
	}
	for _, l := range lhs {
		w.forget(l, st)
	}
	if len(rhs) != 1 {
		return
	}
	call, ok := ast.Unparen(rhs[0]).(*ast.CallExpr)
	if !ok || len(call.Args) != 2 {
		return
	}
	fn := w.c.callee(call)
	if w.dirty(call.Args[0], st) == kindClean {
		return
	}
	sep, isConst := w.constString(call.Args[1])
	switch {
	case (isFunc(fn, "strings", "Cut") || isFunc(fn, "bytes", "Cut")) && len(lhs) == 3:
		// after is the tail of the line, so it keeps the terminator, unless
		// the separator is itself a terminator.
		if isConst && !strings.Contains(sep, "\n") {
			w.set(lhs[1], kindTail, st)
		}
	case (isFunc(fn, "strings", "CutPrefix") || isFunc(fn, "bytes", "CutPrefix")) && len(lhs) == 2:
		if isConst && !strings.Contains(sep, "\n") {
			w.set(lhs[0], kindTail, st)
		}
	}
}

// set records k for the variable named by e, or forgets it.
func (w *walker) set(e ast.Expr, k kind, st state) {
	id, ok := ast.Unparen(e).(*ast.Ident)
	if !ok {
		return
	}
	v, _ := w.c.pass.TypesInfo.ObjectOf(id).(*types.Var)
	if v == nil {
		return
	}
	if k == kindClean || w.c.unsafe[v] {
		delete(st, v)
		return
	}
	st[v] = k
}

func (w *walker) forget(e ast.Expr, st state) {
	if e != nil {
		w.set(e, kindClean, st)
	}
}

// dropAssigned removes every variable assigned anywhere inside n.
func (w *walker) dropAssigned(n ast.Node, st state) {
	info := w.c.pass.TypesInfo
	drop := func(e ast.Expr) {
		if id, ok := ast.Unparen(e).(*ast.Ident); ok {
			if v, ok := info.ObjectOf(id).(*types.Var); ok {
				delete(st, v)
			}
		}
	}
	ast.Inspect(n, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			for _, l := range n.Lhs {
				drop(l)
			}
		case *ast.IncDecStmt:
			drop(n.X)
		case *ast.RangeStmt:
			if n.Key != nil {
				drop(n.Key)
			}
			if n.Value != nil {
				drop(n.Value)
			}
		}
		return true
	})
}

func (w *walker) isBuiltin(call *ast.CallExpr, name string) bool {
	id, ok := ast.Unparen(call.Fun).(*ast.Ident)
	if !ok {
		return false
	}
	b, ok := w.c.pass.TypesInfo.Uses[id].(*types.Builtin)
	return ok && b.Name() == name
}

// constString returns the value of e when it is a constant string, or a
// []byte conversion of one.
func (w *walker) constString(e ast.Expr) (string, bool) {
	e = ast.Unparen(e)
	info := w.c.pass.TypesInfo
	if tv, ok := info.Types[e]; ok && tv.Value != nil && tv.Value.Kind() == constant.String {
		return constant.StringVal(tv.Value), true
	}
	if call, ok := e.(*ast.CallExpr); ok && len(call.Args) == 1 {
		if tv, ok := info.Types[call.Fun]; ok && tv.IsType() {
			if sl, ok := tv.Type.Underlying().(*types.Slice); ok {
				if b, ok := sl.Elem().Underlying().(*types.Basic); ok && b.Kind() == types.Byte {
					return w.constString(call.Args[0])
				}
			}
		}
	}
	return "", false
}

// scan walks an expression looking for sinks. Function literals are walked
// as statement lists with the current state.
func (w *walker) scan(n ast.Node, st state) {
	if n == nil {
		return
	}
	ast.Inspect(n, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			w.closure(n, st)
			return false
		case *ast.BinaryExpr:
			w.checkCompare(n, st)
		case *ast.CallExpr:
			w.checkCall(n, st)
		case *ast.IndexExpr:
			w.checkIndex(n, st)
		case *ast.CompositeLit:
			w.checkLiteral(n, st)
		}
		return true
	})
}

// closure walks a function literal. It inherits only the variables that are
// never reassigned, since it may run at any later time.
func (w *walker) closure(fl *ast.FuncLit, st state) {
	cs := state{}
	for v, k := range st {
		if !w.c.reassign[v] {
			cs[v] = k
		}
	}
	if len(cs) == 0 {
		// Nothing tracked can reach the closure body, but it may still hold
		// its own Lines loop, which the preorder pass finds separately.
		return
	}
	w.block(fl.Body.List, cs)
}
