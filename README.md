# lineslint

A go/analysis analyzer that reports `strings.Lines` and `bytes.Lines` elements used as if the trailing newline were absent.

```go
for line := range strings.Lines(string(output)) {
	if line == "beta" { // reported: line is "beta\n", never "beta"
		continue
	}
	info, err := os.Lstat(line) // reported: no file is named "path\n"
	...
}
```

```
$ lineslint ./...
main.go:12:5: line still ends in "\n" (Lines keeps line terminators), so it never equals "beta" (trim it first)
main.go:15:20: line still ends in "\n" (Lines keeps line terminators), so os.Lstat cannot use it as a file path (trim it first)
```

## Why

Go 1.24 added `strings.Lines` and `bytes.Lines`. Unlike `bufio.Scanner` and `strings.Split`, every element keeps its line terminator (`"\n"` or `"\r\n"`). Code written with the Scanner habit compiles and runs, and silently misbehaves for every line except an unterminated last one:

- `line == "beta"` is never true.
- `os.Lstat(line)` fails for every path but the last, and a loop that skips errors drops them without notice.
- `seen[line]` misses the same text stored without the terminator.
- `strconv.Atoi(line)`, `net.ParseIP(line)` and `url.Parse(line)` reject a valid value.
- `line == ""` never detects a blank line, because a blank line is `"\n"`.

A real instance is tw93/Mole `cmd/analyze/scanner.go` at commit `f32fa0c6b082ed76705cbc45f2c42169a09e2503`:

```go
824:	for line := range strings.Lines(strings.TrimSpace(string(output))) {
...
828:		if line == "" {
...
842:		info, err := os.Lstat(line)
843:		if err != nil {
844:			continue
```

Every large file except the last one is dropped from the report. The Lstat error is swallowed by `continue`.

The checks in go vet, staticcheck 2026.2.1 and golangci-lint 2.14.0 (every linter enabled) were run on two small programs that contain this bug and the `line == "beta"` bug. None of them says anything about it.

Measured: `go vet ./... ; staticcheck ./... ; golangci-lint run ./... ; lineslint ./...` in a module holding those two programs.

```
go vet:          no output, exit 0
staticcheck:     no output, exit 0
golangci-lint:   13 findings, all style (errcheck, forbidigo, noinlineerr, revive package-comments, varnamelen, wsl, wsl_v5); none about the newline
lineslint:       3 findings, exit 1
  lines/main.go:11:6: line still ends in "\n" (Lines keeps line terminators), so it never equals "beta" (trim it first)
  lines/main.go:17:3: line still ends in "\n" (Lines keeps line terminators), so this map key differs from the same text without the terminator (trim it first)
  nl/main.go:15:16: line still ends in "\n" (Lines keeps line terminators), so os.Lstat cannot use it as a file path (trim it first)
```

The programs print `files found: 1 of 3` and `false 3 "alpha\nbeta\ngamma\n"` when run.

## Installation

```sh
go install github.com/quietpelican42/lineslint/cmd/lineslint@latest
```

The module depends on `golang.org/x/tools` only.

## Usage

### Command line

```sh
lineslint ./...
```

The command accepts the flags of any go/analysis `singlechecker` (`-json`, `-test=false`, `-c=N`, `-fix` has no effect because no fixes are offered).

### go vet

```sh
go vet -vettool=$(which lineslint) ./...
```

In this mode `go vet` decides the exit status.

### As a library

```go
import "github.com/quietpelican42/lineslint"

// Add lineslint.Analyzer to a multichecker or an analysistest run.
var _ = lineslint.Analyzer
```

## Reference

### Sinks

A finding is reported when a tracked value reaches one of these without a cleaning step.

| Sink | Reported when |
| --- | --- |
| `x == "lit"`, `x != "lit"`, `strings.EqualFold`, `bytes.Equal` | the constant does not end in `"\n"` |
| `x == ""` | `x` is the unmodified line (a line is never empty) |
| `switch x { case "lit": }` | a case constant does not end in `"\n"` |
| `strings.HasSuffix(x, lit)`, `bytes.HasSuffix` | `lit` is non-empty and does not end in `"\n"` |
| map index `m[x]`, `m[x] = v`, `delete(m, x)`, `map[string]T{x: v}` | always |
| `os.Stat`, `Lstat`, `Open`, `OpenFile`, `ReadFile`, `WriteFile`, `Create`, `ReadDir`, `Remove`, `RemoveAll`, `Mkdir`, `MkdirAll`, `Chdir`, `Chmod`, `Readlink`, `Truncate`, `Rename`, `Symlink` | the path argument (both for `Rename` and `Symlink`) |
| `filepath.Join`, `Base`, `Dir`, `Ext`, `Abs`, `Clean`, `Rel`, `Glob`, `EvalSymlinks`, `Walk`, `WalkDir` | any argument |
| `exec.Command`, `exec.CommandContext`, `exec.LookPath` | any name or argument |
| `strconv.Atoi`, `ParseInt`, `ParseUint`, `ParseFloat`, `ParseBool` | the input |
| `net.ParseIP`, `net.ParseCIDR`, `netip.ParseAddr`, `url.Parse`, `url.ParseRequestURI` | the input |

Every sink has a runtime test in `groundtruth_test.go` that runs the real standard library and shows the misbehavior, and shows that the trimmed line behaves.

### Cleaning steps

These clear the value. The analyzer follows only the operations listed under "Keeps the newline"; every other expression is treated as clean.

| Clears the newline | Keeps the newline |
| --- | --- |
| `strings.TrimSpace`, `TrimFunc`, `TrimRightFunc` | assignment, `string(x)`, `[]byte(x)` |
| `TrimSuffix(x, s)` where `s` is not a constant, or ends in `"\n"` | `ToLower`, `ToUpper`, `ToTitle`, `Clone` |
| `TrimRight(x, c)`, `Trim(x, c)` where `c` is not a constant, or contains `"\n"` | `TrimPrefix`, `TrimLeft`, `TrimLeftFunc` |
| `CutSuffix`, `Fields`, `FieldsFunc`, `Split`, `SplitN` | `TrimSuffix(x, ".go")`, `TrimRight(x, " ")` |
| `x[:n]` (any explicit upper bound, for example after a `HasSuffix(x, "\n")` check) | `x[n:]`, `prefix + x`, `s += x` |
| `bufio.Scanner` over `strings.NewReader(x)`, any call not listed | the `after` value of `Cut(x, sep)` and `CutPrefix(x, p)` with a constant that has no `"\n"` |

The same applies to the `bytes` package functions.

### Command exit codes

| Code | Meaning |
| --- | --- |
| 0 | No findings. |
| 1 | At least one finding. |
| 2 | The packages could not be loaded or type-checked, or the command line was invalid. |

The `singlechecker` driver exits 3 on findings and 1 on load errors, and exits 0 in `-json` mode even with findings. The command runs the driver in a child process and remaps: 3 becomes 1, everything else non-zero becomes 2, and in `-json` mode the output is checked for diagnostics. Measured with the unmodified driver: clean 0, findings 3, type error 1, missing package 1, no arguments 1, unknown flag 2, `-json` with findings 0.

## How it works

For each `for x := range strings.Lines(...)` (or `bytes.Lines`, a stored `iter.Seq` from either, or `slices.Collect` of one), the analyzer walks the loop body in source order with a set of variables that still carry the newline.

- An assignment adds the target when the right side is a tracked value passed through one of the newline-preserving operations, and removes it otherwise.
- Branches merge by intersection. A variable is reported only when it carries the newline on every path that reaches the sink.
- A function literal inherits only variables that are never reassigned. Variables whose address is taken, or that a closure assigns, are not tracked.
- After a loop or a switch, every variable assigned inside it is dropped.
- Expressions it cannot classify are clean. Unknown calls, struct fields and helper functions never produce a finding.

## Limitations

- The analysis is intraprocedural. A line passed to a helper function is not followed, and a helper that returns a line is not a source.
- Elements of `strings.Split(x, sep)` are not tracked. The last element of `Split(line, " ")` keeps the newline, and this is not reported. `Fields` elements are clean, and `Split(line, "\n")` elements are clean.
- Lines stored in slices, maps, channels or struct fields are not tracked, and neither are loops over a `slices.Collect` result assigned more than once.
- `bufio.Reader.ReadString('\n')` and `ReadBytes` have the same property and are out of scope.
- `TrimSuffix(x, ".go")` keeps the newline and is reported, but `TrimSuffix(x, suffixVar)` is assumed to clean.
- The last line of an input that has no final newline does match. Reported code is wrong for every other line, and matches only for the last.
- The map-key sink reports code that keeps the terminator inside the key consistently. Such code works except for the unterminated last line and when the keys are later printed or compared with clean text.
- `x == ""` is reported only on the untouched line.
- There is no suppression comment.
- Only the sinks listed above are checked.

## Compatibility

Run here: Go 1.27.1 on darwin/arm64 with `golang.org/x/tools` v0.50.0. The module requires Go 1.26, the version that `golang.org/x/tools` v0.50.0 requires. The fixtures use range-over-func, so the analyzed code needs Go 1.23 or later, and `strings.Lines` needs 1.24. Linux and Windows are not run. The CI workflow runs the checks on ubuntu-latest.

The pinned-checkout and real-repository checks below need network access and run only when requested.

## Development

```sh
go build ./...
go vet ./...
go test -race -count=1 ./...
gofmt -l .
```

`mole_test.go` checks the real bug. It skips unless `LINESLINT_MOLE_DIR` points at a checkout of commit `f32fa0c6b082ed76705cbc45f2c42169a09e2503`, and it verifies the SHA-256 of `cmd/analyze/scanner.go` before it runs. The fetch commands are in the file.

### Real-repository check

The analyzer was run over 50 public repositories that call `strings.Lines` or `bytes.Lines`, each shallow-cloned at a pinned commit (found with `gh search code`). 42 type-checked far enough to analyze (two of them, cerbos and birdnet-go, only in part). That covers 117 `range` loops over `Lines` in 40 repositories, plus 2 in tw93/Mole and the ones in the birdnet-go packages that loaded. The other 8 were skipped because they need cgo, another platform, a newer Go than 1.27.1, or modules that could not be fetched.

| Repository | Commit | Findings |
| --- | --- | --- |
| tw93/Mole | `f32fa0c6b082ed76705cbc45f2c42169a09e2503` | 3 in `cmd/analyze/scanner.go`: `line == ""` (line 828), `os.Lstat(line)` (842), `filepath.Base(line)` (854). All real. |
| tphakala/birdnet-go | `5dc2ab8813fda60da02315fc15a1076834695ab1` | 1 in `internal/audiocore/ffmpeg/validate.go:499`: `line == ""` on a raw line. Real but harmless: the following parser ignores a `"\n"` line. Only some of the module's packages type-checked. |
| 40 other repositories | pinned per repository | 0 |

All 4 findings were read by hand. None is a false positive. Reading the 32 loops in the analyzed checkouts that contain no obvious cleaning call turned up no missed sink from the list above. Several pass the line to a helper such as `parseLine(string(line))`, which the analyzer does not follow.

## License

MIT. See [LICENSE](LICENSE).
