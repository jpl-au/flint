package flint

import (
	"fmt"
	"go/ast"
	"go/token"
)

// checkShadows reports local declarations that shadow the import alias of a
// fluent package: a variable, parameter, or range binding named after the
// package. Shadowing a fluent alias is a hazard in its own right - a name like
// input or form reads as the package but is not - and it also undermines
// flint's other checks, which resolve identifiers through the file's imports
// and cannot see the shadowing. Rather than guessing at intent, this check
// names the collision directly so the developer can rename one side.
func (l *Linter) checkShadows(fset *token.FileSet, file *ast.File) []Diagnostic {
	if l.registry == nil {
		return nil
	}

	// Only aliases that resolve to a registered fluent package matter;
	// shadowing an unrelated import is outside flint's remit. A blank or dot
	// import has no referenceable alias, so nothing can shadow it.
	fluent := make(map[string]bool)
	for alias, path := range resolveImports(file) {
		if alias == "_" || alias == "." {
			continue
		}
		if _, ok := l.registry.Packages[path]; ok {
			fluent[alias] = true
		}
	}
	if len(fluent) == 0 {
		return nil
	}

	var diags []Diagnostic

	// One report per name per function keeps a repeatedly bound name (a := in
	// several branches, say) from drowning the message in duplicates.
	report := func(seen map[string]bool, id *ast.Ident, decl string) {
		if id == nil || !fluent[id.Name] || seen[id.Name] {
			return
		}
		seen[id.Name] = true
		diags = append(diags, Diagnostic{
			Check:    "shadows",
			Pos:      fset.Position(id.Pos()),
			End:      fset.Position(id.End()),
			Severity: Warning,
			Message:  fmt.Sprintf("%s %q shadows the fluent package imported as %q", decl, id.Name, id.Name),
			Fix:      fmt.Sprintf("Rename the %s, or give the import an alias. While the name is shadowed, %s.X looks like the package but is not, and other diagnostics for %q may be wrong.", decl, id.Name, id.Name),
		})
	}

	ast.Inspect(file, func(n ast.Node) bool {
		var ft *ast.FuncType
		var recv *ast.FieldList
		var body *ast.BlockStmt

		switch x := n.(type) {
		case *ast.FuncDecl:
			ft, recv, body = x.Type, x.Recv, x.Body
		case *ast.FuncLit:
			ft, body = x.Type, x.Body
		default:
			return true
		}

		seen := make(map[string]bool)
		for _, fl := range []*ast.FieldList{recv, ft.Params, ft.Results} {
			if fl == nil {
				continue
			}
			for _, f := range fl.List {
				for _, name := range f.Names {
					report(seen, name, "parameter")
				}
			}
		}
		if body == nil {
			return true
		}

		// Bindings inside the body: short declarations (including if/for/switch
		// inits, type switches, and select comm clauses, which all parse as
		// AssignStmt), var declarations, and range variables. A nested function
		// literal is walked by its own FuncDecl/FuncLit visit above, so stop at
		// the boundary to keep the per-function dedup honest.
		ast.Inspect(body, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.FuncLit:
				return false
			case *ast.AssignStmt:
				if x.Tok != token.DEFINE {
					return true
				}
				for _, lhs := range x.Lhs {
					if id, ok := lhs.(*ast.Ident); ok {
						report(seen, id, "local variable")
					}
				}
			case *ast.ValueSpec:
				for _, name := range x.Names {
					report(seen, name, "local variable")
				}
			case *ast.RangeStmt:
				if x.Tok != token.DEFINE {
					return true
				}
				if id, ok := x.Key.(*ast.Ident); ok {
					report(seen, id, "range variable")
				}
				if id, ok := x.Value.(*ast.Ident); ok {
					report(seen, id, "range variable")
				}
			}
			return true
		})

		return true
	})

	return diags
}
