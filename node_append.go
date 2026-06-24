package flint

import (
	"fmt"
	"go/ast"
	"go/token"
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
