package pikaid

import (
	"bytes"
	"crypto/rand"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestGenerate(t *testing.T) {
	before := time.Now().Unix()
	id, err := Generate()
	after := time.Now().Unix()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if id.Timestamp() < before || id.Timestamp() > after {
		t.Errorf("timestamp %d outside observed window %d..%d", id.Timestamp(), before, after)
	}
	text := id.String()
	if len(text) != StringLength {
		t.Fatalf("length = %d, want %d", len(text), StringLength)
	}
	if !IsValid(text) {
		t.Fatalf("generated value %q is not valid", text)
	}
	if id != newID(uint64(id.Timestamp()), id.Randomness()) {
		t.Fatal("generated ID does not round-trip through its fields")
	}
}

func TestGenerateWithTimestampIsExact(t *testing.T) {
	const timestamp = 1_700_000_000
	id, err := GenerateWithTimestamp(timestamp)
	if err != nil {
		t.Fatalf("GenerateWithTimestamp: %v", err)
	}
	if id.Timestamp() != timestamp {
		t.Errorf("timestamp = %d, want %d", id.Timestamp(), timestamp)
	}
	if !strings.HasPrefix(id.String(), "0s44we8") {
		t.Errorf("string prefix = %q, want 0s44we8", id.String()[:7])
	}
	if !IsValid(id.String()) {
		t.Errorf("generated value %q is not valid", id.String())
	}
}

func TestGenerateBinary(t *testing.T) {
	before := time.Now().Unix()
	bin, err := GenerateBinary()
	after := time.Now().Unix()
	if err != nil {
		t.Fatalf("GenerateBinary: %v", err)
	}

	id, err := FromBinary(bin[:])
	if err != nil {
		t.Fatalf("FromBinary(GenerateBinary()): %v", err)
	}
	if id.Timestamp() < before || id.Timestamp() > after {
		t.Errorf("timestamp %d outside observed window %d..%d", id.Timestamp(), before, after)
	}
	if !IsValid(id.String()) {
		t.Errorf("generated binary value decodes to invalid %q", id.String())
	}
}

func TestGenerateBinaryWithTimestampIsExact(t *testing.T) {
	const timestamp = 36 * 36 * 36
	bin, err := GenerateBinaryWithTimestamp(timestamp)
	if err != nil {
		t.Fatalf("GenerateBinaryWithTimestamp: %v", err)
	}
	id, err := FromBinary(bin[:])
	if err != nil {
		t.Fatalf("FromBinary: %v", err)
	}
	if id.Timestamp() != timestamp {
		t.Errorf("timestamp = %d, want %d", id.Timestamp(), timestamp)
	}
	// 46656 == 36^3, so the padded prefix is "0001000".
	if got := id.String()[:TimestampLength]; got != "0001000" {
		t.Errorf("prefix = %q, want %q", got, "0001000")
	}
}

// TestDefaultRandomSourceIsCryptoRand guards against accidentally replacing
// the production entropy source with a non-cryptographic generator.
func TestDefaultRandomSourceIsCryptoRand(t *testing.T) {
	if randomReader != rand.Reader {
		t.Fatal("default randomReader is not crypto/rand.Reader")
	}
}

type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }

func TestGenerateFailsClosedOnRandomSourceError(t *testing.T) {
	tests := []struct {
		name   string
		reader io.Reader
	}{
		{"error", failingReader{err: errors.New("entropy unavailable")}},
		{"truncated", bytes.NewReader(make([]byte, RandomnessBytes-1))},
		{"eof", strings.NewReader("")},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			original := randomReader
			randomReader = tc.reader
			t.Cleanup(func() { randomReader = original })

			id, err := Generate()
			if err == nil {
				t.Fatalf("Generate returned %v without an error", id)
			}
			if !errors.Is(err, ErrRandomSource) {
				t.Errorf("error = %v, want ErrRandomSource", err)
			}
			if id != (ID{}) {
				t.Errorf("Generate returned a partial ID: %v", id)
			}

			bin, err := GenerateBinary()
			if err == nil {
				t.Fatalf("GenerateBinary returned %x without an error", bin)
			}
			if !errors.Is(err, ErrRandomSource) {
				t.Errorf("binary error = %v, want ErrRandomSource", err)
			}
			if bin != ([BinaryLength]byte{}) {
				t.Errorf("GenerateBinary returned partial bytes: %x", bin)
			}

			// A timestamp error is reported before randomness is requested.
			if _, err := GenerateWithTimestamp(-1); !errors.Is(err, ErrTimestampOutOfRange) {
				t.Errorf("timestamp error = %v, want ErrTimestampOutOfRange", err)
			}
		})
	}

	// Restoring the source restores generation.
	id, err := Generate()
	if err != nil {
		t.Fatalf("Generate after restoring source: %v", err)
	}
	if !IsValid(id.String()) {
		t.Fatalf("generated value %q is invalid", id.String())
	}
}

func TestGeneratedIDsAreDistinct(t *testing.T) {
	const count = 20_000
	seen := make(map[ID]struct{}, count)
	for i := 0; i < count; i++ {
		id, err := Generate()
		if err != nil {
			t.Fatalf("Generate #%d: %v", i, err)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate identifier after %d generations: %v", i, id)
		}
		seen[id] = struct{}{}
	}
	if len(seen) != count {
		t.Fatalf("unique values = %d, want %d", len(seen), count)
	}
}

func TestGeneratedRandomnessIsNotAllZero(t *testing.T) {
	// 20,000 identifiers with a 96-bit all-zero suffix would indicate a
	// broken entropy source with overwhelming probability; this is a sanity
	// check, not a randomness test.
	const count = 20_000
	var zeroes int
	for i := 0; i < count; i++ {
		id, err := Generate()
		if err != nil {
			t.Fatalf("Generate #%d: %v", i, err)
		}
		if id.Randomness() == [RandomnessBytes]byte{} {
			zeroes++
		}
	}
	if zeroes != 0 {
		t.Fatalf("%d of %d generated identifiers had all-zero randomness", zeroes, count)
	}
}
