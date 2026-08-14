package flint

import (
	"fmt"
	"go/ast"
	"go/token"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// checkConstructors reports a pkg.New() chain that a package-level constructor
// would build in a single call:
//
//	div.New().Text("hello")                 // div.Text("hello")
//	link.New().Rel(rel.Stylesheet).Href(u)  // link.Stylesheet(u)
//	a.New().Href(u).Text("Click here")      // a.Link(u, "Click here")
//
// The constructor does not have to share a name with anything in the chain. The
// registry records what each one sets, so the check looks for the constructor
// that replaces the most of the chain: every method it sets must appear there,
// and wherever it pins a value the chain must set that same value - which is
// what separates link.Stylesheet from link.Icon, both of which set rel and href.
// Where two constructors fit equally well the chain is left alone rather than
// guessed at.
func (l *Linter) checkConstructors(fset *token.FileSet, file *ast.File) []Diagnostic {
	if l.registry == nil {
		return nil
	}

	imports := resolveImports(file)
	var diags []Diagnostic
	seen := map[token.Pos]bool{}

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		ch := chainOf(call, imports)
		if ch == nil {
			return true
		}
		// Inspect walks outermost call first, so the first visit to a New()
		// sees the whole chain; the calls nested inside it add nothing.
		if seen[ch.root.Pos()] {
			return true
		}
		seen[ch.root.Pos()] = true

		pkg, registered := l.registry.Packages[ch.path]
		if !registered {
			return true
		}
		name, absorbed := ch.best(pkg, imports)
		if name == "" {
			return true
		}

		newCall := ch.pkg + ".New()"
		if len(ch.root.Args) > 0 {
			newCall = ch.pkg + ".New(...)"
		}
		fix := fmt.Sprintf("%s.%s(...) sets %s", ch.pkg, name, joinWords(absorbed))
		if len(ch.root.Args) > 0 {
			fix += " and takes the children as its trailing arguments"
		}

		diags = append(diags, Diagnostic{
			Check:    "constructors",
			Pos:      fset.Position(ch.root.Fun.(*ast.SelectorExpr).Sel.Pos()),
			End:      fset.Position(call.End()),
			Severity: Info,
			Message:  fmt.Sprintf("use %s.%s(...) directly instead of %s%s", ch.pkg, name, newCall, ch.replaced(absorbed)),
			Fix:      fix + "; chain any remaining methods on the result",
		})
		return true
	})

	return diags
}

// chain is a pkg.New(...) call and the methods chained onto it.
type chain struct {
	pkg   string        // the package identifier as written at the call site
	path  string        // its import path
	root  *ast.CallExpr // the New() call
	calls []chainCall   // the methods chained onto it, in source order
}

type chainCall struct {
	method string
	args   []ast.Expr
}

// chainOf walks a method call down its receivers to the pkg.New(...) at the
// root, or returns nil when the expression is not such a chain.
func chainOf(call *ast.CallExpr, imports map[string]string) *chain {
	var calls []chainCall
	for {
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return nil
		}
		if ident, ok := sel.X.(*ast.Ident); ok {
			path, imported := imports[ident.Name]
			if !imported || sel.Sel.Name != "New" {
				return nil
			}
			slices.Reverse(calls)
			return &chain{pkg: ident.Name, path: path, root: call, calls: calls}
		}
		inner, ok := sel.X.(*ast.CallExpr)
		if !ok {
			return nil
		}
		calls = append(calls, chainCall{method: sel.Sel.Name, args: call.Args})
		call = inner
	}
}

// replaced renders the part of the chain the constructor takes over, with an
// ellipsis standing in for calls it leaves behind: link.New().Rel(...).Href(...)
// for a chain of exactly those two, div.New()...Text(...) where a .Class() call
// sits in between.
func (c *chain) replaced(absorbed []string) string {
	var b strings.Builder
	gap := false
	for _, call := range c.calls {
		if !slices.Contains(absorbed, call.method) {
			gap = true
			continue
		}
		if gap {
			b.WriteString("...")
			gap = false
		} else {
			b.WriteString(".")
		}
		b.WriteString(call.method + "(...)")
	}
	return b.String()
}

// best returns the constructor that replaces the most of the chain, along with
// the methods it makes unnecessary in source order. It returns an empty name
// when nothing fits, or when two constructors fit equally well: naming one of
// them would be a coin toss, and a wrong constructor is worse than none.
func (c *chain) best(pkg Package, imports map[string]string) (string, []string) {
	calls := map[string]chainCall{}
	repeated := map[string]bool{}
	for _, call := range c.calls {
		if _, ok := calls[call.method]; ok {
			repeated[call.method] = true
		}
		calls[call.method] = call
	}

	content := 0
	for method := range calls {
		if isContent(pkg, method) {
			content++
		}
	}

	type candidate struct {
		name   string
		sets   int // methods it replaces
		pinned int // values it had to match to fit
	}
	var fits []candidate
	for name, ctor := range pkg.Constructors {
		if _, deprecated := pkg.DeprecatedFunctions[name]; deprecated {
			continue
		}
		// Children passed to New() need a constructor that still accepts them.
		if len(c.root.Args) > 0 && !ctor.Nodes {
			continue
		}
		// Taking some of the text content but not all of it would reorder the
		// children, since what the constructor takes always comes first.
		if len(ctor.Content) > 0 && len(ctor.Content) != content {
			continue
		}
		if !ctorFits(ctor, calls, repeated, imports) {
			continue
		}
		fits = append(fits, candidate{name, len(ctor.Sets), len(ctor.Pins) + len(ctor.Prefixes)})
	}
	if len(fits) == 0 {
		return "", nil
	}

	// The most of the chain replaced wins; between constructors that replace
	// the same methods, the one that matched the more specific values does -
	// a.MailTo over a.Link for a mailto: href, meta.UTF8 over meta.Charset.
	sort.Slice(fits, func(i, j int) bool {
		if fits[i].sets != fits[j].sets {
			return fits[i].sets > fits[j].sets
		}
		if fits[i].pinned != fits[j].pinned {
			return fits[i].pinned > fits[j].pinned
		}
		return fits[i].name < fits[j].name
	})
	if len(fits) > 1 && fits[0].sets == fits[1].sets && fits[0].pinned == fits[1].pinned {
		return "", nil
	}

	winner := fits[0].name
	var absorbed []string
	for _, call := range c.calls {
		if slices.Contains(pkg.Constructors[winner].Sets, call.method) {
			absorbed = append(absorbed, call.method)
		}
	}
	return winner, absorbed
}

// ctorFits reports whether the chain does by hand everything the constructor
// would do for it: every method set, once each, with any pinned value matched.
func ctorFits(ctor Constructor, calls map[string]chainCall, repeated map[string]bool, imports map[string]string) bool {
	for _, method := range ctor.Sets {
		call, called := calls[method]
		if !called || repeated[method] {
			return false
		}
		if pin, ok := ctor.Pins[method]; ok && !pinMatches(pin, call.args, imports) {
			return false
		}
		if prefix, ok := ctor.Prefixes[method]; ok && !prefixMatches(prefix, call.args) {
			return false
		}
	}
	return true
}

// pinMatches reports whether a call sets the value a constructor fixes. A
// pinned true also matches the bare form of a boolean setter, since
// details.New().Open() and details.New().Open(true) mean the same thing.
func pinMatches(pin string, args []ast.Expr, imports map[string]string) bool {
	if len(args) == 0 {
		return pin == "true"
	}
	if len(args) != 1 {
		return false
	}
	switch arg := args[0].(type) {
	case *ast.BasicLit:
		return arg.Value == pin
	case *ast.Ident:
		return arg.Name == pin
	case *ast.SelectorExpr:
		ident, ok := arg.X.(*ast.Ident)
		if !ok {
			return false
		}
		pkg, name, qualified := strings.Cut(pin, ".")
		if !qualified || arg.Sel.Name != name {
			return false
		}
		// Compare through the import, so an aliased enum package still matches.
		if path, imported := imports[ident.Name]; imported {
			return lastSegment(path) == pkg
		}
		return ident.Name == pkg
	}
	return false
}

// prefixMatches reports whether a call's argument is a literal starting with
// the prefix a constructor adds itself, so that a mailto: href resolves to
// a.MailTo rather than to the plain a.Link.
func prefixMatches(prefix string, args []ast.Expr) bool {
	if len(args) != 1 {
		return false
	}
	lit, ok := args[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return false
	}
	value, err := strconv.Unquote(lit.Value)
	return err == nil && strings.HasPrefix(value, prefix)
}

// isContent reports whether a method adds child nodes rather than setting an
// attribute, which any constructor taking text content declares.
func isContent(pkg Package, method string) bool {
	for _, ctor := range pkg.Constructors {
		if slices.Contains(ctor.Content, method) {
			return true
		}
	}
	return false
}

// joinWords lists names as prose: "Rel and Href", "Href, Download and Text".
func joinWords(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	default:
		return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
	}
}
