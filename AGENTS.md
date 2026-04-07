# Flint - Fluent Linter

Flint validates Go source code that uses the Fluent HTML framework.
It catches mistakes that LLMs commonly make when generating Fluent
code, creating a reinforcing loop: write code, lint, fix, repeat.

## How to use flint in your workflow

Run flint after generating or modifying Fluent code. Read each
diagnostic message and its `fix:` field, then apply the correction.

```bash
flint ./...
```

Or use the library API to lint source code programmatically:

```go
l := flint.New(flint.FluentRegistry())
diags, err := l.Source("file.go", sourceBytes)
for _, d := range diags {
    // d.Message explains the problem
    // d.Fix explains how to correct it
}
```

## Checks

### Static() and RawText() literals

`Static()` marks content for JIT pre-rendering. It must be a string
literal, not a variable or expression. If the content changes between
renders, use `Text()` or `Textf()` instead.

`RawText()` is not HTML-escaped. Dynamic values risk XSS. The first
argument must be a string literal.

```go
// Wrong - flint flags these
div.Static(userName)           // variable, not a literal
div.RawText(htmlContent)       // dynamic content, XSS risk

// Right
div.Static("Copyright 2024")  // literal, safe for JIT
div.Text(userName)             // escaped at runtime
```

### Symbol validation

Catches non-existent functions, methods, types, and variables.
LLMs frequently hallucinate API names that look plausible but do
not exist.

```go
// Wrong
node.Fragment()               // does not exist
div.New().Href("/")           // div has no Href method
inputtype.Telephone           // the constant is inputtype.Tel

// Right
html.Fragment()
a.New().Href("/")
inputtype.Tel
```

### Typed constant enforcement

Methods that accept typed constants reject raw strings. This is a
compile error, but flint catches it earlier with a clearer message.

```go
// Wrong
input.New().Type("email")          // string, not typed constant

// Right
input.New().Type(inputtype.Email)  // typed constant
```

### Argument count

Validates that functions are called with the correct number of
arguments.

```go
// Wrong
meta.UTF8("extra")       // takes 0 arguments

// Right
meta.UTF8()
```

### Constructor optimisation

Detects `New().Method()` patterns where a direct constructor exists.

```go
// Verbose
div.New().Text("hello")

// Better
div.Text("hello")
```

### Typed constructor suggestions

Detects `New()` calls with uniform children where a type-safe
constructor exists. Using the typed constructor catches nesting
errors at compile time.

```go
// Untyped - allows mistakes
ul.New(li.Text("a"), li.Text("b"))

// Type-safe - compiler enforces correct nesting
ul.Items(li.Text("a"), li.Text("b"))
```

### SetAttribute misuse

`SetAttribute` does not return the element, so chaining after it
fails to compile. Flint catches this pattern and suggests
alternatives.

Also detects `SetAttribute` for attributes that have dedicated
typed methods, and suggests `SetData`/`SetAria` for prefixed
attributes.

```go
// Wrong
div.New().SetAttribute("x", "y").Class("z")  // cannot chain
div.New().SetAttribute("class", "x")         // use .Class()
div.New().SetAttribute("data-id", "123")     // use SetData()

// Right
d := div.New().Class("z"); d.SetAttribute("x", "y")
div.New().Class("x")
div.New().SetData("id", "123")
```

### Reserved keyword imports

Catches imports using Go reserved keywords instead of Fluent's
alternative package names.

```go
// Wrong
import "github.com/jpl-au/fluent/html5/select"  // use "dropdown"
import "github.com/jpl-au/fluent/html5/main"     // use "primary"
import "github.com/jpl-au/fluent/html5/var"       // use "variable"
```

## Methods that do NOT exist

These are the most common LLM hallucinations. Flint catches all of
them via symbol validation.

| Non-existent | Use instead |
|-------------|-------------|
| `.Attr()` | Dedicated typed method (`.Class()`, `.Href()`, etc.) |
| `.SetAttr()` | `.SetAttribute()` for custom attributes only |
| `.Attribute()` | Dedicated typed method |
| `node.Fragment()` | `html.Fragment()` |
| `node.StaticText()` | `div.Static()` (method on the element) |
| `node.TextNode()` | `div.Text()` (method on the element) |

## Registry

The registry is generated from the same YAML specs that produce
Fluent's element packages. It contains every function, method, type,
variable, typed parameter, and attribute mapping across all Fluent
packages. When the generator runs, it can regenerate the registry
to stay in sync.

Pass `flint.FluentRegistry()` to enable full validation. Pass `nil`
for Static/RawText checks only.

## Scoping

All registry-based checks (symbols, arity, typed params, constructors,
typed constructors) are scoped to Fluent packages only. They resolve
imports and verify the receiver chain traces back to a registered
package before firing. Non-Fluent code is never flagged.

Static, RawText, and SetAttribute checks are also scoped to Fluent
packages when a registry is available.
