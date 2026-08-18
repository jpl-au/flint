package flint

import (
	"go/ast"
	"go/token"
	"strings"
)

// allowPrefix is the comment form of a suppression directive:
//
//	//flint:allow <check> <reason>
//
// The check name is the stable name every diagnostic carries (raw-text,
// setattr-key, ...), and the reason is mandatory: a suppression without a
// recorded judgement is exactly the unexplained residue the directive exists
// to eliminate. A directive on its own line applies to the next line; a
// trailing directive applies to its own line. There is deliberately no
// file-wide or project-wide form - a suppression is a per-site decision, and
// keeping the reason next to the code is the point.
const allowPrefix = "//flint:allow"

// applyAllowDirectives drops diagnostics a //flint:allow directive covers and
// reports malformed directives, which would otherwise silently suppress
// nothing. src is the file's source, used to tell a standalone directive from
// a trailing one.
func applyAllowDirectives(fset *token.FileSet, file *ast.File, src []byte, diags []Diagnostic) []Diagnostic {
	allowed := map[int]map[string]bool{}
	var malformed []Diagnostic

	for _, group := range file.Comments {
		for _, c := range group.List {
			if !strings.HasPrefix(c.Text, allowPrefix) {
				continue
			}
			fields := strings.Fields(strings.TrimPrefix(c.Text, allowPrefix))
			if len(fields) < 2 {
				malformed = append(malformed, Diagnostic{
					Check:    "allow",
					Pos:      fset.Position(c.Slash),
					End:      fset.Position(c.End()),
					Severity: Warning,
					Message:  "//flint:allow needs a check name and a reason. This directive suppresses nothing.",
					Fix:      "Write //flint:allow <check> <reason>. For example: //flint:allow raw-text trusted markup owned by the server. Every diagnostic prints its check name.",
				})
				continue
			}

			line := fset.Position(c.Slash).Line
			if standalone(fset, c, src) {
				line++
			}
			if allowed[line] == nil {
				allowed[line] = map[string]bool{}
			}
			allowed[line][fields[0]] = true
		}
	}

	if len(allowed) == 0 {
		return append(diags, malformed...)
	}

	kept := diags[:0]
	for _, d := range diags {
		if allowed[d.Pos.Line][d.Check] {
			continue
		}
		kept = append(kept, d)
	}
	return append(kept, malformed...)
}

// standalone reports whether a comment has only whitespace before it on its
// line, meaning it is a directive line of its own rather than a trailing
// comment.
func standalone(fset *token.FileSet, c *ast.Comment, src []byte) bool {
	pos := fset.Position(c.Slash)
	start := pos.Offset - (pos.Column - 1)
	if start < 0 || pos.Offset > len(src) {
		return false
	}
	return strings.TrimSpace(string(src[start:pos.Offset])) == ""
}
