# Changelog

All notable changes to this module are documented here. The module follows
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## v1.0.0 — 2026-11-07

First stable release. Implements pikaid specification revision 1.0.4.

### Added

- `ID` value type with strict `Parse`, `IsValid`, `IsWellFormed` and
  `FromBinary` entry points.
- Generation from `crypto/rand`: `Generate`, `GenerateWithTimestamp`,
  `GenerateBinary` and `GenerateBinaryWithTimestamp`.
- `String`, `Bytes`, `Timestamp`, `TimestampTime`, `Randomness`,
  `RandomnessHex` and `Compare`.
- `encoding.TextMarshaler`, `encoding.TextUnmarshaler`,
  `encoding.BinaryMarshaler` and `encoding.BinaryUnmarshaler` support,
  including canonical JSON encoding.
- Sentinel errors for classification with `errors.Is`/`errors.As`:
  `ErrInvalidLength`, `ErrInvalidCharacter`, `ErrTimestampOutOfRange`,
  `ErrRandomnessOverflow` and `ErrRandomSource`.
- Official interoperability vectors, boundary and overflow tests, an
  independent `math/big` oracle, round-trip properties and fuzz targets.

### Security

- Exactly 12 bytes are read from the operating system CSPRNG for every
  generated identifier. Entropy failure fails closed with an error and no
  identifier.
- Validation is strict: no trimming, case folding, coercion or truncation, and
  randomness suffixes above 2^96-1 are rejected.
- The production entropy source is not replaceable package state; deterministic
  test injection stays private to the package tests.
