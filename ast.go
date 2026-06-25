package flint

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
)

// resolveImports builds a map from local package names to import paths
// for all imports in the file.
func resolveImports(file *ast.File) map[string]string {
	imports := make(map[string]string)
	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)

		var localName string
		if imp.Name != nil {
			localName = imp.Name.Name
		} else {
			localName = pkgName(path)
		}

		imports[localName] = path
	}
	return imports
}

// chainPackage walks leftward through a selector/call chain to find
// the originating package. For example, in div.New().Class("x"),
// starting from the .Class selector's X (which is div.New()), this
// resolves back to the div package.
func chainPackage(expr ast.Expr, imports map[string]string, reg *Registry) (Package, bool) {
	switch e := expr.(type) {
	case *ast.CallExpr:
		return chainPackage(e.Fun, imports, reg)

	case *ast.ParenExpr:
		return chainPackage(e.X, imports, reg)

	case *ast.SelectorExpr:
		if ident, ok := e.X.(*ast.Ident); ok {
			if importPath, ok := imports[ident.Name]; ok {
				if pkg, ok := reg.Packages[importPath]; ok {
					return pkg, true
				}
			}
		}
		return chainPackage(e.X, imports, reg)

	case *ast.Ident:
		if importPath, ok := imports[e.Name]; ok {
			if pkg, ok := reg.Packages[importPath]; ok {
				return pkg, true
			}
		}
	}

	return Package{}, false
}

// unparen strips any enclosing parentheses from expr.
func unparen(expr ast.Expr) ast.Expr {
	for {
		p, ok := expr.(*ast.ParenExpr)
		if !ok {
			return expr
		}
		expr = p.X
	}
}

// lastSegment returns the last path segment of an import path.
func lastSegment(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}

// pkgName returns the local name a Go file would use for an
// unaliased import of path. Most paths have a last segment that is also
// the package's declared name, so [lastSegment] is the answer. When the
// last segment contains hyphens (e.g. "fluent-security") it is not a
// valid Go identifier and the package's declared name is conventionally
// the substring after the final hyphen ("security"). This matches how
// the real fluent ecosystem packages are imported (fluent-security as
// "security", fluent-htmx as "htmx", fluent-jit as "jit").
//
// Used only when the import has no explicit alias - explicit aliases
// always win.
func pkgName(path string) string {
	last := lastSegment(path)
	if i := strings.LastIndex(last, "-"); i >= 0 {
		return last[i+1:]
	}
	return last
}

// calleeName returns the simple name of the called function or method.
// For selector expressions (pkg.Func or recv.Method) it returns the
// selected name. For plain identifiers it returns the identifier name.
func calleeName(call *ast.CallExpr) string {
	switch fn := call.Fun.(type) {
	case *ast.SelectorExpr:
		return fn.Sel.Name
	case *ast.Ident:
		return fn.Name
	}
	return ""
}

// isStringLiteral reports whether expr is a basic string literal.
func isStringLiteral(expr ast.Expr) bool {
	lit, ok := expr.(*ast.BasicLit)
	return ok && lit.Kind == token.STRING
}

// describeExpr returns a human-readable description of an expression
// for use in diagnostic messages.
func describeExpr(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return fmt.Sprintf("variable %q", e.Name)
	case *ast.BinaryExpr:
		return "binary expression"
	case *ast.CallExpr:
		return "function call"
	case *ast.IndexExpr:
		return "index expression"
	case *ast.SliceExpr:
		return "slice expression"
	case *ast.UnaryExpr:
		return "unary expression"
	case *ast.ParenExpr:
		return describeExpr(e.X)
	default:
		return "non-literal expression"
	}
}
