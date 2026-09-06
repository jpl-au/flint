package flint

import (
	"fmt"
	"go/ast"
	"go/token"
	"slices"
	"strconv"
	"strings"
)

// checkDuplicateAttrs reports an attribute set twice within a single chain.
// Most attribute methods store one value, so the later call silently
// overwrites the earlier one: div.New().ID("a").ID("b") renders id="b" and the
// first call is dead. A SetAttribute whose key a dedicated method already set
// is worse: the field and the raw attribute both render, producing a duplicate
// attribute in the output (browsers keep the first and drop the rest).
// Accumulating methods (Class, Style) concatenate across calls and are exempt
// from the repeated-call rule.
//
// The registry's Constructors data extends the same rule to what a chain's
// root constructor sets for you: input.Text pins type="text", so a
// SetAttribute("type", ...) anywhere on that element duplicates it just as a
// chained .Type() call would. Both the single-chain form and the
// split-statement form (constructor assigned to a local, SetAttribute on a
// later line) are covered. A duplicated type attribute renders twice, and the
// browser uses the first, so the second value has no effect.
func (l *Linter) checkDuplicateAttrs(fset *token.FileSet, file *ast.File) []Diagnostic {
	if l.registry == nil {
		return nil
	}

	imports := resolveImports(file)
	locals := fluentLocalPackages(file, imports, l.registry)

	diags := l.checkLocalDuplicateAttrs(fset, file, imports)
	seen := map[*ast.CallExpr]bool{}

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || seen[call] {
			return true
		}

		// Walk the chain spine leftward from its outermost call. Inspect
		// visits parents first, so an unseen call is a chain head; marking
		// the spine ensures each chain is processed exactly once. Arguments
		// are not part of the spine and form their own chains.
		var spine []*ast.CallExpr
		for cur := call; ; {
			seen[cur] = true
			sel, ok := cur.Fun.(*ast.SelectorExpr)
			if !ok {
				break
			}
			spine = append(spine, cur)
			inner, ok := sel.X.(*ast.CallExpr)
			if !ok {
				break
			}
			cur = inner
		}
		if len(spine) < 2 {
			return true
		}

		// Resolve the chain to a fluent package: rooted at an import, or at
		// a local assigned from one. Duplication is judged per package, so
		// an unresolvable receiver stays quiet rather than guessing.
		pkg, found := chainPackage(call, imports, l.registry)
		if !found {
			root := spine[len(spine)-1]
			if sel, ok := root.Fun.(*ast.SelectorExpr); ok {
				if id, ok := unparen(sel.X).(*ast.Ident); ok {
					pkg, found = locals[id.Name]
				}
			}
			if !found {
				return true
			}
		}

		attrMethodSet := make(map[string]bool, len(pkg.AttrMethods))
		for _, m := range pkg.AttrMethods {
			attrMethodSet[m] = true
		}

		// What the chain's root constructor sets for you, so a SetAttribute
		// on one of those attributes is caught even though no chained call
		// names it: input.Text("e", "").SetAttribute("type", ...) renders two
		// type attributes exactly as .Type().SetAttribute("type", ...) would.
		ctorLabel, ctorSet := constructorSets(spine[len(spine)-1], pkg, imports)

		// Replay the chain in source order, remembering the first call of
		// each overwriting attribute method.
		first := map[string]setterCall{}
		for _, c := range slices.Backward(spine) {

			sel := c.Fun.(*ast.SelectorExpr)
			m := sel.Sel.Name
			if id, ok := unparen(sel.X).(*ast.Ident); ok {
				if _, imported := imports[id.Name]; imported {
					continue // package constructors are accounted for by ctorSet
				}
			}

			if m == "SetAttribute" || m == "SetAttributeRaw" {
				if len(c.Args) < 1 || !isStringLiteral(c.Args[0]) {
					continue
				}
				lit := c.Args[0].(*ast.BasicLit)
				key, err := strconv.Unquote(lit.Value)
				if err != nil {
					continue
				}
				dedicated := pkg.AttrMethods[strings.ToLower(key)]
				if dedicated == "" {
					continue
				}
				if previous, set := first[dedicated]; set {
					diags = append(diags, Diagnostic{
						Check:    "duplicate-attr",
						Pos:      fset.Position(sel.Sel.Pos()),
						End:      fset.Position(c.End()),
						Severity: Warning,
						Message:  fmt.Sprintf("%s(%q, ...) repeats the .%s() call earlier in this chain. Both render, so the %s attribute appears twice.", m, key, previous.name, key),
						Fix:      fmt.Sprintf("Move the value into the .%s() call. A browser keeps the first copy of a duplicated attribute.", dedicated),
					})
					continue
				}
				if ctorSet[dedicated] {
					diags = append(diags, constructorDuplicate(fset, sel, c, m, key, dedicated, ctorLabel))
				}
				continue
			}

			canonical := canonicalSetter(pkg, m)
			if !attrMethodSet[canonical] {
				continue
			}
			// Accumulating methods are recorded (a later SetAttribute on the
			// same key still duplicates them) but repeated calls are fine.
			prev, set := first[canonical]
			if set && !pkg.AccumulatingMethods[canonical] {
				diags = append(diags, Diagnostic{
					Check:    "duplicate-attr",
					Pos:      fset.Position(sel.Sel.Pos()),
					End:      fset.Position(c.End()),
					Severity: Warning,
					Message:  fmt.Sprintf(".%s() overwrites the value that .%s() set on line %d. Only the last value renders.", m, prev.name, fset.Position(prev.pos).Line),
					Fix:      fmt.Sprintf("Keep a single .%s() call with the value you want rendered", canonical),
				})
				continue
			}
			if !set {
				first[canonical] = setterCall{name: m, pos: sel.Sel.Pos()}
			}
		}

		return true
	})

	return diags
}

// constructorSets resolves a chain's root call against the registry's
// Constructors data: the label as written at the call site (e.g. "input.Text")
// and the attribute methods the constructor applies for you. Content methods
// (Text, Static, ...) add children rather than set attributes, so they are
// excluded. ok is effectively "the root is a registered constructor"; a root
// at a local variable or an unrecorded function yields nil.
func constructorSets(root *ast.CallExpr, pkg Package, imports map[string]string) (string, map[string]bool) {
	sel, ok := root.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", nil
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok {
		return "", nil
	}
	if _, imported := imports[id.Name]; !imported {
		return "", nil
	}
	ctor, ok := pkg.Constructors[sel.Sel.Name]
	if !ok {
		return "", nil
	}
	set := map[string]bool{}
	for _, m := range ctor.Sets {
		if slices.Contains(ctor.Content, m) {
			continue
		}
		set[m] = true
	}
	return id.Name + "." + sel.Sel.Name, set
}

// constructorDuplicate is the diagnostic for a SetAttribute call whose key a
// constructor has already set: both the field and the raw attribute render,
// and browsers keep the first, so the SetAttribute value is silently dead.
func constructorDuplicate(fset *token.FileSet, sel *ast.SelectorExpr, c *ast.CallExpr, call, key, dedicated, ctorLabel string) Diagnostic {
	return Diagnostic{
		Check:    "duplicate-attr",
		Pos:      fset.Position(sel.Sel.Pos()),
		End:      fset.Position(c.End()),
		Severity: Warning,
		Message:  fmt.Sprintf("%s(%q, ...) sets %s again after %s. Both render, and the browser uses the first, so this value has no effect.", call, key, key, ctorLabel),
		Fix:      fmt.Sprintf("A browser keeps the first copy of a duplicated attribute. Use a constructor that sets the %s you want, or set it once with .%s().", key, dedicated),
	}
}

type setterCall struct {
	name string
	pos  token.Pos
}

func canonicalSetter(pkg Package, method string) string {
	if canonical := pkg.SetterAliases[method]; canonical != "" {
		return canonical
	}
	return method
}

// localChain records what the fluent chain that initialised a local set: the
// registry package, the root constructor's non-content Sets, and the attribute
// methods chained onto it. Everything recorded was set unconditionally at the
// point of assignment, so a later SetAttribute on the same attribute is a
// duplicate regardless of the control flow in between.
type localChain struct {
	pkg       Package
	ctorLabel string
	ctorSet   map[string]bool
	chained   map[string]setterCall
	assigned  token.Pos
}

// checkLocalDuplicateAttrs covers the split-statement form of the duplicate
// rule: a local assigned from a fluent constructor chain, with SetAttribute
// called on it in a later statement. The chain replay above cannot see it - a
// bare local.SetAttribute(...) is a spine of one - yet it renders the same
// duplicate attribute.
// Locals are collected file-wide with last-assignment-wins, the same
// approximation as fluentLocals, and only calls after the assignment count.
func (l *Linter) checkLocalDuplicateAttrs(fset *token.FileSet, file *ast.File, imports map[string]string) []Diagnostic {
	locals := map[string]localChain{}
	record := func(lhs, rhs ast.Expr) {
		id, ok := lhs.(*ast.Ident)
		if !ok || id.Name == "_" {
			return
		}
		call, ok := rhs.(*ast.CallExpr)
		if !ok {
			return
		}
		pkg, ok := chainPackage(call, imports, l.registry)
		if !ok {
			return
		}
		attrMethodSet := make(map[string]bool, len(pkg.AttrMethods))
		for _, m := range pkg.AttrMethods {
			attrMethodSet[m] = true
		}
		state := localChain{pkg: pkg, chained: map[string]setterCall{}, assigned: rhs.Pos()}
		for cur := call; ; {
			sel, ok := cur.Fun.(*ast.SelectorExpr)
			if !ok {
				break
			}
			if id, ok := unparen(sel.X).(*ast.Ident); ok {
				if _, imported := imports[id.Name]; imported {
					state.ctorLabel, state.ctorSet = constructorSets(cur, pkg, imports)
					break
				}
			}
			// Accumulating methods are recorded too: repeated calls
			// concatenate, but a raw attribute alongside still duplicates.
			if m := canonicalSetter(pkg, sel.Sel.Name); attrMethodSet[m] {
				state.chained[m] = setterCall{name: sel.Sel.Name, pos: sel.Sel.Pos()}
			}
			inner, ok := sel.X.(*ast.CallExpr)
			if !ok {
				state.ctorLabel, state.ctorSet = constructorSets(cur, pkg, imports)
				break
			}
			cur = inner
		}
		locals[id.Name] = state
	}

	ast.Inspect(file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.AssignStmt:
			if len(x.Lhs) == len(x.Rhs) {
				for i := range x.Lhs {
					record(x.Lhs[i], x.Rhs[i])
				}
			}
		case *ast.ValueSpec:
			if len(x.Names) == len(x.Values) {
				for i := range x.Names {
					record(x.Names[i], x.Values[i])
				}
			}
		}
		return true
	})
	if len(locals) == 0 {
		return nil
	}

	var diags []Diagnostic
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		name := sel.Sel.Name
		if name != "SetAttribute" && name != "SetAttributeRaw" {
			return true
		}
		id, ok := unparen(sel.X).(*ast.Ident)
		if !ok {
			return true
		}
		state, tracked := locals[id.Name]
		if !tracked || call.Pos() < state.assigned {
			return true
		}
		if len(call.Args) < 1 || !isStringLiteral(call.Args[0]) {
			return true
		}
		key, err := strconv.Unquote(call.Args[0].(*ast.BasicLit).Value)
		if err != nil {
			return true
		}
		dedicated := state.pkg.AttrMethods[strings.ToLower(key)]
		if dedicated == "" {
			return true
		}
		if previous, chained := state.chained[dedicated]; chained {
			diags = append(diags, Diagnostic{
				Check:    "duplicate-attr",
				Pos:      fset.Position(sel.Sel.Pos()),
				End:      fset.Position(call.End()),
				Severity: Warning,
				Message:  fmt.Sprintf("%s(%q, ...) sets %s again after .%s() on line %d. Both render, and the browser uses the first, so this value has no effect.", name, key, key, previous.name, fset.Position(previous.pos).Line),
				Fix:      fmt.Sprintf("Move the value into the .%s() call. A browser keeps the first copy of a duplicated attribute.", dedicated),
			})
			return true
		}
		if state.ctorSet[dedicated] {
			diags = append(diags, constructorDuplicate(fset, sel, call, name, key, dedicated, state.ctorLabel))
		}
		return true
	})
	return diags
}
