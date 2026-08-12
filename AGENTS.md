# Flint - Fluent Linter

Flint validates Go source code that uses the Fluent ecosystem -
core Fluent and the companion fluent-security package. It catches
mistakes that AI tools commonly make when generating Fluent code,
creating a reinforcing loop: write code, lint, fix, repeat.

## How to use flint in your workflow

Run flint after generating or modifying Fluent code. Read each
diagnostic message and its `fix:` field, then apply the correction.

```bash
flint ./...
```

Use `-info` to look up the API surface of any element before writing
code. This shows types, constructors, typed constructors, methods
(with any typed parameters), attribute mappings, and vars.

```bash
flint -info div
flint -info input
```

Pass one or more section names after the element to restrict the
output. Each accepts a long form and (where useful) a short form:
`types`, `constructors`/`ctors`, `typed-constructors`/`typed`,
`methods`, `attributes`/`attrs`, `vars`, `elements`.

```bash
flint -info div methods         # Just the methods
flint -info input ctors attrs   # Constructors and attribute mappings
flint -info ol typed            # Typed constructors only
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

## Registry

The registry is generated from the same YAML specs that produce
Fluent's element packages. It contains every function, method, type,
variable, typed parameter, and attribute mapping across the Fluent
ecosystem packages (core fluent, plus fluent-security). When the
generator runs, it can regenerate the registry to stay in sync.

Pass `flint.FluentRegistry()` to enable full validation. Pass `nil`
for Static/RawText checks only.

## Scoping

All registry-based checks (symbols, arity, typed params, constructors,
typed constructors, deprecated) are scoped to Fluent packages only. They resolve
imports and verify the receiver chain traces back to a registered
package before firing. Non-Fluent code is never flagged.

Static, RawText, and SetAttribute checks are also scoped to Fluent
packages when a registry is available.
