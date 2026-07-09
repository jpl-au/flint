# Flint

A linter for Go code that uses the [Fluent](https://github.com/jpl-au/fluent) ecosystem - core Fluent and the companion [fluent-security](https://github.com/jpl-au/fluent-security) package. It catches wrong method names, incorrect argument types, unsafe patterns, and missed opportunities for type-safe constructors.

It is particularly useful with AI-generated code. AI tools frequently hallucinate Fluent API names or use raw strings where typed constants are required. Flint catches these mistakes and each diagnostic includes a `fix:` field explaining the correction, so the agent can self-correct without human intervention.

## Install

```bash
go install github.com/jpl-au/flint/cmd/flint@latest
```

## Usage

```bash
flint ./...              # Check all Go files recursively
flint ./views            # Check a specific directory
flint views/home.go      # Check a single file
cat file.go | flint -    # Read from stdin
```

### Element info

Use `-info` to inspect the registry entry for any fluent element. This displays its types, constructors, typed constructors, methods (with any typed parameters), attribute mappings, and vars.

```bash
flint -info div          # Show everything about <div>
flint -info input        # Show everything about <input>
flint -info ol           # Show everything about <ol>
```

For a multi-element package (svg, whose single import path hosts many shapes),
querying the package lists its elements, a bare element name resolves within
it, and the `package:element` form disambiguates an element whose name is
shadowed by a package of its own (the svg `text` shape, versus the text node
package).

```bash
flint -info svg          # The shared svg surface, plus its elements
flint -info rect         # The rect shape within the svg package
flint -info svg:text     # The svg text shape, not the text node package
```

Pass one or more section names after the element to restrict the output. Each section accepts a long form and (where useful) a short form:

| Long form | Short form |
|-----------|------------|
| `types` | |
| `constructors` | `ctors` |
| `typed-constructors` | `typed` |
| `methods` | |
| `attributes` | `attrs` |
| `vars` | |
| `elements` | |

```bash
flint -info div methods         # Just the methods
flint -info input ctors attrs   # Constructors and attribute mappings
flint -info ol typed            # Typed constructors only
```

### Flags

| Flag | Description |
|------|-------------|
| `-include-tests` | Include `_test.go` files (excluded by default) |
| `-no-registry` | Disable registry-backed symbol validation; the literal Static/RawText and SetAttribute-chain checks still run |
| `-info <element> [section]...` | Show registry info for an element and exit |
| `-version` | Print flint version and exit |

Exit codes: `0` clean or advisory-only, `1` errors found, `2` usage or I/O error.

### Severity levels

Each diagnostic carries a severity:

- **error** - incorrect code that will not compile (a missing symbol, wrong
  arity, a chain that cannot compile). Sets exit code `1`.
- **warning** - code that compiles but carries a real reason to change: a
  security or correctness hazard, a silent bug, a duplicate attribute, or a
  typed API sidestepped. Reported, but does not fail the run.
- **info** - advisory. The code is correct and fine as written; an optional
  alternative exists. Reported, never fails the run.

## What it checks

### Static and RawText literals

`Static()` content is marked for JIT pre-rendering and must be a string literal. `RawText()` content is not HTML-escaped, so passing dynamic values risks XSS.

```go
div.Static(userName)           // flagged: got variable "userName"
div.RawText(htmlContent)       // flagged: got variable "htmlContent"

div.Static("Copyright 2024")  // ok
div.Text(userName)             // ok - Text() escapes at runtime
```

### Symbol validation

Every function, method, type, and variable reference is checked against the generated registry. The registry covers the Fluent ecosystem packages, so calls into both core fluent and fluent-security are validated.

```go
node.Fragment()              // flagged: node.Fragment does not exist
div.New().Href("/")          // flagged: method Href does not exist on this element
inputtype.Telephone          // flagged: inputtype.Telephone does not exist
```

### Typed constant enforcement

Fluent methods that accept typed constants will reject raw strings.

```go
input.New().Type("email")          // flagged: expects typed constant, not string
input.New().Type(inputtype.Email)  // ok
```

### Argument count

Function and method calls are checked against their expected argument counts.

```go
meta.UTF8("extra")       // flagged: UTF8 takes 0 arguments, got 1
```

### Constructor optimisation (advisory)

The `New().Method()` pattern is flagged at **info** when a direct constructor
exists for that method. Both forms are equivalent in behaviour and cost, so this
is a readability suggestion, not a fix.

```go
div.New().Text("hello")  // info: use div.Text("hello") directly
```

### Typed constructor suggestions

When `New()` is called with children that all come from the same element package, flint suggests the type-safe constructor instead.

```go
ul.New(li.Text("a"), li.Text("b"))  // flagged: use ul.Items(...) instead
tr.New(td.Text("x"), td.Text("y")) // flagged: use tr.Cells(...) instead
```

### SetAttribute misuse

Chaining after `SetAttribute()` is flagged because the method does not return the element. Using `SetAttribute` for attributes that have dedicated typed methods is also flagged.

```go
div.New().SetAttribute("x", "y").Class("z")  // flagged: cannot chain after SetAttribute
div.New().SetAttribute("class", "x")         // flagged: use .Class() instead
div.New().SetAttribute("data-id", "123")     // flagged: use SetData("id", ...) instead
```

There is also a safety reason to prefer the typed methods. For URL attributes (`href`, `src`, ...) the typed methods scheme-filter the value against the allowlist, whereas `SetAttribute` escapes the value but does not filter it. `SetAttributeRaw` does neither - it stores the value verbatim, for trusted content only.

### Reserved keyword imports

Go reserved keywords used as import paths are flagged with the correct Fluent alternative.

```go
import "github.com/jpl-au/fluent/html5/select"  // flagged: use "dropdown" instead
import "github.com/jpl-au/fluent/html5/main"     // flagged: use "primary" instead
```

### Package alias shadowing

A local variable, parameter, or range binding named after an imported fluent
package shadows it. Names like `input`, `form`, and `option` are natural
variable names, so this is an easy slip - and while the name is shadowed, code
that reads as a package reference is not one. Flint names the collision
directly so you can rename one side; its other diagnostics resolve identifiers
through the file's imports, so they may also be misleading until it is fixed.

```go
import "github.com/jpl-au/fluent/html5/input"

func handle(input string) {  // flagged: parameter "input" shadows the package
	...
}
```

### Children built with append (advisory)

A local `[]node.Node` grown with `append` and then passed into a Fluent call with
`...` is reported at **info**. Building the slice and passing it in is correct and,
for render-once output, the cheapest option - so this is an advisory: Fluent can
compose the children directly if you prefer. The fix names the helper for the shape
it sees - `node.When`/`node.Unless` for a conditional child, `node.Map`/`node.Funcs`
for a loop, variadic children or `.Add(...)` for the plain case.

```go
// info: optional - Fluent can compose these directly
kids := []node.Node{}
if isAdmin { kids = append(kids, span.Text("admin")) }
return div.New(kids...)

// the shorter form
return div.New(node.When(isAdmin, span.Text("admin")))
```

Two shapes are genuine problems and stay at **warning**:

- **Appending in a `defer` or goroutine** after the slice has been passed in -
  those children run too late and never render.
- **`make([]node.Node, n)`** with a non-zero length, then `append` - it seeds `n`
  nil entries and doubles the slice; almost always a slip for `make([]node.Node, 0, n)`.

The check is conservative: it fires only when the slice is a local whose element
type resolves to a Fluent node, is grown by at least one `append`, is passed into
exactly one call with `...`, and is used nowhere else (not indexed, returned, or
passed without `...`). Slices that escape those bounds are left alone.

### Buffer hint for large static content (advisory)

When a Fluent element already holds at least 4 KiB of static content, flint shows
an **info** suggesting you chain `.BufferHint(n)` on it. Above that size the render
buffer comes from a shared pool, and a hint lets fluent reuse the buffer between
renders instead of growing a new one each time. It is a suggestion only - the hint
is optional and setting it cannot break anything, so the size gate keeps it quiet
on all but genuinely large trees.

```go
// info: this element renders at least 5120 bytes of static content
div.Static(`...five KiB of markup as a literal...`).BufferHint(5120)
```

The size is an estimate from the string literals flint can see: dynamic content
counts as zero, so in typical trees the real render is at least that large.

## Library usage

Flint can be used as a library for custom tooling or editor integrations.

```go
l := flint.New(flint.FluentRegistry())

diags, err := l.Source("file.go", sourceBytes)
if err != nil {
    // parse error
}
for _, d := range diags {
    fmt.Printf("%s:%d:%d: %s\n", d.Pos.Filename, d.Pos.Line, d.Pos.Column, d.Message)
    if d.Fix != "" {
        fmt.Printf("  fix: %s\n", d.Fix)
    }
}
```

## How it works

Flint parses Go source using `go/ast` and walks the AST looking for patterns that indicate misuse of the Fluent API. It has no dependency on Fluent itself.

A generated registry (`FluentRegistry()`) provides the complete API surface of every Fluent package - functions, methods, types, variables, typed parameters, and attribute mappings. The registry is generated from the same YAML specifications that produce the Fluent element packages, so it stays in sync automatically.

Generated files (containing `// Code generated` and `DO NOT EDIT`) are skipped automatically.

## Licence

MIT
