package pikaid

import (
	"bytes"
	"errors"
	"io"
	"math"
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

type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }

// readRecorder reports whether the entropy source was consulted.
type readRecorder struct{ reads int }

func (r *readRecorder) Read([]byte) (int, error) {
	r.reads++
	return 0, errors.New("entropy unavailable")
}

// TestGenerateWithReaderUsesSuppliedRandomness pins the private generation core
// against a deterministic reader, including both timestamp boundaries.
func TestGenerateWithReaderUsesSuppliedRandomness(t *testing.T) {
	source := make([]byte, RandomnessBytes)
	for i := range source {
		source[i] = byte(0xa0 + i)
	}
	var wantRandomness [RandomnessBytes]byte
	copy(wantRandomness[:], source)

	for _, timestamp := range []int64{0, MaxTimestamp} {
		id, err := generateWithReader(bytes.NewReader(source), timestamp)
		if err != nil {
			t.Fatalf("generateWithReader(%d): %v", timestamp, err)
		}
		want := newID(uint64(timestamp), wantRandomness)
		if id != want {
			t.Errorf("generateWithReader(%d) = %v, want %v", timestamp, id, want)
		}
	}
}

// TestGenerateRejectsTimestampBeforeReadingEntropy proves the fail-closed
// ordering: an out-of-range timestamp must not consume entropy.
func TestGenerateRejectsTimestampBeforeReadingEntropy(t *testing.T) {
	timestamps := []int64{-1, math.MinInt64, MaxTimestamp + 1, math.MaxInt64}

	for _, timestamp := range timestamps {
		recorder := &readRecorder{}
		id, err := generateWithReader(recorder, timestamp)
		if !errors.Is(err, ErrTimestampOutOfRange) {
			t.Errorf("timestamp %d: error = %v, want ErrTimestampOutOfRange", timestamp, err)
		}
		if recorder.reads != 0 {
			t.Errorf("timestamp %d: random source read %d times, want 0", timestamp, recorder.reads)
		}
		if id != (ID{}) {
			t.Errorf("timestamp %d: returned partial ID %v", timestamp, id)
		}
	}
}

func TestGenerateFailsClosedOnRandomSourceError(t *testing.T) {
	sourceErr := errors.New("entropy unavailable")
	tests := []struct {
		name   string
		reader io.Reader
	}{
		{"error", failingReader{err: sourceErr}},
		{"truncated", bytes.NewReader(make([]byte, RandomnessBytes-1))},
		{"eof", strings.NewReader("")},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			id, err := generateWithReader(tc.reader, 1_700_000_000)
			if err == nil {
				t.Fatalf("generateWithReader returned %v without an error", id)
			}
			if !errors.Is(err, ErrRandomSource) {
				t.Errorf("error = %v, want ErrRandomSource", err)
			}
			if id != (ID{}) {
				t.Errorf("generateWithReader returned a partial ID: %v", id)
			}
			// The underlying source error is preserved in the chain.
			if tc.name == "error" && !errors.Is(err, sourceErr) {
				t.Errorf("error = %v, want wrapped source error %v", err, sourceErr)
			}
		})
	}
}

func TestGenerateBinaryFailsClosedOnRandomSourceError(t *testing.T) {
	sourceErr := errors.New("entropy unavailable")
	bin, err := generateBinaryWithReader(failingReader{err: sourceErr}, 1_700_000_000)
	if !errors.Is(err, ErrRandomSource) {
		t.Errorf("error = %v, want ErrRandomSource", err)
	}
	if !errors.Is(err, sourceErr) {
		t.Errorf("error = %v, want wrapped source error %v", err, sourceErr)
	}
	if bin != ([BinaryLength]byte{}) {
		t.Errorf("returned partial bytes: %x", bin)
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
