package flint

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
)

// checkImports reports imports that use Go reserved keywords as
// package names when fluent provides an alternative. For example,
// html5/select should be html5/dropdown, html5/main should be
// html5/primary, and html5/var should be html5/variable.
func (l *Linter) checkImports(fset *token.FileSet, file *ast.File) []Diagnostic {
	var diags []Diagnostic

	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)

		// Only check fluent HTML element imports.
		if !strings.Contains(path, "fluent/html5/") {
			continue
		}

		// Extract the last segment of the import path.
		seg := lastSegment(path)

		alt, reserved := ReservedAliases[seg]
		if !reserved {
			continue
		}

		correctedPath := strings.TrimSuffix(path, seg) + alt

		diags = append(diags, Diagnostic{
			Pos:     fset.Position(imp.Path.Pos()),
			End:     fset.Position(imp.Path.End()),
			Message: fmt.Sprintf("%q is a Go reserved keyword; use %q instead", seg, alt),
			Fix:     fmt.Sprintf("Import %q which renders the <%s> element", correctedPath, seg),
		})
	}

	return diags
}
