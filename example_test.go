package flint_test

import (
	"fmt"

	"github.com/jpl-au/flint"
)

func Example() {
	// Enable symbol validation with the generated registry.
	flint.WithRegistry(flint.FluentRegistry())

	// Source code that an LLM might generate.
	src := []byte(`package ui

import (
	"github.com/jpl-au/fluent/html5/div"
	"github.com/jpl-au/fluent/html5/input"
	"github.com/jpl-au/fluent/html5/attr/inputtype"
	"github.com/jpl-au/fluent/node"
	"github.com/jpl-au/fluent/text"
)

func render(name string) {
	// Correct usage
	_ = div.New().Class("container").Text("Hello")
	_ = input.Email("email").Required()
	_ = text.Static("Copyright 2024")
	_ = node.Condition(true)
	_ = inputtype.Email

	// Mistakes an LLM might make
	_ = div.New().Class("x").Static(name)                // Static with variable
	_ = node.Fragment()                                  // Fragment does not exist
	_ = div.New().Href("/")                              // div has no Href method
	_ = inputtype.Telephone                              // not a valid inputtype
	_ = text.RawText(name)                               // RawText with variable
	_ = input.New().Type("email")                        // string where typed constant expected
	_ = div.New().SetAttribute("hx-get", "/items").ID("x") // chaining after SetAttribute
	_ = div.New().Text("hello")                          // should use div.Text("hello")
}
`)

	diags, err := flint.Source("example.go", src)
	if err != nil {
		fmt.Printf("parse error: %v\n", err)
		return
	}

	for _, d := range diags {
		fmt.Printf("line %d: %s\n", d.Pos.Line, d.Message)
	}

	// Output:
	// line 20: Static() argument must be a string literal; got variable "name"
	// line 21: node.Fragment does not exist
	// line 22: method Href does not exist on this element
	// line 23: inputtype.Telephone does not exist
	// line 24: RawText() first argument must be a string literal; got variable "name"
	// line 25: .Type() expects a typed constant, not a string literal "email"
	// line 26: SetAttribute does not return the element; cannot chain .ID() after it
	// line 27: use div.Text(...) directly instead of div.New().Text(...)
}
