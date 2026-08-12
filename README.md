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
| `-telemetry <value>` | Set the telemetry mode (`off`, `local`, `on`) or show it (`status`), then exit |
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

### Telemetry

Telemetry is opt-in and **off by default**. Nothing is collected, and nothing
leaves your machine, until you turn it on.

```bash
flint -telemetry status   # Show the current mode
flint -telemetry local    # Record runs to local .tlf files
flint -telemetry off      # Stop collecting
```

When enabled, an ordinary lint run records its diagnostics and attribute usage
to a single `<run-id>.tlf` file beneath the user cache directory (see
`os.UserCacheDir`). The file is greppable text with three regions, `[meta]`,
`[issues]` and `[attrs]`. The chosen mode persists beneath the user config
directory (see `os.UserConfigDir`), so wiping the cache does not lose the
choice.

The `on` mode reserves the "collect and upload" meaning for when uploading
exists. Until then it behaves exactly like `local`: everything stays on disk.

## What it checks

The first column is the stable check name that appears in telemetry. The
severity decides whether the run fails: only **error** sets exit code `1`.

| Check | Severity | Catches |
|-------|----------|---------|
| `symbols` | error | A function, method, type or var that does not exist in the registry |
| `arity` | error | Wrong argument count on a function call |
| `method-arity` | error | Wrong argument count on a method call |
| `imports` | error | A Go reserved keyword as an import path, in place of Fluent's alternative |
| `setattr-chain` | error | Chaining after `SetAttribute()`, which does not return the element |
| `static` | warning | `Static()` given something other than a string literal |
| `raw-text` | warning | `RawText()` given a non-literal, which skips HTML escaping |
| `setattr-key` | warning | `SetAttribute()` where a dedicated typed method exists |
| `typed-params` | warning | A raw string where a typed constant is expected |
| `typed-constructors` | warning | `New()` with same-package children that a typed constructor expresses |
| `shadows` | warning | A local name shadowing an imported Fluent package |
| `deprecated` | warning | Use of a deprecated Fluent API; the `fix:` names the replacement |
| `node-append` | warning | Children appended from a `defer` or goroutine after the splat, or `make([]node.Node, n)` with a non-zero length |
| `node-append` | info | A `[]node.Node` grown by append and splatted. Correct as written, and the cheapest option for render-once output |
| `constructors` | info | `New().Method()` where a direct constructor exists |
| `buffer-hint` | info | An element already holding 4 KiB or more of static content, which could carry `.BufferHint(n)` |

Each diagnostic carries a `fix:` line describing the correction for the code it
found, and `-info <element>` shows an element's API surface from the registry.

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
