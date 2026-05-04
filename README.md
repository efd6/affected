# affected

`affected` identifies Go packages within a module that are transitively
affected by changes between two git commits. It outputs one package
import path per line, suitable for passing directly to `go test`.

## Install

```
go install github.com/efd6/affected@latest
```

## Usage

```
affected [flags] [from-ref] [to-ref]
```

With no arguments, it computes the merge-base against the upstream
default branch and compares to HEAD — the common CI case.

### Flags

| Flag | Description |
|------|-------------|
| `-tags` | Build tags (comma-separated); `go list` runs once per tag value. Use `-` to include an untagged run. |
| `-json` | Output as JSON array with metadata (includes `tags` field when `-tags` is given) |
| `-include-no-tests` | Include packages that have no test files |
| `-relative` | Output `./relative` paths instead of full import paths |

### Examples

Run tests for everything affected since the branch diverged:

```
go test $(affected -relative)
```

Explicit commit range:

```
affected HEAD~5 HEAD
```

With integration build tag — behaves like `go test -tags integration`
(sees unconstrained code plus integration-gated code):

```
affected -tags integration HEAD~5 HEAD
```

Multiple tags — runs `go list` once with `integration` and once with
`fips`, then merges:

```
affected -tags integration,fips HEAD~5 HEAD
```

Include an explicit untagged run alongside integration (captures files
gated with `//go:build !integration` that would be hidden under the
integration tag):

```
affected '-tags=-,integration' HEAD~5 HEAD
```

JSON output showing which packages were directly changed, transitively
affected, and which tag configurations include them:

```
affected -json -tags integration HEAD~3 HEAD
```

Example JSON entries:

```json
[
  {
    "import_path": "example.com/mod/pkg/foo",
    "direct": true,
    "tags": ["integration"]
  },
  {
    "import_path": "example.com/mod/pkg/bar",
    "direct": false,
    "tags": ["-", "integration"]
  }
]
```

The `tags` array lists each configuration that includes the package:
`"-"` means visible with no build tags, named entries mean the package
is visible under that tag. When no `-tags` flag is given, the field is
omitted entirely.

## How it works

1. Runs `git diff --name-only` between the two refs to get changed files.
2. Maps each changed file to its owning Go package (walking up from
   subdirectories like `testdata/` to find the nearest enclosing package).
3. Builds a reverse import graph of the module using `go list -json ./...`.
   When `-tags` is specified, runs `go list` once per tag value and merges
   the results, recording which configurations each package belongs to.
4. If `go.mod` changed, diffs the require/replace directives to find
   external modules whose version changed, then identifies internal
   packages that transitively depend on those modules.
5. Starting from the combined seed set, walks the reverse graph (BFS) to
   collect all transitively-dependent packages.
6. Filters to packages with test files (unless `-include-no-tests`).

## Cross-package test assets

If tests in one package read files from an unrelated directory at runtime,
the tool cannot detect that relationship automatically. For this case,
place a `.affected` file in the package directory with one filepath pattern
per line (relative to module root). When any matching file changes, the
package is added to the seed set.

```
# .affected
shared/fixtures/*.json
testdata/common/**
```

## Limitations

- Each `go list -json ./...` invocation takes a few seconds on large
  modules. With `-tags`, the cost multiplies by the number of
  configurations. Results are not cached between runs (yet).
- Cross-package runtime file reads are invisible without a `.affected`
  manifest.
- When `go.mod` changes, the tool identifies affected packages by checking
  transitive dependency closures. It does not distinguish between API
  changes and no-op version bumps in external modules.
