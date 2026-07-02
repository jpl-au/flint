package flint

import (
	"fmt"
	"go/ast"
	"go/token"
	"strconv"
)

// bufferHintThreshold mirrors fluent's buffer pool small/large boundary
// (pool.poolThreshold, 4 KiB). A render whose output clears it is served from the
// large pool, where a BufferHint lets the buffer be reused across renders rather
// than regrown from scratch each time. flint has no dependency on fluent, so the
// value is duplicated here as a heuristic threshold.
const bufferHintThreshold = 4096

// checkBufferHint suggests BufferHint(n) on a Fluent element whose statically
// visible content is already large enough to clear the buffer pool's small/large
// boundary. BufferHint pre-sizes the pooled render buffer; above the threshold it
// is what lets fluent reuse the buffer between renders. It is opt-in and harmless -
// the worst a wrong guess does is route the buffer to a different pool - so this is
// advisory (Info), and the size gate keeps it quiet on all but genuinely large
// trees.
func (l *Linter) checkBufferHint(fset *token.FileSet, file *ast.File) []Diagnostic {
	if l.registry == nil {
		return nil
	}
	imports := resolveImports(file)

	var diags []Diagnostic
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		// Only a chain that resolves to a Fluent element (one with a BufferHint
		// method) is a candidate. This is the first such call met top-down, so it
		// is the outermost element in its expression; handle it and do not descend,
		// so its children are not flagged in their own right (the parent's size
		// already includes them).
		pkg, ok := chainPackage(call.Fun, imports, l.registry)
		if !ok {
			return true
		}
		if _, hasHint := pkg.Methods["BufferHint"]; !hasHint {
			return true
		}

		if !chainHasBufferHint(call) {
			if size := staticSize(call); size >= bufferHintThreshold {
				diags = append(diags, Diagnostic{
					Pos:      fset.Position(call.Pos()),
					End:      fset.Position(call.End()),
					Severity: Info,
					Message:  fmt.Sprintf("this element renders at least %d bytes of static content", size),
					Fix:      fmt.Sprintf("chain .BufferHint(%d) to pre-size the pooled render buffer - for a render this large it lets the buffer be reused between renders instead of regrown each time", size),
				})
			}
		}
		return false
	})
	return diags
}

// staticSize estimates the bytes the expression will render by summing the
// length of every string literal it contains. Dynamic content (variables,
// loops, conditionals) counts as zero and tag markup is not counted, which
// pulls the estimate down; a literal in a non-rendered position (a comparison
// inside a closure, say) counts even though it never renders, which pulls it
// up. In typical trees the figure is a lower bound on the render size, and it
// only ever gates an advisory hint, so the imprecision is acceptable.
func staticSize(expr ast.Expr) int {
	total := 0
	ast.Inspect(expr, func(n ast.Node) bool {
		if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			if s, err := strconv.Unquote(lit.Value); err == nil {
				total += len(s)
			}
		}
		return true
	})
	return total
}

// chainHasBufferHint reports whether the method chain rooted at call already
// includes a .BufferHint(...) call, so the suggestion is not repeated.
func chainHasBufferHint(call *ast.CallExpr) bool {
	for {
		if calleeName(call) == "BufferHint" {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		inner, ok := sel.X.(*ast.CallExpr)
		if !ok {
			return false
		}
		call = inner
	}
}
