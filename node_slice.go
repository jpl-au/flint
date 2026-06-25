package flint

import (
	"go/ast"
	"go/token"
)

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

// makeWithLength reports whether stmt declares a node slice via make with a
// non-zero length argument, e.g. make([]node.Node, n). That seeds the slice
// with n nil entries; growing it with append after is almost always a slip for
// make([]node.Node, 0, n). make([]node.Node, 0) is the empty form and is fine.
func makeWithLength(stmt ast.Stmt) bool {
	as, ok := stmt.(*ast.AssignStmt)
	if !ok || len(as.Rhs) != 1 {
		return false
	}
	call, ok := as.Rhs[0].(*ast.CallExpr)
	if !ok {
		return false
	}
	id, ok := call.Fun.(*ast.Ident)
	if !ok || id.Name != "make" || len(call.Args) != 2 {
		return false
	}
	lit, ok := call.Args[1].(*ast.BasicLit)
	return !(ok && lit.Kind == token.INT && lit.Value == "0")
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
