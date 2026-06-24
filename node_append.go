package flint

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
)

// checkNodeAppend reports a local []node.Node accumulator that is grown with
// append and then splatted into a Fluent call, where Fluent's own composition
// expresses the same thing without the intermediate slice: variadic children or
// .Add() for the plain case, node.When/node.Unless for a conditional child, and
// node.Map for a loop.
//
// It is deliberately conservative. It fires only when the slice is a local whose
// element type resolves to a Fluent node (node.Node / node.Element), is grown by
// at least one append, is consumed by exactly one splat (f(v...)), and has no
// other use - not indexed, returned, re-sliced, or passed un-splatted. Those
// guardrails keep it to the cases where the rewrite is mechanical and safe.
func (l *Linter) checkNodeAppend(fset *token.FileSet, file *ast.File) []Diagnostic {
	if l.registry == nil {
		return nil
	}
	imports := resolveImports(file)

	var diags []Diagnostic
	ast.Inspect(file, func(n ast.Node) bool {
		var body *ast.BlockStmt
		switch x := n.(type) {
		case *ast.FuncDecl:
			body = x.Body
		case *ast.FuncLit:
			body = x.Body
		}
		if body != nil {
			diags = append(diags, l.nodeAppendInBody(fset, body, imports)...)
		}
		return true
	})
	return diags
}

// nodeAppendInBody flags qualifying accumulators declared at the top level of a
// single function body. Declarations nested inside control blocks are left alone:
// the pattern this targets - build a slice, grow it, splat it - is a
// function-level shape, and restricting to top-level declarations keeps variable
// scoping unambiguous without a full data-flow pass.
func (l *Linter) nodeAppendInBody(fset *token.FileSet, body *ast.BlockStmt, imports map[string]string) []Diagnostic {
	var diags []Diagnostic

	for _, stmt := range body.List {
		name, declIdent := l.nodeSliceDecl(stmt, imports)
		if name == "" {
			continue
		}

		// One pass to collect the appends, the splat, and the set of identifier
		// occurrences that are legitimate uses of this accumulator.
		blessed := map[*ast.Ident]bool{declIdent: true}
		appendCount := 0
		splatCount := 0
		var splat *ast.CallExpr

		ast.Inspect(body, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.AssignStmt:
				if isAppendTo(x, name) {
					appendCount++
					blessed[x.Lhs[0].(*ast.Ident)] = true
					if arg0, ok := x.Rhs[0].(*ast.CallExpr).Args[0].(*ast.Ident); ok {
						blessed[arg0] = true
					}
				}
			case *ast.CallExpr:
				// A splat is f(..., name...). Exclude append/copy: feeding the slice
				// into another append is a merge, not a sink we can inline.
				if x.Ellipsis.IsValid() && len(x.Args) > 0 {
					if id, ok := x.Args[len(x.Args)-1].(*ast.Ident); ok && id.Name == name {
						if c := calleeName(x); c != "append" && c != "copy" {
							splatCount++
							splat = x
							blessed[id] = true
						}
					}
				}
			}
			return true
		})

		if appendCount == 0 || splatCount != 1 {
			continue
		}

		// Any occurrence of the name that is not a blessed use (the declaration, an
		// append, or the splat) means the slice does more than accumulate-then-splat;
		// stay quiet rather than suggest an unsafe rewrite.
		other := false
		ast.Inspect(body, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok && id.Name == name && !blessed[id] {
				other = true
				return false
			}
			return true
		})
		if other {
			continue
		}

		class := classifyAppends(body, name)
		diags = append(diags, Diagnostic{
			Pos:      fset.Position(declIdent.Pos()),
			End:      fset.Position(splat.End()),
			Severity: Warning,
			Message:  fmt.Sprintf("build the element's children with Fluent composition instead of accumulating %q with append", name),
			Fix:      nodeAppendFix(class),
		})
	}

	return diags
}

// nodeSliceDecl reports the name and declaring identifier when stmt declares a
// local []node.Node: `v := []node.Node{...}`, `v := make([]node.Node, ...)`, or
// `var v []node.Node`. It returns an empty name otherwise.
func (l *Linter) nodeSliceDecl(stmt ast.Stmt, imports map[string]string) (string, *ast.Ident) {
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		if s.Tok != token.DEFINE || len(s.Lhs) != 1 || len(s.Rhs) != 1 {
			return "", nil
		}
		id, ok := s.Lhs[0].(*ast.Ident)
		if !ok || id.Name == "_" {
			return "", nil
		}
		if l.rhsIsNodeSlice(s.Rhs[0], imports) {
			return id.Name, id
		}
	case *ast.DeclStmt:
		gd, ok := s.Decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			return "", nil
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok || len(vs.Names) != 1 || vs.Type == nil {
				continue
			}
			if isNodeSliceType(vs.Type, imports, l.registry) {
				return vs.Names[0].Name, vs.Names[0]
			}
		}
	}
	return "", nil
}

// rhsIsNodeSlice reports whether expr builds a fresh []node.Node, via a composite
// literal or make.
func (l *Linter) rhsIsNodeSlice(expr ast.Expr, imports map[string]string) bool {
	switch r := expr.(type) {
	case *ast.CompositeLit:
		return isNodeSliceType(r.Type, imports, l.registry)
	case *ast.CallExpr:
		if id, ok := r.Fun.(*ast.Ident); ok && id.Name == "make" && len(r.Args) >= 1 {
			return isNodeSliceType(r.Args[0], imports, l.registry)
		}
	}
	return false
}

// isNodeSliceType reports whether expr is a slice type whose element is a Fluent
// node type (node.Node or node.Element), scoped through the registry so only
// Fluent slices match.
func isNodeSliceType(expr ast.Expr, imports map[string]string, reg *Registry) bool {
	arr, ok := expr.(*ast.ArrayType)
	if !ok || arr.Len != nil {
		return false
	}
	sel, ok := arr.Elt.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if sel.Sel.Name != "Node" && sel.Sel.Name != "Element" {
		return false
	}
	_, found := chainPackage(sel, imports, reg)
	return found
}

// isAppendTo reports whether s is `name = append(name, ...)`.
func isAppendTo(s *ast.AssignStmt, name string) bool {
	if s.Tok != token.ASSIGN || len(s.Lhs) != 1 || len(s.Rhs) != 1 {
		return false
	}
	lhs, ok := s.Lhs[0].(*ast.Ident)
	if !ok || lhs.Name != name {
		return false
	}
	call, ok := s.Rhs[0].(*ast.CallExpr)
	if !ok {
		return false
	}
	fn, ok := call.Fun.(*ast.Ident)
	if !ok || fn.Name != "append" || len(call.Args) < 1 {
		return false
	}
	arg0, ok := call.Args[0].(*ast.Ident)
	return ok && arg0.Name == name
}

// conditionalKind describes how an appending if-statement maps onto Fluent's
// conditional helpers.
type conditionalKind int

const (
	condWhen   conditionalKind = iota // node.When(cond, child)
	condUnless                        // node.Unless(cond, child)
	condBoth                          // node.Condition(cond).True(child).False(child)
)

// appendClass records the control-flow shapes that wrap the appends to an
// accumulator, so the fix can name the Fluent idiom matching each one.
type appendClass struct {
	when   bool // a plain conditional child: if cond { append }
	unless bool // a negated or else-only conditional child
	both   bool // an if/else that appends in both branches
	loop   bool // a for/range loop
	branch bool // a switch/select that builds children by branching
}

// classifyAppends inspects the top-level statements that append to name and
// records which Fluent composition idioms the fix should suggest.
func classifyAppends(body *ast.BlockStmt, name string) appendClass {
	var c appendClass
	for _, stmt := range body.List {
		if stmtHasAppendTo(stmt, name) {
			c.classify(stmt, name)
		}
	}
	return c
}

// classify records the idiom implied by a single appending statement, unwrapping
// a labelled statement to the loop or switch it labels.
func (c *appendClass) classify(stmt ast.Stmt, name string) {
	switch s := stmt.(type) {
	case *ast.ForStmt, *ast.RangeStmt:
		c.loop = true
	case *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
		c.branch = true
	case *ast.LabeledStmt:
		c.classify(s.Stmt, name)
	case *ast.IfStmt:
		switch ifKind(s, name) {
		case condUnless:
			c.unless = true
		case condBoth:
			c.both = true
		default:
			c.when = true
		}
	}
}

// ifKind decides which conditional idiom an appending if maps to: Condition when
// both branches append, Unless when only the else branch appends or the condition
// is negated, and When otherwise.
func ifKind(s *ast.IfStmt, name string) conditionalKind {
	thenAppends := stmtHasAppendTo(s.Body, name)
	elseAppends := s.Else != nil && stmtHasAppendTo(s.Else, name)
	switch {
	case thenAppends && elseAppends:
		return condBoth
	case elseAppends:
		return condUnless
	case isNegated(s.Cond):
		return condUnless
	default:
		return condWhen
	}
}

// isNegated reports whether cond is a logical negation (!x), ignoring one layer
// of parentheses.
func isNegated(cond ast.Expr) bool {
	if p, ok := cond.(*ast.ParenExpr); ok {
		cond = p.X
	}
	u, ok := cond.(*ast.UnaryExpr)
	return ok && u.Op == token.NOT
}

// stmtHasAppendTo reports whether stmt contains an append to name anywhere within.
func stmtHasAppendTo(stmt ast.Stmt, name string) bool {
	found := false
	ast.Inspect(stmt, func(n ast.Node) bool {
		if as, ok := n.(*ast.AssignStmt); ok && isAppendTo(as, name) {
			found = true
			return false
		}
		return true
	})
	return found
}

// nodeAppendFix builds the fix advice, naming only the idioms the code's shape
// actually calls for.
func nodeAppendFix(c appendClass) string {
	var options []string
	if c.when {
		options = append(options, "node.When(cond, child) for a conditional child")
	}
	if c.unless {
		options = append(options, "node.Unless(cond, child) for a negated conditional child")
	}
	if c.both {
		options = append(options, "node.Condition(cond).True(child).False(child) for an if/else")
	}
	if c.loop {
		options = append(options, "node.Map(items, fn) for a loop")
	}
	if c.branch {
		options = append(options, "node.Funcs(func() []node.Node { ... }) for branching that builds a slice")
	}
	options = append(options, "passing children directly to the constructor or via .Add(...)")
	return "compose children with Fluent instead of a []node.Node grown by append: " + strings.Join(options, "; ")
}
