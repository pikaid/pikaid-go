package pikaid

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

type vectorFile struct {
	Meta struct {
		StringLength int `json:"string_length"`
		Timestamp    struct {
			Chars int    `json:"chars"`
			Min   uint64 `json:"min"`
			Max   uint64 `json:"max"`
		} `json:"timestamp"`
		Entropy struct {
			Bytes int `json:"bytes"`
			Chars int `json:"chars"`
		} `json:"entropy"`
		Binary struct {
			Bytes int `json:"bytes"`
		} `json:"binary"`
		SpecRevision string `json:"spec_revision"`
	} `json:"meta"`
	Valid []struct {
		Name      string `json:"name"`
		Timestamp int64  `json:"timestamp"`
		Entropy   string `json:"entropy_hex"`
		ID        string `json:"id"`
		Binary    string `json:"bin_hex"`
	} `json:"valid"`
	Invalid struct {
		String []struct {
			Name string `json:"name"`
			ID   string `json:"id"`
		} `json:"string"`
		Binary []struct {
			Name   string `json:"name"`
			Binary string `json:"bin_hex"`
		} `json:"binary"`
	} `json:"invalid"`
}

func loadVectors(t *testing.T) vectorFile {
	t.Helper()

	raw, err := os.ReadFile("testdata/pikaid-v1.json")
	if err != nil {
		t.Fatalf("read vectors: %v", err)
	}
	var v vectorFile
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("decode vectors: %v", err)
	}

	// Guard against a stale or edited fixture: the values below are the
	// format dimensions fixed by specification revision 1.0.4.
	if v.Meta.StringLength != StringLength ||
		v.Meta.Timestamp.Chars != TimestampLength ||
		v.Meta.Timestamp.Min != 0 ||
		v.Meta.Timestamp.Max != MaxTimestamp ||
		v.Meta.Entropy.Bytes != RandomnessBytes ||
		v.Meta.Entropy.Chars != RandomnessLength ||
		v.Meta.Binary.Bytes != BinaryLength ||
		v.Meta.SpecRevision != "1.0.4" {
		t.Fatalf("unexpected vector metadata: %+v", v.Meta)
	}
	if len(v.Valid) == 0 || len(v.Invalid.String) == 0 || len(v.Invalid.Binary) == 0 {
		t.Fatal("vector file is missing cases")
	}
	return v
}

// TestOfficialVectors checks every official PHP v1.2.0 valid vector against
// parsing, binary decoding, field extraction and both conversion directions.
func TestOfficialVectors(t *testing.T) {
	v := loadVectors(t)

	for _, tc := range v.Valid {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			bin, err := hex.DecodeString(tc.Binary)
			if err != nil || len(bin) != BinaryLength {
				t.Fatalf("fixture binary is invalid: %v", err)
			}
			entropy, err := hex.DecodeString(tc.Entropy)
			if err != nil || len(entropy) != RandomnessBytes {
				t.Fatalf("fixture entropy is invalid: %v", err)
			}

			if !IsValid(tc.ID) || !IsWellFormed(tc.ID) {
				t.Fatalf("IsValid/IsWellFormed = false for %q", tc.ID)
			}

			id, err := Parse(tc.ID)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tc.ID, err)
			}
			if got := id.String(); got != tc.ID {
				t.Errorf("String() = %q, want %q", got, tc.ID)
			}
			if got := id.Timestamp(); got != tc.Timestamp {
				t.Errorf("Timestamp() = %d, want %d", got, tc.Timestamp)
			}
			if got := id.RandomnessHex(); got != tc.Entropy {
				t.Errorf("RandomnessHex() = %q, want %q", got, tc.Entropy)
			}
			if got := id.Randomness(); !bytes.Equal(got[:], entropy) {
				t.Errorf("Randomness() = %x, want %s", got, tc.Entropy)
			}
			if got := id.TimestampTime().Unix(); got != tc.Timestamp {
				t.Errorf("TimestampTime().Unix() = %d, want %d", got, tc.Timestamp)
			}

			gotBin := id.Bytes()
			if hex.EncodeToString(gotBin[:]) != tc.Binary {
				t.Errorf("Bytes() = %x, want %s", gotBin, tc.Binary)
			}

			// The string encoder must reproduce the fixture from the decoded
			// fields, not merely echo the input string.
			var ent [RandomnessBytes]byte
			copy(ent[:], entropy)
			if got := newID(uint64(tc.Timestamp), ent).String(); got != tc.ID {
				t.Errorf("encoder produced %q, want %q", got, tc.ID)
			}

			fromBin, err := FromBinary(bin)
			if err != nil {
				t.Fatalf("FromBinary(%s): %v", tc.Binary, err)
			}
			if fromBin != id {
				t.Errorf("FromBinary(bin) = %v, want %v", fromBin, id)
			}
			if got := fromBin.String(); got != tc.ID {
				t.Errorf("FromBinary(bin).String() = %q, want %q", got, tc.ID)
			}

			marshaled, err := id.MarshalBinary()
			if err != nil {
				t.Fatalf("MarshalBinary: %v", err)
			}
			if !bytes.Equal(marshaled, bin) {
				t.Errorf("MarshalBinary() = %x, want %s", marshaled, tc.Binary)
			}
			text, err := id.MarshalText()
			if err != nil {
				t.Fatalf("MarshalText: %v", err)
			}
			if string(text) != tc.ID {
				t.Errorf("MarshalText() = %q, want %q", text, tc.ID)
			}
		})
	}
}

// TestOfficialInvalidStrings checks every official invalid string vector, and
// that the same inputs are rejected by the boolean and conversion APIs.
func TestOfficialInvalidStrings(t *testing.T) {
	v := loadVectors(t)

	for _, tc := range v.Invalid.String {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			if IsValid(tc.ID) {
				t.Errorf("IsValid(%q) = true, want false", tc.ID)
			}
			if id, err := Parse(tc.ID); err == nil {
				t.Errorf("Parse(%q) = %v, want error", tc.ID, id)
			}
			if _, err := Parse(tc.ID); err == nil {
				t.Errorf("Parse(%q) returned no error", tc.ID)
			}
		})
	}
}

// TestOfficialInvalidBinary checks every official invalid binary vector.
func TestOfficialInvalidBinary(t *testing.T) {
	v := loadVectors(t)

	for _, tc := range v.Invalid.Binary {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			bin, err := hex.DecodeString(tc.Binary)
			if err != nil {
				t.Fatalf("fixture binary is not hex: %v", err)
			}
			if _, err := FromBinary(bin); err == nil {
				t.Errorf("FromBinary(%x) returned no error", bin)
			}

			var id ID
			if err := id.UnmarshalBinary(bin); err == nil {
				t.Errorf("UnmarshalBinary(%x) returned no error", bin)
			}
		})
	}
}

// TestSpecExamples checks the interoperability examples in section 7 of
// specification revision 1.0.4, including the explicitly invalid row.
func TestSpecExamples(t *testing.T) {
	valid := []struct {
		id        string
		timestamp int64
		entropy   string
		binary    string
	}{
		{"00000000000000000000000000", 0, "000000000000000000000000", "0000000000000000000000000000000000"},
		{"00000010000000000000000001", 1, "000000000000000000000001", "0000000001000000000000000000000001"},
		{"zzzzzzz7oiylpimjg5u2ca1ypr", MaxTimestamp, "ffffffffffffffffffffffff", "123ede3fffffffffffffffffffffffffff"},
	}
	for _, tc := range valid {
		id, err := Parse(tc.id)
		if err != nil {
			t.Fatalf("Parse(%q): %v", tc.id, err)
		}
		if id.Timestamp() != tc.timestamp || id.RandomnessHex() != tc.entropy {
			t.Errorf("%q fields = (%d, %s), want (%d, %s)",
				tc.id, id.Timestamp(), id.RandomnessHex(), tc.timestamp, tc.entropy)
		}
		b := id.Bytes()
		if got := hex.EncodeToString(b[:]); got != tc.binary {
			t.Errorf("%q binary = %s, want %s", tc.id, got, tc.binary)
		}
	}

	const tooLarge = "0000000zzzzzzzzzzzzzzzzzzz"
	if !IsWellFormed(tooLarge) {
		t.Error("spec's overflow example should be well formed")
	}
	if IsValid(tooLarge) {
		t.Error("spec's overflow example must be invalid")
	}
	if _, err := Parse(tooLarge); err == nil {
		t.Error("spec's overflow example must not parse")
	}
}
