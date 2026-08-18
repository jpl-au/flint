package flint

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
)

// literalArgCheck describes a check that flags calls to named functions
// where the first argument is not a string literal.
type literalArgCheck struct {
	name     string   // check name recorded on the diagnostic
	names    []string // function/method names to match
	nargs    int      // exact arg count to match, or -1 for any
	severity Severity
	message  string // fmt pattern for the diagnostic
	fix      string

	// passThrough exempts the paired-constructor idiom: a function whose name
	// ends in Static, forwarding its own parameter to the flagged call. The
	// forwarding is the contract, the same one fluent's own paired Static and
	// Text constructors carry, so the wrapper's name passes the literal-only
	// obligation to its callers as div.Static does.
	//
	// The exemption does not extend to RawText. RawText skips escaping, so
	// waiving the check on a naming convention alone would hide an XSS hole. A
	// trusted RawText wrapper needs a //flint:allow directive.
	passThrough bool
}

// checkStatic reports calls to Static() where the argument is not a
// string literal. Static content is marked for JIT pre-rendering and
// must not contain dynamic values.
func (l *Linter) checkStatic(fset *token.FileSet, file *ast.File) []Diagnostic {
	return l.checkLiteralArgs(fset, file, literalArgCheck{
		name:        "static",
		names:       []string{"Static"},
		nargs:       1,
		severity:    Warning,
		message:     "Static() argument must be a string literal; got %s",
		fix:         "Static() is for string literals only (JIT pre-rendering); replace Static with Text or Textf for dynamic content",
		passThrough: true,
	})
}

// checkLiteralArgs walks the AST and reports calls matching check where
// the first argument is not a string literal. Only flags calls on
// fluent elements (scoped via the registry).
func (l *Linter) checkLiteralArgs(fset *token.FileSet, file *ast.File, check literalArgCheck) []Diagnostic {
	var diags []Diagnostic

	names := make(map[string]bool, len(check.names))
	for _, n := range check.names {
		names[n] = true
	}

	// Scope to fluent packages when a registry is available.
	var imports map[string]string
	if l.registry != nil {
		imports = resolveImports(file)
	}

	var wrappers []staticWrapper
	if check.passThrough {
		wrappers = staticWrappers(file)
	}

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		name := calleeName(call)
		if !names[name] {
			return true
		}

		// Scope check: verify the receiver traces back to a fluent package.
		if imports != nil && l.registry != nil {
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if _, found := chainPackage(sel.X, imports, l.registry); !found {
				return true
			}
		}

		if len(call.Args) == 0 {
			return true
		}
		if check.nargs > 0 && len(call.Args) != check.nargs {
			return true
		}

		arg := call.Args[0]
		if isStringLiteral(arg) {
			return true
		}
		if check.passThrough && forwardsWrapperParam(wrappers, call.Pos(), arg) {
			return true
		}

		var msg string
		if len(check.names) == 1 {
			msg = fmt.Sprintf(check.message, describeExpr(arg))
		} else {
			msg = fmt.Sprintf(check.message, name, describeExpr(arg))
		}

		diags = append(diags, Diagnostic{
			Check:    check.name,
			Pos:      fset.Position(arg.Pos()),
			End:      fset.Position(arg.End()),
			Severity: check.severity,
			Message:  msg,
			Fix:      check.fix,
		})

		return true
	})

	return diags
}

// staticWrapper is one function whose name ends in Static: its source extent
// and parameter names, for recognising the pass-through idiom.
type staticWrapper struct {
	pos, end token.Pos
	params   map[string]bool
}

// staticWrappers collects every function declaration in the file whose name
// ends in Static, with its parameter names. Method receivers qualify the same
// way as plain functions: form.LabelStatic is the idiom as much as Static.
func staticWrappers(file *ast.File) []staticWrapper {
	var wrappers []staticWrapper
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || !strings.HasSuffix(fd.Name.Name, "Static") {
			continue
		}
		params := map[string]bool{}
		if fd.Type.Params != nil {
			for _, field := range fd.Type.Params.List {
				for _, id := range field.Names {
					if id.Name != "_" {
						params[id.Name] = true
					}
				}
			}
		}
		wrappers = append(wrappers, staticWrapper{pos: fd.Pos(), end: fd.End(), params: params})
	}
	return wrappers
}

// forwardsWrapperParam reports whether a call at pos passes a parameter of an
// enclosing Static-named function as arg - the pass-through idiom. A local
// that shadows the parameter would be blessed too; the approximation errs
// only inside functions that already declare the literal-only contract.
func forwardsWrapperParam(wrappers []staticWrapper, pos token.Pos, arg ast.Expr) bool {
	id, ok := unparen(arg).(*ast.Ident)
	if !ok {
		return false
	}
	for _, w := range wrappers {
		if pos >= w.pos && pos < w.end && w.params[id.Name] {
			return true
		}
	}
	return false
}
