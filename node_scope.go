package flint

import (
	"go/ast"
	"go/token"
)

// accumScan summarises how the accumulator named by a declaration is used across
// a function body. It is what lets the rule tell "the same slice, grown then
// spread" from an unrelated variable that merely shares the name: occurrences
// inside a scope that redeclares the name are skipped, because they are a
// different variable.
type accumScan struct {
	appends int           // appends made directly, outside any closure
	splats  int           // spreads of the slice made directly, outside any closure
	splat   *ast.CallExpr // the sole splat, when splats == 1
	tooLate bool          // an append happens inside a defer/goroutine, after the spread
	other   bool          // any other use: read elsewhere, mutated in a plain closure, indexed, re-sliced
}

// scanAccumulator reads body for the accumulator named name (declared at
// declIdent) and classifies every use of it. A nested scope that redeclares the
// name shadows it, so those occurrences are ignored. Appends and spreads made
// inside a closure are escapes - for a defer/goroutine they run too late, for a
// plain closure the rewrite is not local - and are not counted as the simple
// accumulate-then-spread shape.
func scanAccumulator(body *ast.BlockStmt, name string, declIdent *ast.Ident) accumScan {
	var s accumScan
	blessed := map[*ast.Ident]bool{declIdent: true}
	var uses []*ast.Ident

	var walkStmts func(list []ast.Stmt, shadowed, inClosure, inDeferGo bool)
	var walkStmt func(stmt ast.Stmt, shadowed, inClosure, inDeferGo bool)
	var walkExpr func(expr ast.Expr, shadowed, inClosure, inDeferGo bool)
	var walkCall func(call *ast.CallExpr, shadowed, inClosure, deferGo bool)
	var walkMaybeRedecl func(stmt ast.Stmt, shadowed, inClosure, inDeferGo bool) bool

	// walkExpr scans an expression for spreads and name uses with a fixed
	// shadow/closure context; a nested closure is recursed into separately.
	walkExpr = func(expr ast.Expr, shadowed, inClosure, inDeferGo bool) {
		if expr == nil {
			return
		}
		ast.Inspect(expr, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.FuncLit:
				walkStmts(x.Body.List, shadowed || fieldsDeclareName(x.Type, name), true, false)
				return false
			case *ast.CallExpr:
				if !shadowed && isSplat(x, name) {
					if inClosure {
						s.other = true
					} else {
						s.splats++
						s.splat = x
						blessed[x.Args[len(x.Args)-1].(*ast.Ident)] = true
					}
				}
				return true
			case *ast.Ident:
				if x.Name == name && !shadowed {
					uses = append(uses, x)
				}
				return true
			}
			return true
		})
	}

	// walkCall handles a defer/go call: its arguments evaluate immediately, but a
	// literal function body runs later (so appends to the slice there are too late).
	walkCall = func(call *ast.CallExpr, shadowed, inClosure, deferGo bool) {
		if fl, ok := call.Fun.(*ast.FuncLit); ok {
			walkStmts(fl.Body.List, shadowed || fieldsDeclareName(fl.Type, name), true, deferGo)
		} else {
			walkExpr(call.Fun, shadowed, inClosure, false)
		}
		for _, a := range call.Args {
			walkExpr(a, shadowed, inClosure, false)
		}
	}

	walkStmt = func(stmt ast.Stmt, shadowed, inClosure, inDeferGo bool) {
		switch x := stmt.(type) {
		case *ast.AssignStmt:
			if !shadowed && isAppendTo(x, name) {
				blessed[x.Lhs[0].(*ast.Ident)] = true
				if arg0, ok := x.Rhs[0].(*ast.CallExpr).Args[0].(*ast.Ident); ok {
					blessed[arg0] = true
				}
				switch {
				case inDeferGo:
					s.tooLate = true
				case inClosure:
					s.other = true
				default:
					s.appends++
				}
			}
			for _, e := range x.Rhs {
				walkExpr(e, shadowed, inClosure, inDeferGo)
			}
			for _, e := range x.Lhs {
				walkExpr(e, shadowed, inClosure, inDeferGo)
			}
		case *ast.ExprStmt:
			walkExpr(x.X, shadowed, inClosure, inDeferGo)
		case *ast.ReturnStmt:
			for _, e := range x.Results {
				walkExpr(e, shadowed, inClosure, inDeferGo)
			}
		case *ast.BlockStmt:
			walkStmts(x.List, shadowed, inClosure, inDeferGo)
		case *ast.IfStmt:
			sh := shadowed
			if x.Init != nil {
				sh = walkMaybeRedecl(x.Init, sh, inClosure, inDeferGo)
			}
			walkExpr(x.Cond, sh, inClosure, inDeferGo)
			walkStmts(x.Body.List, sh, inClosure, inDeferGo)
			if x.Else != nil {
				walkStmt(x.Else, sh, inClosure, inDeferGo)
			}
		case *ast.ForStmt:
			sh := shadowed
			if x.Init != nil {
				sh = walkMaybeRedecl(x.Init, sh, inClosure, inDeferGo)
			}
			walkExpr(x.Cond, sh, inClosure, inDeferGo)
			if x.Post != nil {
				walkStmt(x.Post, sh, inClosure, inDeferGo)
			}
			walkStmts(x.Body.List, sh, inClosure, inDeferGo)
		case *ast.RangeStmt:
			walkExpr(x.X, shadowed, inClosure, inDeferGo)
			sh := shadowed
			if x.Tok == token.DEFINE {
				if id, ok := x.Key.(*ast.Ident); ok && id.Name == name {
					blessed[id] = true
					sh = true
				}
				if id, ok := x.Value.(*ast.Ident); ok && id.Name == name {
					blessed[id] = true
					sh = true
				}
			}
			walkStmts(x.Body.List, sh, inClosure, inDeferGo)
		case *ast.SwitchStmt:
			sh := shadowed
			if x.Init != nil {
				sh = walkMaybeRedecl(x.Init, sh, inClosure, inDeferGo)
			}
			walkExpr(x.Tag, sh, inClosure, inDeferGo)
			walkStmts(x.Body.List, sh, inClosure, inDeferGo)
		case *ast.TypeSwitchStmt:
			sh := shadowed
			if x.Init != nil {
				sh = walkMaybeRedecl(x.Init, sh, inClosure, inDeferGo)
			}
			if x.Assign != nil {
				walkStmt(x.Assign, sh, inClosure, inDeferGo)
			}
			walkStmts(x.Body.List, sh, inClosure, inDeferGo)
		case *ast.CaseClause:
			for _, e := range x.List {
				walkExpr(e, shadowed, inClosure, inDeferGo)
			}
			walkStmts(x.Body, shadowed, inClosure, inDeferGo)
		case *ast.SelectStmt:
			walkStmts(x.Body.List, shadowed, inClosure, inDeferGo)
		case *ast.CommClause:
			if x.Comm != nil {
				walkStmt(x.Comm, shadowed, inClosure, inDeferGo)
			}
			walkStmts(x.Body, shadowed, inClosure, inDeferGo)
		case *ast.DeferStmt:
			walkCall(x.Call, shadowed, inClosure, true)
		case *ast.GoStmt:
			walkCall(x.Call, shadowed, inClosure, true)
		case *ast.LabeledStmt:
			walkStmt(x.Stmt, shadowed, inClosure, inDeferGo)
		case *ast.DeclStmt:
			if gd, ok := x.Decl.(*ast.GenDecl); ok {
				for _, spec := range gd.Specs {
					if vs, ok := spec.(*ast.ValueSpec); ok {
						for _, e := range vs.Values {
							walkExpr(e, shadowed, inClosure, inDeferGo)
						}
					}
				}
			}
		case *ast.IncDecStmt:
			walkExpr(x.X, shadowed, inClosure, inDeferGo)
		case *ast.SendStmt:
			walkExpr(x.Chan, shadowed, inClosure, inDeferGo)
			walkExpr(x.Value, shadowed, inClosure, inDeferGo)
		}
	}

	// walkMaybeRedecl walks stmt and reports the shadow state for the statements
	// that follow it: a lone `name := ...` or `var name ...` (other than the
	// accumulator's own declaration) shadows the name from here on.
	walkMaybeRedecl = func(stmt ast.Stmt, shadowed, inClosure, inDeferGo bool) bool {
		if !shadowed && stmtRedeclares(stmt, name, declIdent) {
			blessRedecl(stmt, name, blessed)
			walkStmt(stmt, shadowed, inClosure, inDeferGo)
			return true
		}
		walkStmt(stmt, shadowed, inClosure, inDeferGo)
		return shadowed
	}

	walkStmts = func(list []ast.Stmt, shadowed, inClosure, inDeferGo bool) {
		sh := shadowed
		for _, stmt := range list {
			sh = walkMaybeRedecl(stmt, sh, inClosure, inDeferGo)
		}
	}

	walkStmts(body.List, false, false, false)

	for _, id := range uses {
		if !blessed[id] {
			s.other = true
			break
		}
	}
	return s
}

// stmtRedeclares reports whether stmt declares name as a new variable that
// shadows the accumulator (declared at declIdent). A lone `name := ...` or a
// `var name ...` qualifies; a multi-assignment `name, y := ...` may instead be
// reassigning an existing name, so it is left to count as a use.
func stmtRedeclares(stmt ast.Stmt, name string, declIdent *ast.Ident) bool {
	switch x := stmt.(type) {
	case *ast.AssignStmt:
		if x.Tok != token.DEFINE || len(x.Lhs) != 1 {
			return false
		}
		id, ok := x.Lhs[0].(*ast.Ident)
		return ok && id.Name == name && id != declIdent
	case *ast.DeclStmt:
		gd, ok := x.Decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			return false
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, n := range vs.Names {
				if n.Name == name && n != declIdent {
					return true
				}
			}
		}
	}
	return false
}

// blessRedecl marks the declaring identifier of a shadowing redeclaration so it
// is not later counted as a use of the accumulator.
func blessRedecl(stmt ast.Stmt, name string, blessed map[*ast.Ident]bool) {
	switch x := stmt.(type) {
	case *ast.AssignStmt:
		if len(x.Lhs) == 1 {
			if id, ok := x.Lhs[0].(*ast.Ident); ok && id.Name == name {
				blessed[id] = true
			}
		}
	case *ast.DeclStmt:
		if gd, ok := x.Decl.(*ast.GenDecl); ok {
			for _, spec := range gd.Specs {
				if vs, ok := spec.(*ast.ValueSpec); ok {
					for _, n := range vs.Names {
						if n.Name == name {
							blessed[n] = true
						}
					}
				}
			}
		}
	}
}

// fieldsDeclareName reports whether a function type's parameters or results
// declare an identifier named name, which would shadow the accumulator inside
// the function body.
func fieldsDeclareName(ft *ast.FuncType, name string) bool {
	declares := func(fl *ast.FieldList) bool {
		if fl == nil {
			return false
		}
		for _, f := range fl.List {
			for _, n := range f.Names {
				if n.Name == name {
					return true
				}
			}
		}
		return false
	}
	return declares(ft.Params) || declares(ft.Results)
}
