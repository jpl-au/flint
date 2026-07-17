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

		// Read the function for this accumulator, scope-aware: a same-named
		// variable in another scope is a different variable, not this one.
		sc := scanAccumulator(body, name, declIdent)
		if sc.splats != 1 {
			continue
		}

		// An append inside a defer or goroutine runs after the element has
		// already taken the slice, so those children never reach the output.
		if sc.tooLate {
			diags = append(diags, Diagnostic{
				Check:    "node-append",
				Pos:      fset.Position(declIdent.Pos()),
				End:      fset.Position(sc.splat.End()),
				Severity: Warning,
				Message:  fmt.Sprintf("%q is appended to inside a defer or goroutine, after it has already been passed in - those children will not appear", name),
				Fix:      "build the children before passing the slice in; an append in a defer or goroutine runs too late to reach the element",
			})
			continue
		}

		// No appends, or the slice is used in some other way (read elsewhere,
		// mutated in a plain closure, indexed, re-sliced): stay quiet rather than
		// suggest a rewrite that may not be safe.
		if sc.appends == 0 || sc.other {
			continue
		}

		// Only a sink that resolves to a Fluent package (div.New, ul.New, an
		// inline div.New().Add, ...) is a constructor we can name. A plain
		// function or a method on a local of unknown type does not resolve, so
		// the advice drops the element-specific wording rather than inventing a
		// constructor that is not there.
		_, intoElement := chainPackage(sc.splat.Fun, imports, l.registry)
		fix := nodeAppendFix(sc.class, intoElement)

		// make([]node.Node, n) with a non-zero length seeds n nil entries and then
		// doubles the slice on append - a genuine slip, so keep it a Warning with
		// the note. Otherwise the accumulator is correct: building it and splatting
		// is the cheapest render-once option, and composing with Fluent is an
		// idiomatic alternative rather than a fix. Report that as advisory (Info),
		// not a defect.
		if makeWithLength(stmt) {
			fix += fmt.Sprintf("; note: make([]node.Node, n) seeds %q with n nil entries before the appended children - use make([]node.Node, 0, n) to reserve capacity", name)
			message := fmt.Sprintf("compose these children with Fluent instead of accumulating %q with append", name)
			if intoElement {
				message = fmt.Sprintf("build the element's children with Fluent composition instead of accumulating %q with append", name)
			}
			diags = append(diags, Diagnostic{
				Check:    "node-append",
				Pos:      fset.Position(declIdent.Pos()),
				End:      fset.Position(sc.splat.End()),
				Severity: Warning,
				Message:  message,
				Fix:      fix,
			})
			continue
		}

		diags = append(diags, Diagnostic{
			Check:    "node-append",
			Pos:      fset.Position(declIdent.Pos()),
			End:      fset.Position(sc.splat.End()),
			Severity: Info,
			Message:  fmt.Sprintf("%q is assembled with append; Fluent can compose these children directly", name),
			Fix:      fix,
		})
	}

	return diags
}
