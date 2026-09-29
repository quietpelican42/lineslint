// Package lineslint provides a go/analysis analyzer that reports uses of
// strings.Lines and bytes.Lines elements that silently misbehave because every
// yielded line keeps its trailing "\n" (or "\r\n").
//
// The analyzer follows the range variable of "for line := range strings.Lines(s)"
// through simple assignments and reports it only when it reaches a known sink
// (a comparison with a constant, a map key, a file path, a strconv or net
// parser, an exec argument, a HasSuffix check) without first passing through
// a call that removes the newline. Anything it cannot classify is left alone.
package lineslint

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Doc is the analyzer documentation shown by "lineslint help".
const Doc = `report strings.Lines and bytes.Lines elements used as if the trailing newline were absent

Every element yielded by strings.Lines or bytes.Lines keeps its line
terminator ("\n" or "\r\n"). Comparing an element with a string constant,
using it as a map key or a file name, or parsing it with strconv, net or
net/url silently fails for every terminated line. The analyzer follows the
range variable through simple assignments and reports it at those sinks
unless TrimSpace, TrimSuffix, CutSuffix, TrimRight or similar cleaned it
first. Values it cannot classify are never reported.`

// Analyzer reports misuse of the trailing newline in strings.Lines and
// bytes.Lines elements.
var Analyzer = &analysis.Analyzer{
	Name:     "lineslint",
	Doc:      Doc,
	URL:      "https://github.com/__GH_OWNER__/lineslint",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

type srcKind int

const (
	srcNone  srcKind = iota
	srcSeq           // an iter.Seq of lines: the first range variable is a line
	srcSlice         // a slice of lines (slices.Collect): the second range variable is a line
)

func run(pass *analysis.Pass) (any, error) {
	in := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	c := &checker{
		pass:     pass,
		reported: map[token.Pos]bool{},
		defs:     map[*types.Var]ast.Expr{},
		assigns:  map[*types.Var]int{},
		unsafe:   map[*types.Var]bool{},
		reassign: map[*types.Var]bool{},
	}
	c.prescan(in)

	in.Preorder([]ast.Node{(*ast.RangeStmt)(nil), (*ast.CallExpr)(nil)}, func(n ast.Node) {
		switch n := n.(type) {
		case *ast.RangeStmt:
			c.startRange(n)
		case *ast.CallExpr:
			c.startDirectCall(n)
		}
	})
	return nil, nil
}

type checker struct {
	pass     *analysis.Pass
	reported map[token.Pos]bool

	defs     map[*types.Var]ast.Expr // the initializer of each variable, when it has exactly one
	assigns  map[*types.Var]int      // how many times each variable is initialized or assigned
	unsafe   map[*types.Var]bool     // address taken, or assigned inside a closure that captured it
	reassign map[*types.Var]bool     // assigned with = (or op=, ++, range) after its declaration
}

// prescan collects the facts the flow walk relies on: which variables have a
// single defining expression (for stored iter.Seq values), which have their
// address taken, and which are reassigned.
func (c *checker) prescan(in *inspector.Inspector) {
	info := c.pass.TypesInfo
	varOf := func(e ast.Expr) *types.Var {
		id, ok := ast.Unparen(e).(*ast.Ident)
		if !ok {
			return nil
		}
		v, _ := info.ObjectOf(id).(*types.Var)
		return v
	}
	noteAssign := func(e ast.Expr, rhs ast.Expr, closure *ast.FuncLit) {
		v := varOf(e)
		if v == nil {
			return
		}
		c.assigns[v]++
		c.defs[v] = rhs
		if id, ok := ast.Unparen(e).(*ast.Ident); ok {
			if _, isDef := info.Defs[id]; !isDef {
				c.reassign[v] = true
			}
		}
		if closure != nil && (v.Pos() < closure.Pos() || v.Pos() >= closure.End()) {
			c.unsafe[v] = true
		}
	}

	var closures []*ast.FuncLit
	in.Nodes(nil, func(n ast.Node, push bool) bool {
		if fl, ok := n.(*ast.FuncLit); ok {
			if push {
				closures = append(closures, fl)
			} else {
				closures = closures[:len(closures)-1]
			}
			return true
		}
		if !push {
			return true
		}
		var cl *ast.FuncLit
		if len(closures) > 0 {
			cl = closures[len(closures)-1]
		}
		switch n := n.(type) {
		case *ast.AssignStmt:
			for i, l := range n.Lhs {
				var rhs ast.Expr
				if len(n.Lhs) == len(n.Rhs) {
					rhs = n.Rhs[i]
				}
				if n.Tok != token.ASSIGN && n.Tok != token.DEFINE {
					// op= is a reassignment with no usable initializer.
					if v := varOf(l); v != nil {
						c.assigns[v] += 2
						c.reassign[v] = true
						if cl != nil && (v.Pos() < cl.Pos() || v.Pos() >= cl.End()) {
							c.unsafe[v] = true
						}
					}
					continue
				}
				noteAssign(l, rhs, cl)
			}
		case *ast.ValueSpec:
			if len(n.Values) == 0 {
				return true
			}
			for i, name := range n.Names {
				var rhs ast.Expr
				if len(n.Names) == len(n.Values) {
					rhs = n.Values[i]
				}
				noteAssign(name, rhs, cl)
			}
		case *ast.IncDecStmt:
			if v := varOf(n.X); v != nil {
				c.assigns[v] += 2
				c.reassign[v] = true
				if cl != nil && (v.Pos() < cl.Pos() || v.Pos() >= cl.End()) {
					c.unsafe[v] = true
				}
			}
		case *ast.RangeStmt:
			for _, e := range []ast.Expr{n.Key, n.Value} {
				if e != nil {
					noteAssign(e, nil, cl)
				}
			}
		case *ast.UnaryExpr:
			if n.Op == token.AND {
				if v := varOf(n.X); v != nil {
					c.unsafe[v] = true
				}
			}
		}
		return true
	})
}

// startRange begins a flow walk at a range statement over a Lines source.
func (c *checker) startRange(rs *ast.RangeStmt) {
	kind := c.source(rs.X, 0)
	if kind == srcNone {
		return
	}
	target := rs.Key
	if kind == srcSlice {
		target = rs.Value
	}
	v := c.lineVar(target)
	if v == nil {
		return
	}
	w := &walker{c: c}
	w.block(rs.Body.List, state{v: kindRaw})
}

// startDirectCall handles strings.Lines(s)(func(line string) bool { ... }).
func (c *checker) startDirectCall(call *ast.CallExpr) {
	if len(call.Args) != 1 {
		return
	}
	fl, ok := ast.Unparen(call.Args[0]).(*ast.FuncLit)
	if !ok || c.source(call.Fun, 0) != srcSeq {
		return
	}
	params := fl.Type.Params.List
	if len(params) == 0 || len(params[0].Names) == 0 {
		return
	}
	v := c.lineVar(params[0].Names[0])
	if v == nil {
		return
	}
	w := &walker{c: c}
	w.block(fl.Body.List, state{v: kindRaw})
}

// lineVar returns the variable named by e when it is safe to track.
func (c *checker) lineVar(e ast.Expr) *types.Var {
	if e == nil {
		return nil
	}
	id, ok := ast.Unparen(e).(*ast.Ident)
	if !ok || id.Name == "_" {
		return nil
	}
	v, _ := c.pass.TypesInfo.ObjectOf(id).(*types.Var)
	if v == nil || c.unsafe[v] {
		return nil
	}
	return v
}

// source classifies e as a Lines iterator, a slice collected from one, or
// neither. A stored iterator is followed when the variable is assigned once.
func (c *checker) source(e ast.Expr, depth int) srcKind {
	if depth > 4 {
		return srcNone
	}
	switch e := ast.Unparen(e).(type) {
	case *ast.CallExpr:
		fn := c.callee(e)
		switch {
		case isFunc(fn, "strings", "Lines"), isFunc(fn, "bytes", "Lines"):
			return srcSeq
		case isFunc(fn, "slices", "Collect") && len(e.Args) == 1:
			if c.source(e.Args[0], depth+1) == srcSeq {
				return srcSlice
			}
		}
	case *ast.Ident:
		v, _ := c.pass.TypesInfo.ObjectOf(e).(*types.Var)
		if v == nil || c.unsafe[v] || c.assigns[v] != 1 || c.defs[v] == nil {
			return srcNone
		}
		return c.source(c.defs[v], depth+1)
	}
	return srcNone
}

// callee returns the package-level function called by call, if any.
func (c *checker) callee(call *ast.CallExpr) *types.Func {
	fun := ast.Unparen(call.Fun)
	switch f := fun.(type) {
	case *ast.IndexExpr:
		fun = ast.Unparen(f.X)
	case *ast.IndexListExpr:
		fun = ast.Unparen(f.X)
	}
	var id *ast.Ident
	switch f := fun.(type) {
	case *ast.Ident:
		id = f
	case *ast.SelectorExpr:
		id = f.Sel
	default:
		return nil
	}
	fn, _ := c.pass.TypesInfo.Uses[id].(*types.Func)
	if fn == nil || fn.Pkg() == nil {
		return nil
	}
	if sig, ok := fn.Type().(*types.Signature); !ok || sig.Recv() != nil {
		return nil
	}
	return fn
}

func isFunc(fn *types.Func, pkg, name string) bool {
	return fn != nil && fn.Name() == name && fn.Pkg().Path() == pkg
}

func (c *checker) report(n ast.Node, format string, args ...any) {
	if c.reported[n.Pos()] {
		return
	}
	c.reported[n.Pos()] = true
	c.pass.Reportf(n.Pos(), format, args...)
}
