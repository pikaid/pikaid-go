// Package pikaid implements the pikaid identifier format, specification
// revision 1.0.4.
//
// A pikaid is exactly 26 lowercase ASCII Base36 characters:
//
//	[0..6]   7-character unsigned Unix timestamp in whole seconds (UTC)
//	[7..25]  19-character unsigned 96-bit cryptographic randomness
//
// The equivalent binary representation is exactly 17 bytes:
//
//	[0..4]   5-byte unsigned big-endian timestamp
//	[5..16]  12 randomness bytes, unchanged
//
// Both fields are fixed-width and left-padded with '0'. The format is
// immutable and unversioned; no version byte or separator is used.
//
// Identifiers with different timestamps sort chronologically under ordinary
// ASCII lexicographic or unsigned bytewise comparison. Identifiers generated
// within the same second sort by their random suffix, not by generation
// order: pikaid is not a monotonic sequence, has one-second precision, and is
// not an authentication token or a secret.
package pikaid

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"time"
)

// Format dimensions. The identifier format is immutable; these values are
// part of specification revision 1.0.4.
const (
	// StringLength is the number of characters in the canonical string form.
	StringLength = 26

	// BinaryLength is the number of bytes in the binary representation.
	BinaryLength = 17

	// TimestampLength is the number of Base36 characters in the timestamp
	// prefix.
	TimestampLength = 7

	// RandomnessLength is the number of Base36 characters in the randomness
	// suffix.
	RandomnessLength = 19

	// RandomnessBytes is the number of randomness bytes in both the binary
	// representation and the generation process.
	RandomnessBytes = 12

	// TimestampBytes is the number of timestamp bytes in the binary form.
	TimestampBytes = 5

	// MaxTimestamp is the largest valid timestamp, 36^7-1 (78364164095
	// seconds after the Unix epoch, in the year 4453). Five timestamp bytes
	// can hold larger values, but they are not valid pikaid timestamps.
	MaxTimestamp = 78_364_164_095
)

// ID is a pikaid value. Its zero value is valid and represents timestamp 0
// with all-zero randomness, i.e. "00000000000000000000000000".
//
// ID is a comparable value type: two IDs are equal if and only if they
// represent the same timestamp and randomness. Use Compare for ordering.
type ID struct {
	timestamp  uint64
	randomness [RandomnessBytes]byte
}

// newID assembles an ID from an already validated timestamp and randomness.
func newID(timestamp uint64, randomness [RandomnessBytes]byte) ID {
	return ID{timestamp: timestamp, randomness: randomness}
}

// Parse validates s and returns the identifier it represents.
//
// It requires exactly StringLength bytes, only lowercase Base36 characters,
// and a randomness suffix no greater than 2^96-1. Uppercase letters,
// separators, prefixes, invalid characters, wrong lengths and oversized
// suffixes are rejected rather than normalized or truncated. Non-ASCII input
// is rejected because no byte outside [0-9a-z] is accepted.
func Parse(s string) (ID, error) {
	if len(s) != StringLength {
		return ID{}, fmt.Errorf("%w: got %d bytes, want %d", ErrInvalidLength, len(s), StringLength)
	}
	for i := 0; i < StringLength; i++ {
		if !isBase36(s[i]) {
			return ID{}, fmt.Errorf("%w: %q at offset %d", ErrInvalidCharacter, s[i], i)
		}
	}

	timestamp := decodeTimestamp(s[:TimestampLength])
	randomness, err := decodeRandomness(s[TimestampLength:])
	if err != nil {
		return ID{}, err
	}
	return newID(timestamp, randomness), nil
}

// IsValid reports whether s is a fully valid pikaid, applying the same
// semantic checks as Parse.
func IsValid(s string) bool {
	if !IsWellFormed(s) {
		return false
	}
	_, err := decodeRandomness(s[TimestampLength:])
	return err == nil
}

// IsWellFormed reports whether s has the correct length and alphabet but does
// not check whether the randomness suffix fits in 96 bits. It is a fast shape
// check only; a true result does not mean s is a valid pikaid. Use IsValid or
// Parse for full validation.
func IsWellFormed(s string) bool {
	if len(s) != StringLength {
		return false
	}
	for i := 0; i < StringLength; i++ {
		if !isBase36(s[i]) {
			return false
		}
	}
	return true
}

// FromBinary decodes a 17-byte binary pikaid. It rejects buffers of any other
// length and buffers whose timestamp exceeds MaxTimestamp.
func FromBinary(b []byte) (ID, error) {
	if len(b) != BinaryLength {
		return ID{}, fmt.Errorf("%w: got %d bytes, want %d", ErrInvalidLength, len(b), BinaryLength)
	}

	timestamp := uint64(b[0])<<32 | uint64(b[1])<<24 | uint64(b[2])<<16 | uint64(b[3])<<8 | uint64(b[4])
	if timestamp > MaxTimestamp {
		return ID{}, fmt.Errorf("%w: %d exceeds %d", ErrTimestampOutOfRange, timestamp, int64(MaxTimestamp))
	}

	var randomness [RandomnessBytes]byte
	copy(randomness[:], b[TimestampBytes:])
	return newID(timestamp, randomness), nil
}

// String returns the 26-character canonical form of id.
func (id ID) String() string {
	timestamp := encodeTimestamp(id.timestamp)
	randomness := encodeRandomness(id.randomness)

	var buf [StringLength]byte
	copy(buf[:TimestampLength], timestamp[:])
	copy(buf[TimestampLength:], randomness[:])
	return string(buf[:])
}

// Bytes returns the 17-byte binary representation of id.
func (id ID) Bytes() [BinaryLength]byte {
	var buf [BinaryLength]byte
	buf[0] = byte(id.timestamp >> 32)
	buf[1] = byte(id.timestamp >> 24)
	buf[2] = byte(id.timestamp >> 16)
	buf[3] = byte(id.timestamp >> 8)
	buf[4] = byte(id.timestamp)

	copy(buf[TimestampBytes:], id.randomness[:])
	return buf
}

// Timestamp returns the Unix timestamp in whole seconds. It is always in
// 0..MaxTimestamp.
func (id ID) Timestamp() int64 {
	return int64(id.timestamp)
}

// TimestampTime returns the timestamp as a time.Time in UTC. The location is
// always time.UTC; pikaid timestamps have one-second precision.
func (id ID) TimestampTime() time.Time {
	return time.Unix(int64(id.timestamp), 0).UTC()
}

// Randomness returns a copy of the 12 randomness bytes. The name follows the
// specification; these bytes are the "entropy" of the identifier.
func (id ID) Randomness() [RandomnessBytes]byte {
	return id.randomness
}

// RandomnessHex returns the 12 randomness bytes as 24 lowercase hexadecimal
// characters.
func (id ID) RandomnessHex() string {
	return hex.EncodeToString(id.randomness[:])
}

// Compare returns -1, 0 or 1 as id sorts before, equal to, or after other.
//
// Different timestamps sort chronologically. Identifiers with the same
// timestamp sort by their random suffix, which is not generation order. The
// result is identical to lexicographic comparison of String values and to
// unsigned bytewise comparison of Bytes values.
func (id ID) Compare(other ID) int {
	if id.timestamp != other.timestamp {
		if id.timestamp < other.timestamp {
			return -1
		}
		return 1
	}
	return bytes.Compare(id.randomness[:], other.randomness[:])
}

// MarshalText implements encoding.TextMarshaler. The output is the canonical
// 26-character string.
func (id ID) MarshalText() ([]byte, error) {
	return []byte(id.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler using the strict rules of
// Parse. The receiver is left unchanged when parsing fails.
func (id *ID) UnmarshalText(text []byte) error {
	parsed, err := Parse(string(text))
	if err != nil {
		return err
	}
	*id = parsed
	return nil
}

// MarshalBinary implements encoding.BinaryMarshaler. The output is the
// 17-byte binary representation.
func (id ID) MarshalBinary() ([]byte, error) {
	buf := id.Bytes()
	return buf[:], nil
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler using the strict rules
// of FromBinary. The receiver is left unchanged when decoding fails.
func (id *ID) UnmarshalBinary(data []byte) error {
	parsed, err := FromBinary(data)
	if err != nil {
		return err
	}
	*id = parsed
	return nil
}

// randomnessOverflowError annotates an oversized randomness suffix.
func randomnessOverflowError(s string) error {
	return fmt.Errorf("%w: suffix %q exceeds 2^96-1", ErrRandomnessOverflow, s)
}
