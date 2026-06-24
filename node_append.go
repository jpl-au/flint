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

		anyIf, anyLoop := classifyAppends(body, name)
		diags = append(diags, Diagnostic{
			Pos:      fset.Position(declIdent.Pos()),
			End:      fset.Position(splat.End()),
			Severity: Warning,
			Message:  fmt.Sprintf("build the element's children with Fluent composition instead of accumulating %q with append", name),
			Fix:      nodeAppendFix(anyIf, anyLoop),
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

// classifyAppends reports whether any append to name sits inside a conditional
// (an if) or a loop (a for/range), so the fix can name node.When / node.Map.
func classifyAppends(body *ast.BlockStmt, name string) (anyIf, anyLoop bool) {
	for _, stmt := range body.List {
		if !stmtHasAppendTo(stmt, name) {
			continue
		}
		switch stmt.(type) {
		case *ast.ForStmt, *ast.RangeStmt:
			anyLoop = true
		case *ast.IfStmt:
			anyIf = true
		}
	}
	return anyIf, anyLoop
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

// nodeAppendFix builds the fix advice, naming the conditional and loop idioms
// only when the code actually uses those shapes.
func nodeAppendFix(anyIf, anyLoop bool) string {
	var options []string
	if anyIf {
		options = append(options, "node.When(cond, child) for a conditional child")
	}
	if anyLoop {
		options = append(options, "node.Map(items, fn) for a loop")
	}
	options = append(options, "passing children directly to the constructor or via .Add(...)")
	return "compose children with Fluent instead of a []node.Node grown by append: " + strings.Join(options, "; ")
}
