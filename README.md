<p align="center">
  <img src="https://raw.githubusercontent.com/pikaid/pikaid-specs/v1.0.4/logo.png" alt="pikaid" width="300" />
</p>

<p align="center">
  <strong>Small · Sortable · Secure ID generator</strong>
</p>

# pikaid — Official Go Implementation

> **Status: unreleased and unpublished.** There is no tagged version, no
> registry or module-proxy publication, and this private repository has no
> public package documentation. Pin a commit (`@<commit>`) for reproducible use.

The official Go implementation of [pikaid](https://github.com/pikaid), a compact,
chronologically sortable identifier for databases, APIs and distributed systems.

A pikaid is exactly **26 lowercase Base36 characters**:

```text
0s44we8 002loz3po6mcf6vsn23
└──┬──┘ └─────────┬────────┘
   7 chars       19 chars
   Unix seconds 96 bits of cryptographic randomness
```

The equivalent binary form is exactly **17 bytes**:

```text
[ 5-byte big-endian timestamp ][ 12-byte randomness ] = 17 bytes
```

The identifier format is **immutable and unversioned**: specification revisions
describe the same format; they never change the wire representation. This module
implements [specification revision 1.0.4](https://github.com/pikaid/pikaid-specs/tree/v1.0.4).

## Installation

```sh
go get github.com/pikaid/pikaid-go@main
```

The module declares Go 1.21 as its minimum supported version and has no
dependencies outside the Go standard library.

## Quick start

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

	fmt.Println(id)                  // 0s44we8002loz3po6mcf6vsn23 (example)
	fmt.Println(id.Timestamp())      // 1700000000
	fmt.Println(id.RandomnessHex())  // 00112233445566778899aabb
	fmt.Println(pikaid.IsValid(id.String()))
}
```

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

## API

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

## Security and limitations

- **Not a secret or an authentication token.** A pikaid is not signed,
  encrypted or MACed, and part of it is predictable (the timestamp).
- **Uniqueness is probabilistic, not guaranteed.** 96 bits of randomness make
  collisions vanishingly unlikely, but applications that require uniqueness
  must still enforce a primary key or unique constraint.
- **No monotonicity.** Two identifiers from the same second are unordered with
  respect to creation time. Do not use pikaid as a sequence or for
  client-conflict resolution that assumes monotonic ordering.
- **One-second precision.** There is no sub-second component and timestamps are
  whole Unix seconds.
- **No timestamp verification.** Parsing accepts any timestamp in
  `0..MaxTimestamp`; it does not judge whether the value is plausible.
- **Fail closed.** A failing CSPRNG produces an error, never a partly random or
  predictable identifier.
- **Clock skew.** `Generate` uses the local system clock; hosts with skewed
  clocks produce skewed timestamps. Synchronize clocks (for example with NTP)
  when chronological grouping matters.

## Development

Requires Go 1.21 or newer. CI runs the tests on Linux, macOS and Windows with
Go 1.21.x and the current stable Go release, together with the race detector, a
coverage gate, 32-bit (`GOARCH=386`) tests, and cross-builds.

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

## Specification and license

- Canonical specification: [pikaid-specs v1.0.4](https://github.com/pikaid/pikaid-specs/tree/v1.0.4)
- Official implementations: [github.com/pikaid](https://github.com/pikaid)

MIT licensed. See [LICENSE](LICENSE).
