package flint

import (
	"go/ast"
	"go/token"
)

// checkSetAttributeChain reports attempts to chain method calls after
// SetAttribute. SetAttribute does not return the element (it satisfies
// the node.Element interface which has a void signature), so any
// subsequent method call on the result will fail to compile.
func checkSetAttributeChain(fset *token.FileSet, file *ast.File) []Diagnostic {
	var diags []Diagnostic

	ast.Inspect(file, func(n ast.Node) bool {
		// Look for method calls where the receiver is a SetAttribute call.
		// Pattern: expr.SetAttribute(k, v).Method(...)
		// In the AST this is a CallExpr whose Fun is a SelectorExpr
		// whose X is a CallExpr to SetAttribute.
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		// Check if the receiver (sel.X) is a call to SetAttribute.
		innerCall, ok := sel.X.(*ast.CallExpr)
		if !ok {
			return true
		}

		innerSel, ok := innerCall.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		if innerSel.Sel.Name != "SetAttribute" {
			return true
		}

		diags = append(diags, Diagnostic{
			Pos:     fset.Position(sel.Sel.Pos()),
			End:     fset.Position(sel.Sel.End()),
			Message: "SetAttribute does not return the element; cannot chain ." + sel.Sel.Name + "() after it",
			Fix:     "Call SetAttribute separately, or use SetData/SetAria which do support chaining",
		})

		return true
	})

	return diags
}
