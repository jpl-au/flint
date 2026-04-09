package flint

import (
	"fmt"
	"strings"
)

// wrap places a Go expression inside a minimal valid file so the parser
// can handle it. The imports cover the packages that test snippets use.
func wrap(expr string) []byte {
	return fmt.Appendf(nil, `package example

import (
	"fmt"
	"github.com/jpl-au/fluent/html5/div"
	"github.com/jpl-au/fluent/text"
)

var _ = fmt.Sprintf
var _ = text.Static
var _ = div.New

func build() {
	%s
}
`, expr)
}

// wrapWithImports builds a valid Go file from a snippet with custom imports.
func wrapWithImports(imports []string, body string) []byte {
	var importBlock strings.Builder
	for _, imp := range imports {
		importBlock.WriteString(fmt.Sprintf("\t%q\n", imp))
	}
	return fmt.Appendf(nil, `package example

import (
%s)

func build() {
	%s
}
`, importBlock.String(), body)
}

// wrapReturningFunc builds a valid Go file with a function that accepts
// a colour parameter and returns interface{}, allowing chained return
// value tests.
func wrapReturningFunc(imports []string, body string) []byte {
	var importBlock strings.Builder
	for _, imp := range imports {
		importBlock.WriteString(fmt.Sprintf("\t%q\n", imp))
	}
	return fmt.Appendf(nil, `package example

import (
%s)

func build(colour string) interface{} {
	%s
}
`, importBlock.String(), body)
}

// testRegistry returns a minimal registry for testing symbol validation.
func testRegistry() *Registry {
	return &Registry{
		Packages: map[string]Package{
			"github.com/jpl-au/fluent/html5/div": {
				Functions: map[string]int{"New": -1},
				Types:     map[string]bool{"Element": true},
				Methods: map[string]bool{
					"Class": true, "ID": true, "Text": true,
					"Static": true, "Add": true, "Dynamic": true,
				},
			},
			"github.com/jpl-au/fluent/html5/input": {
				Functions: map[string]int{
					"New": 0, "Text": 2, "Email": 1,
					"Password": 1, "Checkbox": 2,
				},
				Types: map[string]bool{"Element": true},
				Methods: map[string]bool{
					"Class": true, "ID": true, "Name": true,
					"Value": true, "Type": true, "Required": true,
					"Placeholder": true,
				},
			},
			"github.com/jpl-au/fluent/text": {
				Functions: map[string]int{
					"Static": 1, "Text": 1, "RawText": 1,
					"Textf": -1, "RawTextf": -1,
				},
				Types: map[string]bool{"Node": true},
			},
			"github.com/jpl-au/fluent/node": {
				Functions: map[string]int{
					"Condition": 1, "When": 2, "Unless": 2,
					"Func": 1, "Funcs": 1, "Memoise": 2,
				},
				Types: map[string]bool{
					"Node": true, "Element": true, "Dynamic": true,
					"Attribute": true, "Memoiser": true,
				},
			},
			"github.com/jpl-au/fluent/html5/attr/inputtype": {
				Vars: map[string]bool{
					"Text": true, "Email": true, "Password": true,
				},
				Functions: map[string]int{"Custom": 1},
			},
		},
	}
}
