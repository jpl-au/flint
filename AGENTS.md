# Flint - Fluent Linter

Run flint after you generate or edit Fluent code. Read each diagnostic's message and its `fix:` field, then apply the correction. Write code, lint, fix, repeat.

```bash
flint ./...
```

Severities: an **error** does not compile and you must fix it. A **warning** compiles but has a reason to change. An **info** is advisory and safe to leave.

Exit codes: `0` nothing found at or above the `-fail-on` level, `1` one or more diagnostics at or above it, `2` usage or I/O error. `-fail-on` takes `error`, `warning` (the default) or `never`. Info never fails a run at any level.

Every diagnostic prints its check name, as in `warning[setattr-key]: ...`. Flint prints each `fix:` paragraph once per run. A later diagnostic that shares an earlier fix prints its message alone. To read that fix, find the first diagnostic for the same check, or use `-json`.

To suppress a diagnostic at one site, add a directive that names the check and gives a reason: `//flint:allow raw-text trusted server-owned markup`. On its own line, the directive covers the next line. At the end of a line, it covers that line. The reason is mandatory. Do not suppress a diagnostic you have not judged. Fix the code instead.

Pass `-json` for machine-readable output. Flint prints one JSON object per diagnostic per line, with the file, positions, severity, check name, message and fix. Exit codes do not change.

```bash
flint -json ./...
```

## Element lookup

Use `-info` to read the API of an element before you write code. It shows types, constructors, typed constructors, methods with their typed parameters, attribute mappings, and vars. A deprecated entry carries its deprecation note. Use the replacement that the note names.

```bash
flint -info div
flint -info input
```

To restrict the output, name one or more sections. Each has a long form, and some have a short form: `types`, `constructors`/`ctors`, `typed-constructors`/`typed`, `methods`, `attributes`/`attrs`, `vars`, `elements`.

```bash
flint -info div methods         # Just the methods
flint -info input ctors attrs   # Constructors and attribute mappings
flint -info ol typed            # Typed constructors only
```

The svg package holds more than one element. Query the package to list its elements. A bare element name resolves inside the package. The `package:element` form, such as `flint -info svg:text`, selects an element that shares its name with a package.

## Library API

```go
l := flint.New(flint.FluentRegistry())
diags, err := l.Source("file.go", sourceBytes)
for _, d := range diags {
    // d.Message explains the problem
    // d.Fix explains how to correct it
}
```

## Registry

The registry comes from the same YAML specs that produce Fluent's element packages. It holds every function, method, type, variable, typed parameter and attribute mapping in core fluent and fluent-security. Pass `flint.FluentRegistry()` for full validation. Pass `nil` to run only the literal Static/RawText checks and the SetAttribute-chain check.

## Scoping

Flint scopes every registry-based check to Fluent packages. These are the symbols, arity, typed params, constructors, typed constructors, deprecated, duplicate attrs and url scheme checks. Each one resolves the imports and confirms that the receiver chain reaches a registered package before it reports. Flint does not report on other code.

When a registry is available, flint also scopes the Static, RawText and SetAttribute checks to Fluent packages.
