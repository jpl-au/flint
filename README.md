# Flint

A linter for Go code that uses the [Fluent](https://github.com/jpl-au/fluent) HTML framework. It catches wrong method names, incorrect argument types, unsafe patterns, and missed opportunities for type-safe constructors.

It is particularly useful with LLM-generated code. LLMs frequently hallucinate Fluent API names or use raw strings where typed constants are required. Flint catches these mistakes and each diagnostic includes a `fix:` field explaining the correction, so the LLM can self-correct without human intervention.

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

Flags:

| Flag | Description |
|------|-------------|
| `-include-tests` | Include `_test.go` files (excluded by default) |
| `-no-registry` | Disable symbol validation (only run Static/RawText checks) |

Exit codes: `0` clean, `1` diagnostics found, `2` usage or I/O error.

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

Every function, method, type, and variable reference is checked against the generated registry. This catches typos and hallucinated APIs.

```go
node.Fragment()           // flagged: node.Fragment does not exist
div.New().Href("/")       // flagged: method Href does not exist on this element
inputtype.Telephone       // flagged: inputtype.Telephone does not exist
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

### Constructor optimisation

The `New().Method()` pattern is flagged when a direct constructor exists for that method.

```go
div.New().Text("hello")  // flagged: use div.Text("hello") directly
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

### Reserved keyword imports

Go reserved keywords used as import paths are flagged with the correct Fluent alternative.

```go
import "github.com/jpl-au/fluent/html5/select"  // flagged: use "dropdown" instead
import "github.com/jpl-au/fluent/html5/main"     // flagged: use "primary" instead
```

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
