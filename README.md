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

Exit codes: `0` nothing found at or above the `-fail-on` level, `1` one or more diagnostics at or above it, `2` usage or I/O error. The default level is `warning`, so any error or warning fails the run. Pass `-fail-on=error` to fail on errors alone, or `-fail-on=never` to report without failing. Run `flint -h` for the full flag reference.

Beyond linting, `-info` prints an element's API surface straight from the registry - types, constructors, methods, typed parameters, attribute mappings - so you can look up what an element offers without leaving the terminal. Deprecated entries carry their deprecation note.

```bash
flint -info div
flint -info input ctors attrs
```

## What it checks

The first column is the stable check name, printed with every diagnostic (`warning[setattr-key]: ...`) and carried in `-json` output and telemetry. The severity decides whether the run fails, against the `-fail-on` level.

| Check | Severity | Catches |
|-------|----------|---------|
| `symbols` | error | A function, method, type or var that does not exist in the registry |
| `arity` | error | Wrong argument count on a function call |
| `method-arity` | error | Wrong argument count on a method call |
| `imports` | error | A Go reserved keyword as an import path, in place of Fluent's alternative |
| `setattr-chain` | error | Chaining after `SetAttribute()`, which does not return the element |
| `static` | warning | `Static()` given something other than a string literal. The paired-constructor idiom is exempt: a function whose name ends in `Static` forwarding its own parameter is the contract, not a violation |
| `raw-text` | warning | `RawText()` given a non-literal, which skips HTML escaping |
| `setattr-key` | warning | `SetAttribute()` where a dedicated typed method exists |
| `verbatim-key` | warning | A key built at run time on `SetAttribute()`, `SetData()`, `SetAria()` or `SetEvent()`. Keys render verbatim - the value is escaped, the key is not - so a key from user input changes the markup structure. A named constant also fires; the `fix:` names that case a false positive |
| `duplicate-attr` | warning | An attribute set twice: a repeated setter whose last value silently wins, or `SetAttribute()` duplicating an attribute a dedicated method - or the chain's constructor, `input.Text` pins `type="text"` - already set, in the same chain or on a later line |
| `allow` | warning | A malformed `//flint:allow` directive, which would silently suppress nothing |
| `url-scheme` | warning | A URL literal whose scheme fluent's runtime filter rejects (`javascript:`, `data:`, ...); it renders as the `#fluent-unsafe-url` sentinel |
| `typed-params` | warning | A raw string where a typed constant is expected, or `Custom()` re-creating a predefined constant; the `fix:` names the exact constant when the value matches one |
| `typed-constructors` | warning | `New()` with same-package children that a typed constructor expresses |
| `shadows` | warning | A local name shadowing an imported Fluent package |
| `nesting` | warning | A child from the same element package inside `a`, `button`, `form` or `label`, which HTML forbids nesting in themselves; `a.New(a.Static("Back"))` builds an anchor inside an anchor, and browsers unnest it |
| `deprecated` | warning | Use of a deprecated Fluent API; the `fix:` names the replacement |
| `node-append` | warning | Children appended from a `defer` or goroutine after the splat, or `make([]node.Node, n)` with a non-zero length |
| `node-append` | info | A `[]node.Node` grown by append and splatted. Correct as written, and the cheapest option for render-once output |
| `constructors` | info | A `New()` chain a package-level constructor would build in one call, matched on what the chain sets rather than on method names |
| `buffer-hint` | info | An element already holding 4 KiB or more of static content, which could carry `.BufferHint(n)` |

Each diagnostic carries a `fix:` line describing the correction for the code it found. In the terminal output a given `fix:` paragraph is printed the first time it appears in a run and omitted on later diagnostics that share it, so a check firing across many call sites reports every site without repeating the same explanation each time. Every diagnostic still prints, and `-json` always carries the full `fix` on every object.

## Severity levels

- **error** - incorrect code that will not compile (a missing symbol, wrong arity, a chain that cannot compile). The Go compiler rejects this code too, so flint reports it first.
- **warning** - code that compiles but carries a real reason to change: a security or correctness hazard, a silent bug, a duplicate attribute, or a typed API sidestepped. This tier holds what only flint can see.
- **info** - advisory. The code is correct and fine as written; an optional alternative exists. Never fails the run, at any `-fail-on` level.

`-fail-on` sets the lowest severity that exits `1`:

| Level | Exits `1` on |
|-------|--------------|
| `error` | errors only |
| `warning` (default) | errors and warnings |
| `never` | nothing; always exits `0` |

The default is `warning` because the error tier duplicates the compiler. A build step already rejects every error flint reports, so failing on errors alone would never fail a run that `go build` had not failed first.

## Suppressing a diagnostic

Some flagged patterns are deliberate - a `RawText` block whose contract is trusted, server-owned markup, say. Record that judgement where the code is:

```go
//flint:allow raw-text the ErrorStatic contract is trusted server-owned markup
return div.New(text.RawText(markup))
```

The check name is required and is the one printed with the diagnostic; the reason is required too, so the next reader inherits the judgement instead of re-auditing the line. A directive on its own line covers the next line; a trailing directive covers its own line. There is deliberately no file- or project-wide form: a suppression is a per-site decision.

## Telemetry

Opt-in and **off by default**: nothing is collected, and nothing leaves your
machine, until you turn it on. When enabled, runs are recorded as greppable
text files beneath your user cache directory.

```bash
flint -telemetry status   # Show the current mode
flint -telemetry local    # Record runs to local .tlf files
flint -telemetry off      # Stop collecting
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

A generated registry (`FluentRegistry()`) provides the complete API surface of every Fluent package - functions, methods, types, variables, typed parameters, attribute mappings, and what each constructor already sets. The registry is generated from the same YAML specifications that produce the Fluent element packages, so it stays in sync automatically.

Generated files (containing `// Code generated` and `DO NOT EDIT`) are skipped automatically.

## Licence

MIT
