# Flint - Fluent Linter

Run flint after generating or modifying Fluent code. Read each
diagnostic's message and its `fix:` field, then apply the correction:
write code, lint, fix, repeat.

```bash
flint ./...
```

Severities: an **error** will not compile and must be fixed; a
**warning** compiles but carries a real reason to change; **info** is
advisory and safe to leave. Exit codes: `0` clean or advisory-only,
`1` errors found, `2` usage or I/O error. Every diagnostic prints its
stable check name (`warning[setattr-key]: ...`).

A deliberate pattern flint flags can be suppressed per site with a
reasoned directive naming the check:
`//flint:allow raw-text trusted server-owned markup`. On its own line
it covers the next line; trailing, it covers its own line. The reason
is mandatory. Do not add a directive to silence a diagnostic you have
not judged - fix the code instead.

Pass `-json` for machine-readable output: one JSON object per
diagnostic per line, carrying the file, positions, severity, check
name, message, and fix. Exit codes are unchanged.

```bash
flint -json ./...
```

## Element lookup

Use `-info` to look up the API surface of any element before writing
code. This shows types, constructors, typed constructors, methods
(with any typed parameters), attribute mappings, and vars. Deprecated
entries carry their deprecation note, so prefer the named replacement.

```bash
flint -info div
flint -info input
```

Restrict the output by section; each accepts a long form and (where
useful) a short form: `types`, `constructors`/`ctors`,
`typed-constructors`/`typed`, `methods`, `attributes`/`attrs`, `vars`,
`elements`.

```bash
flint -info div methods         # Just the methods
flint -info input ctors attrs   # Constructors and attribute mappings
flint -info ol typed            # Typed constructors only
```

For a multi-element package (svg), querying the package lists its
elements, a bare element name resolves within it, and the
`package:element` form (`flint -info svg:text`) disambiguates an
element shadowed by a package of its own.

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

The registry is generated from the same YAML specs that produce
Fluent's element packages, so it is the authoritative surface: every
function, method, type, variable, typed parameter and attribute
mapping across core fluent and fluent-security. Pass
`flint.FluentRegistry()` for full validation, or `nil` for the
literal Static/RawText and SetAttribute-chain checks only.

## Scoping

All registry-based checks (symbols, arity, typed params, constructors,
typed constructors, deprecated, duplicate attrs, url scheme) are scoped
to Fluent packages only. They resolve imports and verify the receiver
chain traces back to a registered package before firing. Non-Fluent
code is never flagged.

Static, RawText, and SetAttribute checks are also scoped to Fluent
packages when a registry is available.
