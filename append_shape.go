package flint

import (
	"go/ast"
	"go/token"
	"strings"
)

// isAppendTo reports whether s is `name = append(name, ...)`.
func isAppendTo(s *ast.AssignStmt, name string) bool {
	if s.Tok != token.ASSIGN || len(s.Lhs) != 1 || len(s.Rhs) != 1 {
		return false
	}
	lhs, ok := s.Lhs[0].(*ast.Ident)
	if !ok || lhs.Name != name {
		return false
	}
	call, ok := s.Rhs[0].(*ast.CallExpr)
	if !ok {
		return false
	}
	fn, ok := call.Fun.(*ast.Ident)
	if !ok || fn.Name != "append" || len(call.Args) < 2 {
		return false
	}
	arg0, ok := call.Args[0].(*ast.Ident)
	return ok && arg0.Name == name
}

// isSplat reports whether call spreads the slice named name as its final
// argument (f(..., name...)). append and copy are excluded: feeding the slice
// into another append is a merge, not a sink the rewrite can target.
func isSplat(call *ast.CallExpr, name string) bool {
	if !call.Ellipsis.IsValid() || len(call.Args) == 0 {
		return false
	}
	id, ok := call.Args[len(call.Args)-1].(*ast.Ident)
	if !ok || id.Name != name {
		return false
	}
	c := calleeName(call)
	return c != "append" && c != "copy"
}

// stmtHasAppendTo reports whether stmt contains an append to name anywhere within.
func stmtHasAppendTo(stmt ast.Stmt, name string) bool {
	found := false
	ast.Inspect(stmt, func(n ast.Node) bool {
		if as, ok := n.(*ast.AssignStmt); ok && isAppendTo(as, name) {
			found = true
			return false
		}
		return true
	})
	return found
}

// conditionalShape describes how an appending if-statement maps onto Fluent's
// conditional helpers.
type conditionalShape int

const (
	condWhen   conditionalShape = iota // node.When(cond, child)
	condUnless                         // node.Unless(cond, child)
	condBoth                           // node.Condition(cond).True(child).False(child)
)

// appendClass records the control-flow shapes that wrap the appends to an
// accumulator, so the fix can name the Fluent idiom matching each one.
type appendClass struct {
	when      bool // a plain conditional child: if cond { append }
	unless    bool // a negated or else-only conditional child
	both      bool // an if/else that appends in both branches
	loop      bool // a for or range loop
	rangeLoop bool // the loop ranges a value, so a slice may exist for node.Map
	branch    bool // a switch/select that builds children by branching
}

// classify records the idiom implied by a single appending statement, unwrapping
// a labelled statement to the loop or switch it labels.
func (c *appendClass) classify(stmt ast.Stmt, name string) {
	switch s := stmt.(type) {
	case *ast.ForStmt:
		// A bare counting loop has no slice, so node.Map cannot apply.
		c.loop = true
	case *ast.RangeStmt:
		c.loop = true
		c.rangeLoop = true
	case *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
		c.branch = true
	case *ast.LabeledStmt:
		c.classify(s.Stmt, name)
	case *ast.IfStmt:
		// An else-if branch that appends is guarded by its own condition, which
		// When/Unless/Condition cannot express; suggest the general node.Funcs.
		if elseIf, ok := s.Else.(*ast.IfStmt); ok && stmtHasAppendTo(elseIf, name) {
			c.branch = true
			return
		}
		switch ifShape(s, name) {
		case condUnless:
			c.unless = true
		case condBoth:
			c.both = true
		default:
			c.when = true
		}
	}
}

// ifShape decides which conditional idiom an appending if maps to: Condition when
// both branches append, Unless when only the else branch appends or the condition
// is negated, and When otherwise.
func ifShape(s *ast.IfStmt, name string) conditionalShape {
	thenAppends := stmtHasAppendTo(s.Body, name)
	elseAppends := s.Else != nil && stmtHasAppendTo(s.Else, name)
	switch {
	case thenAppends && elseAppends:
		return condBoth
	case elseAppends:
		return condUnless
	case isNegated(s.Cond):
		return condUnless
	default:
		return condWhen
	}
}

// isNegated reports whether cond is a logical negation (!x), ignoring one layer
// of parentheses.
func isNegated(cond ast.Expr) bool {
	if p, ok := cond.(*ast.ParenExpr); ok {
		cond = p.X
	}
	u, ok := cond.(*ast.UnaryExpr)
	return ok && u.Op == token.NOT
}

// nodeAppendFix lists the Fluent idioms the code's shape actually calls for.
// The caller supplies the framing, because the two diagnostics mean different
// things: the nil-seeding case is a real slip to correct, while the ordinary
// case is already the cheapest form and the idioms are only alternatives.
func nodeAppendFix(c appendClass, intoElement bool) string {
	var options []string
	if c.when {
		options = append(options, "node.When(cond, child) for a conditional child")
	}
	if c.unless {
		options = append(options, "node.Unless(cond, child) for a negated conditional child")
	}
	if c.both {
		options = append(options, "node.Condition(cond).True(child).False(child) for an if/else")
	}
	// The deferred forms carry a cost the others do not, so the note goes on
	// them rather than on the advice as a whole: node.Map wraps node.Funcs, and
	// both park the loop in a component and closure that render later. The
	// conditional idioms and passing children straight to the constructor add
	// nothing.
	const deferred = " (defers the loop into a component and closure, so it allocates more than the slice you already have)"
	if c.loop {
		if c.rangeLoop {
			// node.Funcs first and node.Map second: Map wraps Funcs and costs the
			// same, so it is the generic convenience rather than the faster path.
			options = append(options, "node.Funcs(func() []node.Node { ... }) to build the children yourself, or node.Map(slice, fn) for a generic per-element mapping"+deferred)
		} else {
			// No range means no slice to map over, so node.Map cannot express it.
			options = append(options, "node.Funcs(func() []node.Node { ... }) to build the children yourself"+deferred)
		}
	}
	if c.branch {
		options = append(options, "node.Funcs(func() []node.Node { ... }) for branching that builds a slice"+deferred)
	}
	if intoElement {
		options = append(options, "passing children directly to the constructor or via .Add(...)")
	} else {
		options = append(options, "composing the children with Fluent rather than assembling a []node.Node by hand")
	}
	return strings.Join(options, "; ")
}
