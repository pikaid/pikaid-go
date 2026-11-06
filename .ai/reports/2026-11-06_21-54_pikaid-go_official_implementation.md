# pikaid-go Official Implementation

## Summary

Implemented the official Go implementation of the pikaid identifier format in
`/Users/lara/Dev/projects/pikaid-go` as module `github.com/pikaid/pikaid-go`.

The implementation follows pikaid specification revision 1.0.4 (pinned commit
`17014d76f568c3ac9f9ce2927c1a018fdbc36d1d`) and produces the exact 26-character
lowercase Base36 string form and 17-byte binary form. It uses `crypto/rand` for
exactly 12 bytes of randomness, decodes strictly, preserves lossless
string/binary conversion and ordering, exposes an idiomatic value-type API with
sentinel errors, and ships conformance tests, fuzzing, benchmarks, documentation,
an MIT license, and GitHub Actions CI.

No commit, tag, push, release, or publication was performed. All work remains in
the working tree as uncommitted/untracked changes on `main`.

## Requirement Coverage

### REQ-1 — Exact format: 26 lowercase Base36 characters / 17 binary bytes
**PASS.** `pikaid.go` defines `StringLength = 26`, `BinaryLength = 17`,
`TimestampLength = 7`, `RandomnessLength = 19`, `TimestampBytes = 5`,
`RandomnessBytes = 12`, and `MaxTimestamp = 78_364_164_095`. `String()` emits
seven fixed-width timestamp digits plus nineteen fixed-width randomness digits;
`Bytes()` emits five big-endian timestamp bytes plus twelve randomness bytes.
Boundary vectors cover timestamp `0`, `MaxTimestamp`, all-zero randomness, and
`2^96-1` randomness. `vectors_test.go` validates the canonical examples and
official PHP v1.2.0 vectors.

### REQ-2 — CSPRNG generation with exactly 12 random bytes and fail-closed behavior
**PASS.** `generate.go` implements `Generate`, `GenerateWithTimestamp`,
`GenerateBinary`, and `GenerateBinaryWithTimestamp`. All read exactly
`RandomnessBytes` (12) bytes from `crypto/rand.Reader` and never derive
randomness from the timestamp. A short read or reader error returns the zero
value and an error wrapping `ErrRandomSource`; no partially random identifier is
returned. Tests inject a failing/short reader through the unexported
`randomReader` seam and assert the fail-closed contract.

### REQ-3 — Strict decoding without truncation, wrapping, or normalization
**PASS.** `Parse` accepts only exactly 26 ASCII bytes from `[0-9a-z]` and rejects
wrong length, uppercase, separators, prefixes, non-ASCII input, randomness
greater than `2^96-1`, and unrepresentable timestamps. It does not trim or
normalize. `FromBinary` requires exactly 17 bytes and rejects timestamps above
`MaxTimestamp`. Errors wrap sentinel values (`ErrInvalidLength`,
`ErrInvalidCharacter`, `ErrTimestampOutOfRange`, `ErrRandomnessOverflow`) so
`errors.Is` works. Malformed-input and overflow tables cover these cases.

### REQ-4 — Lossless conversions and consistent ordering
**PASS.** String-to-binary and binary-to-string round trips preserve both fields
bit-for-bit; timestamp and randomness accessors are exact. `Compare` implements
unsigned `(timestamp, randomness)` ordering, which tests verify is identical to
ASCII lexicographic `String()` ordering and unsigned bytewise `Bytes()` ordering
across edge values, same-timestamp values, and randomized properties.
`TimestampTime` converts the stored Unix seconds to UTC.

### REQ-5 — Idiomatic, documented public API
**PASS.** `ID` is a comparable value type with unexported fields; its zero value
is valid and renders `"00000000000000000000000000"`. The package exports
generation, parsing, validation, conversion, accessors, comparison,
`encoding.TextMarshaler`/`TextUnmarshaler`, and
`encoding.BinaryMarshaler`/`BinaryUnmarshaler`. Exported identifiers have doc
comments, and four runnable examples are included. The package has no
dependencies outside the Go standard library.

### REQ-6 — Substantive independent conformance tests and ≥90% coverage
**PASS.** The suite contains 28 top-level test functions, 114 executed test and
subtest cases, plus two fuzz targets. It includes official PHP v1.2.0
interoperability vectors (static fixtures), independently derived known answers,
`math/big` oracle comparisons, malformed inputs, integer and entropy boundaries,
zero-padding cases, property-based round trips, cross-representation ordering,
generation failure injection, and fuzz targets for `Parse` and `FromBinary`.
Statement coverage is **99.2%** (threshold 90%). The only uncovered branch is the
unreachable default of `digitValue` after alphabet prevalidation.

### REQ-7 — Packaging, MIT license, and README
**PASS.** `go.mod` declares `module github.com/pikaid/pikaid-go` and `go 1.21`.
`LICENSE` is MIT (`Copyright (c) 2026 larananas (Lara G.)`). `README.md`
documents installation, quick start, parsing, binary form, generation, ordering,
the full API, sentinel errors, security limitations, development commands, and
the specification link. README snippets were compiled and executed in a separate
module; deterministic outputs matched the documented values.

### REQ-8 — GitHub Actions CI
**PASS (configuration); execution pending a push.** `.github/workflows/ci.yml`
defines five jobs: a build/vet/test matrix over Ubuntu, macOS, and Windows with
Go `1.21.x` and `stable`; race detection on Ubuntu; cross-builds for
`linux/386`, `linux/arm64`, `windows/amd64`, `windows/arm64`, and `darwin/arm64`;
a `gofmt` gate; and a coverage job with a hard 90% gate. The YAML parses and the
job set is `test,race,cross-build,format,coverage`. The workflow has not run on
GitHub because nothing was pushed.

### REQ-9 — Minimal scope and truthful limitation disclosure
**PASS.** Changes are confined to the `pikaid/pikaid-go` repository. No other
repositories were modified; no tags, releases, commits, pushes, force-pushes, or
package publications were made. The README explicitly discloses that pikaid is
not a secret or authentication token, uniqueness is probabilistic, there is no
monotonicity within a second, precision is one second, timestamps are not
plausibility-checked, generation fails closed, and clock skew affects output.

## Root Cause or Technical Approach

This was a greenfield implementation rather than a defect fix.

- **Fixed-width Base36.** Timestamp encoding uses seven digits and is inherently
  bounded by `36^7-1`. Randomness encoding uses `math/bits` 96-bit hi/lo
  multiply-add arithmetic and rejects any overflow beyond `2^96-1`, so a
  nineteen-digit suffix cannot silently wrap.
- **Value semantics.** `ID` stores `timestamp uint64` and `randomness [12]byte`.
  It is comparable and copy-safe; accessors return arrays, not aliases into
  mutable internal state.
- **Strict validation ordering.** Length and alphabet checks run before numeric
  decoding; `FromBinary` checks length and timestamp range. Every failure
  returns a zero value plus a wrapped sentinel.
- **Entropy isolation.** Generation reads through an unexported `io.Reader`
  (`randomReader`, defaulting to `crypto/rand.Reader`) so tests can prove
  short-read and failure behavior without weakening production entropy.
- **Ordering equivalence.** Because both fields are fixed-width big-endian
  encodings, tuple order, ASCII string order, and unsigned bytewise binary order
  coincide; the tests assert this rather than assuming it.

A real portability defect was found and fixed during validation: `fmt.Errorf`
formatting of the untyped `MaxTimestamp` constant defaulted to `int` and
overflowed on 32-bit targets. Both production call sites now cast to
`int64(MaxTimestamp)`, and one test-only formatting site was corrected the same
way. `GOOS=linux GOARCH=386 go vet ./...` now passes.

## Files Changed

Created:

- `go.mod`
- `errors.go`
- `base36.go`
- `pikaid.go`
- `generate.go`
- `pikaid_test.go`
- `vectors_test.go`
- `generate_test.go`
- `fuzz_test.go`
- `example_test.go`
- `bench_test.go`
- `testdata/pikaid-v1.json`
- `testdata/README.md`
- `LICENSE`
- `.gitignore`
- `.github/workflows/ci.yml`
- `.ai/reports/2026-11-06_21-54_pikaid-go_official_implementation.md`

Modified:

- `README.md` (replaced the placeholder with full documentation)

Repository state: all files above are untracked except `README.md`, which is
modified but unstaged. Nothing is committed or pushed.

## Validation

Environment: macOS `darwin/arm64`, Go `1.27.3`; compatibility run with
`GOTOOLCHAIN=go1.21.13`. Fixture SHA-256:
`b0c9dfdd063613424414fef5c9065fbd2f81d645d0ab42b34e27cde715613500`.

| Check | Command | Result |
| --- | --- | --- |
| Formatting | `gofmt -l .` | **PASS** (no output) |
| Vet (native) | `go vet ./...` | **PASS** |
| Vet (32-bit) | `GOOS=linux GOARCH=386 go vet ./...` | **PASS** |
| Build | `go build ./...` | **PASS** |
| Tests | `go test ./... -count=1` | **PASS** (114 passed, 0 failed) |
| Race detector | `go test -race ./... -count=1` | **PASS** (114 passed, 0 failed) |
| Coverage | `go test -coverprofile ...` + `go tool cover -func` | **PASS** (99.2% statements) |
| Minimum Go | `GOTOOLCHAIN=go1.21.13 go test ./... -count=1` | **PASS** (114 passed) |
| Examples | `go test -run '^Example' -v` | **PASS** (4 examples) |
| Cross-build | `GOOS/GOARCH` build for windows/amd64, windows/arm64, linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, linux/386 | **PASS** (7/7) |
| Fuzz `FuzzParse` | `-fuzz=FuzzParse -fuzztime=15s` | **PASS** (5,110,973 execs; 10s rerun also PASS) |
| Fuzz `FuzzFromBinary` | `-fuzz=FuzzFromBinary -fuzztime=15s` | **PASS** (5,305,170 execs; 10s rerun also PASS) |
| Benchmark `Parse` | `-bench=BenchmarkParse` | **PASS** (33.23 ns/op, 0 allocs) |
| Benchmark `String` | `-bench=BenchmarkString` | **PASS** (71.12 ns/op, 32 B/op, 1 alloc) |
| Benchmark `Generate` | `-bench=BenchmarkGenerate` | **PASS** (225.0 ns/op, 24 B/op, 1 alloc) |
| CI YAML parse | Ruby `YAML.load_file` | **PASS** (jobs: test, race, cross-build, format, coverage) |
| GitHub Actions execution | Requires push | **NOT RUN** |
| Native Windows/macOS test execution | Requires those hosts or CI | **NOT RUN** (cross-compilation only) |

## Deviations and Unresolved Issues

1. **No independent subagent review.** Both requested review subagents failed
   with `Model unavailable: profile/large` and `Model unavailable: profile/small`.
   Review was therefore performed inline: source, tests, spec rules, README
   claims, 32-bit behavior, and CI configuration were re-read and re-run. No
   second independent reviewer signed off.
2. **Work is uncommitted.** All implementation files are untracked and
   `README.md` is modified on `main`. This is intentional per the no-commit /
   no-push scope, but the repository does not yet contain the work in history.
3. **CI has not executed.** The workflow is syntactically valid and its commands
   were run locally where possible, but the OS/Go matrix and coverage gate have
   not run on GitHub Actions.
4. **Windows/macOS runtime tests not executed locally.** Those targets were
   cross-compiled only. Windows path/line-ending behavior is not exercised.
5. **Coverage is 99.2%, not 100%.** The remaining branch is the unreachable
   default in `digitValue`, which follows an alphabet precheck.
6. **PHP fixtures are static.** Official PHP v1.2.0 vectors were consumed as
   fixtures; PHP itself was not executed to regenerate them.
7. **`go get ...@main` in the README.** No release tag exists, so the README
   documents pinning a commit for reproducibility.
8. **Benchmarks are single-machine.** Results are indicative only (Apple M5,
   Go 1.27.3) and are not a performance guarantee.

## Final Status

task_result: PASS

The implementation satisfies REQ-1 through REQ-9 with local validation:
formatting, vet (including 32-bit), build, 114 tests, race detector, Go 1.21.13
compatibility, 99.2% coverage, seven cross-build targets, two fuzz targets, four
examples, and a parseable CI workflow. Remaining limitations are the unexecuted
GitHub Actions matrix, unexecuted native Windows/macOS tests, the uncommitted
working-tree state, and the absence of an independent subagent review; these are
disclosed above rather than treated as passing evidence.
