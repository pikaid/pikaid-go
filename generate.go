package pikaid

import (
	"crypto/rand"
	"fmt"
	"io"
	"time"
)

// randomReader is the cryptographic random source used for generation. It is
// a package variable only so tests can exercise the failure path; production
// code always reads from crypto/rand.Reader, which is the operating system
// CSPRNG.
var randomReader io.Reader = rand.Reader

// Generate returns a new identifier for the current UTC Unix second.
//
// Exactly 12 bytes are read from the operating system CSPRNG. If the random
// source fails, Generate returns an error and no identifier. Generate does not
// guarantee uniqueness: applications that require it should enforce a
// primary key or unique constraint.
func Generate() (ID, error) {
	return GenerateWithTimestamp(time.Now().Unix())
}

// GenerateWithTimestamp returns a new identifier with the supplied Unix
// timestamp in whole seconds. The timestamp must be in 0..MaxTimestamp.
//
// Randomness always comes from the operating system CSPRNG; the timestamp is
// the only caller-controlled component. Using a caller-supplied timestamp does
// not provide deterministic output.
func GenerateWithTimestamp(timestamp int64) (ID, error) {
	if timestamp < 0 || timestamp > MaxTimestamp {
		return ID{}, fmt.Errorf("%w: %d is outside 0..%d", ErrTimestampOutOfRange, timestamp, int64(MaxTimestamp))
	}

	id := ID{timestamp: uint64(timestamp)}
	if _, err := io.ReadFull(randomReader, id.randomness[:]); err != nil {
		return ID{}, fmt.Errorf("%w: %v", ErrRandomSource, err)
	}
	return id, nil
}

// GenerateBinary returns a new identifier for the current UTC Unix second in
// its 17-byte binary representation.
func GenerateBinary() ([BinaryLength]byte, error) {
	id, err := Generate()
	if err != nil {
		return [BinaryLength]byte{}, err
	}
	return id.Bytes(), nil
}

// GenerateBinaryWithTimestamp returns a new identifier with the supplied Unix
// timestamp in its 17-byte binary representation. The timestamp must be in
// 0..MaxTimestamp.
func GenerateBinaryWithTimestamp(timestamp int64) ([BinaryLength]byte, error) {
	id, err := GenerateWithTimestamp(timestamp)
	if err != nil {
		return [BinaryLength]byte{}, err
	}
	return id.Bytes(), nil
}
