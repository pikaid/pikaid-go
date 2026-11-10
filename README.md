<p align="center">
  <a href="https://github.com/pikaid/pikaid-specs">
    <img src="https://raw.githubusercontent.com/pikaid/pikaid-specs/v1.0.4/logo.png" alt="pikaid" width="300">
  </a>
</p>

<p align="center">
  <strong>Small · Sortable · Secure ID generator</strong>
</p>

<div align="center">

[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/language-Go-00ADD8.svg)](https://github.com/pikaid/pikaid-go)
[![CI](https://github.com/pikaid/pikaid-go/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/pikaid/pikaid-go/actions/workflows/ci.yml)
[![Official implementation](https://img.shields.io/badge/official-implementation-2e7d32.svg)](https://github.com/pikaid/pikaid-specs)
[![Specification 1.0.4](https://img.shields.io/badge/spec-1.0.4-4b56a6.svg)](https://github.com/pikaid/pikaid-specs/tree/v1.0.4)

</div>

**pikaid** is the **official Go implementation** of a compact, chronologically sortable, collision-resistant identifier format for databases, APIs, and distributed systems.

This is the immutable, unversioned [pikaid format](https://github.com/pikaid/pikaid-specs/tree/v1.0.4), documented in specification revision **1.0.4**.

## ✨ Why pikaid?

- **Short:** Compact, readable IDs for URLs, logs, and API resources.
- **Sortable:** Lexicographically ordered by timestamp, with **one-second precision**.
- **Collision-resistant:** 96 bits of cryptographic randomness make collisions extremely unlikely, never impossible.
- **Storage-friendly:** Use `BINARY(17)` or readable `CHAR(26)`.
- **Distributed-friendly:** No central sequence generator or coordination required.

## 🧬 Identifier format

A pikaid contains **26 lowercase Base36 characters**: a **7-character Unix-seconds timestamp** followed by **19 characters encoding 96 bits of cryptographic randomness**. Its binary form is **17 bytes**: 5 unsigned big-endian timestamp bytes followed by 12 randomness bytes.

![Identifier structure](https://raw.githubusercontent.com/pikaid/pikaid-specs/v1.0.4/structure.svg)

> **Ordering:** Different seconds sort chronologically. Within the same second, randomness—not issuance order—decides the order.

> **Security:** These are identifiers, **not secrets**, session credentials, authentication tokens, or API keys.

## ⚙️ Requirements

Go 1.21 or newer.

## 📦 Installation

From a new Go module (the public Git repository is directly consumable; no `GOPRIVATE` setting is needed):

```sh
mkdir pikaid-demo
cd pikaid-demo
go mod init example.com/pikaid-demo
go get github.com/pikaid/pikaid-go@main
```

Save the quick start below as `main.go`, then run `go run .`. Go 1.21 or newer.

## 🚀 Basic Usage

```go
package main

import (
	"fmt"
	"log"

	"github.com/pikaid/pikaid-go"
)

func main() {
	id, err := pikaid.Generate()
	if err != nil {
		log.Fatal(err) // cryptographic failure: fail closed, no ID was produced
	}

	fmt.Println(id)                  // 26-character ID, values vary
	fmt.Println(id.Timestamp())      // Unix seconds at generation time
	fmt.Println(id.RandomnessHex())  // 24 hex characters
	fmt.Println(pikaid.IsValid(id.String()))
}
```

## 💡 Example Use Case

Use pikaid to identify **users, orders, invoices, uploads, events, and API resources**. Independent services can generate compact IDs without a shared counter, while timestamp-based ordering helps with indexing, sorting, and logs.

## 📚 Canonical specification

- [Canonical specification](https://github.com/pikaid/pikaid-specs)
- [Revision 1.0.4](https://github.com/pikaid/pikaid-specs/tree/v1.0.4)

Specification revisions describe the format; they do not change the encoded identifiers.

## 🧩 API Reference

```go
const (
	StringLength     = 26
	BinaryLength     = 17
	TimestampLength  = 7
	RandomnessLength = 19
	TimestampBytes   = 5
	RandomnessBytes  = 12
	MaxTimestamp     = 78_364_164_095 // 36^7-1, year 4453
)

type ID struct{ /* comparable value type; zero value is valid */ }

func Generate() (ID, error)
func GenerateWithTimestamp(timestamp int64) (ID, error)
func GenerateBinary() ([BinaryLength]byte, error)
func GenerateBinaryWithTimestamp(timestamp int64) ([BinaryLength]byte, error)

func Parse(s string) (ID, error)
func IsValid(s string) bool
func IsWellFormed(s string) bool
func FromBinary(b []byte) (ID, error)

func (id ID) String() string
func (id ID) Bytes() [BinaryLength]byte
func (id ID) Timestamp() int64
func (id ID) TimestampTime() time.Time
func (id ID) Randomness() [RandomnessBytes]byte
func (id ID) RandomnessHex() string
func (id ID) Compare(other ID) int
func (id ID) MarshalText() ([]byte, error)
func (id *ID) UnmarshalText(text []byte) error
func (id ID) MarshalBinary() ([]byte, error)
func (id *ID) UnmarshalBinary(data []byte) error
```

Sentinel errors for `errors.Is`:

| Error | Meaning |
| --- | --- |
| `ErrInvalidLength` | Input is not 26 string bytes or 17 binary bytes |
| `ErrInvalidCharacter` | A byte outside `[0-9a-z]`, including uppercase |
| `ErrTimestampOutOfRange` | Timestamp outside `0..MaxTimestamp` |
| `ErrRandomnessOverflow` | Randomness suffix exceeds 2^96-1 |
| `ErrRandomSource` | The cryptographic random source failed |

The zero `ID` is valid and equals `"00000000000000000000000000"` (timestamp 0,
all-zero randomness). `Randomness` returns a copy; mutating it cannot change the
identifier.

### Parse and validate

```go
id, err := pikaid.Parse("0s44we8002loz3po6mcf6vsn23")
if err != nil {
	// wrapped sentinel: errors.Is(err, pikaid.ErrInvalidCharacter) and friends
	return err
}

fmt.Println(id.TimestampTime().Format(time.RFC3339)) // 2023-11-14T22:13:20Z
```

`Parse` is strict. It accepts only 26 lowercase Base36 bytes and rejects wrong
lengths, uppercase letters, separators, prefixes, non-ASCII input, and randomness
suffixes larger than 2^96-1. Nothing is trimmed, normalized or truncated.

`IsWellFormed` is the fast shape check (length and alphabet only).
`IsValid` adds the 96-bit randomness range check and is therefore the boolean
equivalent of `Parse`. A 19-character suffix has more than 96 bits of range, so
`IsWellFormed` can return true for input that `IsValid` and `Parse` reject.

### Binary form

```go
bin := id.Bytes()        // [17]byte, value type, no allocation

stored := bin[:]         // e.g. for a BINARY(17) / BYTEA column

restored, err := pikaid.FromBinary(stored)
if err != nil {
	return err
}
fmt.Println(restored == id) // true
```

`ID` implements `encoding.TextMarshaler`, `encoding.TextUnmarshaler`,
`encoding.BinaryMarshaler` and `encoding.BinaryUnmarshaler`. `encoding/json`
therefore encodes an `ID` as the canonical 26-character JSON string and decodes
it strictly:

```go
data, err := json.Marshal(id) // `"0s44we8002loz3po6mcf6vsn23"`
```

`database/sql` is not covered by those interfaces. `pikaid.ID` is a struct and
does not implement `driver.Valuer`, so `database/sql`'s default parameter
converter rejects it. Pass `id.String()` for a text column, `id.Bytes()` for a
`BINARY(17)`/`BYTEA` column, or wrap the identifier in an application-specific
type that implements `driver.Valuer`.

### Generation

| Function | Returns |
| --- | --- |
| `Generate()` | `(ID, error)` for the current Unix second |
| `GenerateWithTimestamp(ts int64)` | `(ID, error)` with an explicit timestamp |
| `GenerateBinary()` | `([17]byte, error)` for the current Unix second |
| `GenerateBinaryWithTimestamp(ts int64)` | `([17]byte, error)` with an explicit timestamp |

All four read exactly 12 bytes from `crypto/rand` (the operating system CSPRNG).
`GenerateWithTimestamp` fixes only the timestamp; randomness is never derived
from the timestamp, and output is not deterministic. A timestamp outside
`0..pikaid.MaxTimestamp` returns an error wrapping `ErrTimestampOutOfRange`.
If the random source fails or returns fewer than 12 bytes, generation fails
closed: the returned value is zero and the error wraps `ErrRandomSource`.

### Ordering

```go
if a.Compare(b) < 0 {
	// a sorts before b
}
```

Comparison order is identical to:

- ASCII lexicographic comparison of `String()` values,
- unsigned bytewise comparison of `Bytes()` values,
- `(timestamp, randomness)` tuple order.

Identifiers with different timestamps sort chronologically. **Identifiers
created within the same second are ordered by their random suffix, not by
generation time** — there is no monotonic counter and no sub-second component.

## 🔄 Order Guarantee

Text and 17-byte binary values preserve the canonical timestamp-then-randomness ordering. Same-second values are **not** ordered by issuance.

## 🛢 Storage Recommendations

| Representation | Size | Recommended for |
| --- | ---: | --- |
| **BINARY(17)** | 17 bytes | Compact primary keys and indexes |
| **CHAR(26)** | 26 characters | Readable SQL, debugging, URLs, and logs |

Use an **ASCII bytewise/binary collation** for text columns (for example, `ascii_bin` on MariaDB/MySQL). Case-insensitive or locale-aware sorting is not a substitute for canonical bytewise comparison.

## ✅ Verification and release status

The [CI workflow](https://github.com/pikaid/pikaid-go/actions/workflows/ci.yml) runs native tests, pinned automated conformance checks, and language-specific quality gates. To run the native checks locally from a checkout: `go test ./...`.

**Automated passing tests are not a security certification.** The independent CSPRNG source and failure-path audits (`P-GENERATE-RNG-SOURCE` and `P-GENERATE-RNG-FAILURE`) still require separate human evidence.

<details>
<summary>Platform, tooling, and conformance details</summary>

Requires Go 1.21 or newer. CI runs the tests on Linux, macOS and Windows with
Go 1.21.x and the current stable Go release, together with the race detector, a
coverage gate, 32-bit (`GOARCH=386`) tests, cross-builds, and the pinned
official conformance suite described below.

```sh
go build ./...
go vet ./...
gofmt -l .
go test ./...
go test -race ./...
go test -cover ./...
go test -run=^$ -fuzz=FuzzParse -fuzztime=10s
```

The test suite covers every official PHP v1.2.0 interoperability vector
(vendored in `testdata/`), independently computed known answers, `math/big`
oracle comparisons, malformed and overflow inputs, integer and entropy
boundaries, zero padding, property-based round trips, cross-representation
ordering, generation including pre-entropy timestamp validation and CSPRNG
failure, the text/binary/JSON codecs, plus fuzz targets for `Parse` and
`FromBinary`.

### Official conformance

The repository runs the official cross-language PikaID conformance suite from
[pikaid/pikaid-specs](https://github.com/pikaid/pikaid-specs) pinned at commit
`059780cc0e5b113443812720d95c630649bf3aa3` (package revision 1.0.0, spec
revision 1.0.4). The suite covers 1,100 fixture rows across six check
families and is wired into the required `official conformance` CI job.

`cmd/conformance-adapter` is a test-only JSON-Lines bridge. It delegates every
operation to the exported `pikaid` API and never looks up fixtures, so a defect
in the library cannot be hidden by the adapter. It advertises the optional
capabilities the library honestly implements (`boolean_validation`,
`format_only_validation`, `parse_components`, `parse_datetime`,
`generate_with_timestamp`, `generate_binary`,
`generate_binary_with_timestamp`, `public_compare`) and deliberately not
`csprng_failure_injection`: the library exposes no safe hook for injecting a
production entropy failure.

Run the whole pinned suite, including the fixture self-check, the runner unit
tests, the independent Node oracle, the adapter run and the report
assertions:

```sh
./scripts/verify-official-conformance.sh
```

Local prerequisites are Go, Node.js, and Python 3.11+ (the script creates its
own virtualenv and clones the pinned suite into a scratch directory; nothing is
written into the repository). Useful overrides:

| Variable | Purpose |
| --- | --- |
| `PIKAID_SPECS_DIR` | Use an existing checkout instead of cloning |
| `PIKAID_CONFORMANCE_VENV` | Reuse an existing virtualenv |
| `PIKAID_CONFORMANCE_WORKDIR` | Keep the scratch directory |
| `PIKAID_CONFORMANCE_REPORT_JSON` | Copy the JSON report to a path |
| `PYTHON`, `GO` | Choose the interpreters/toolchains |

Two property checks are reported as not applicable, and the runner therefore
exits 3 by default (the script treats 3 as the expected value and fails on
anything else):

- `P-GENERATE-RNG-SOURCE` is a manual CSPRNG source audit; automated tooling
  cannot supply it.
- `P-GENERATE-RNG-FAILURE` exercises entropy-failure injection, which this
  library does not advertise as a capability.

Both remain `MANUAL_EVIDENCE_REQUIRED`. A passing automated run is **not** a
conformance certification.

</details>

Registry publication is not implied by a green CI: use the installation method documented above.

## 🤝 Contributing

Contributions are always welcome! 💙

Whether you're fixing a typo, improving documentation, adding tests, or making pikaid even better, every contribution is appreciated.

**Want to help? Here's how:**

- 🐛 **Found a bug?** [Open an issue](https://github.com/pikaid/pikaid-go/issues) with clear reproduction steps.
- 💡 **Have an idea?** Start with an issue before making major changes.
- 🛠️ **Ready to contribute?** Open a pull request explaining your changes.
- 🧪 **Keep it tested.** Add meaningful regression tests for behavior changes and follow the existing conventions.
- ✅ **Make sure CI passes.** All required checks must be green before a pull request is merged. No shortcuts!

**One important rule:** pikaid's identifier format is immutable. Contributions must preserve the [canonical specification (revision 1.0.4)](https://github.com/pikaid/pikaid-specs/tree/v1.0.4).

🔐 **Found a security vulnerability?** Report it privately using GitHub's security reporting (when available), or [contact the maintainer](mailto:larananas.dev@proton.me)—not through a public issue.

**Every improvement matters, big or small. Thanks for helping make pikaid better!** 🐇

## 📄 License

The implementation is available under the [MIT License](LICENSE).

## ✉️ Contact

For licensing or project-related questions: [larananas.dev@proton.me](mailto:larananas.dev@proton.me).

