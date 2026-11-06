package pikaid

import (
	"bytes"
	"testing"
)

func FuzzParse(f *testing.F) {
	seeds := []string{
		"00000000000000000000000000",
		"00000010000000000000000001",
		"zzzzzzz7oiylpimjg5u2ca1ypr",
		"0s44we8002loz3po6mcf6vsn23",
		"0000000zzzzzzzzzzzzzzzzzzz",
		"000000A0000000000000000000",
		"",
		"0000000000000000000000000",
		"😀000000000000000000000000",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		id, err := Parse(s)
		if err != nil {
			if IsValid(s) {
				t.Fatalf("IsValid(%q) = true but Parse failed: %v", s, err)
			}
			return
		}

		if !IsValid(s) {
			t.Fatalf("IsValid(%q) = false but Parse succeeded", s)
		}
		if len(s) != StringLength {
			t.Fatalf("Parse accepted a %d-byte string", len(s))
		}
		if got := id.String(); got != s {
			t.Fatalf("String() = %q, want %q", got, s)
		}
		if id.Timestamp() < 0 || id.Timestamp() > MaxTimestamp {
			t.Fatalf("timestamp out of range: %d", id.Timestamp())
		}

		b := id.Bytes()
		decoded, err := FromBinary(b[:])
		if err != nil {
			t.Fatalf("FromBinary(Bytes()) failed: %v", err)
		}
		if decoded != id {
			t.Fatalf("binary round trip changed the value")
		}

		text, err := id.MarshalText()
		if err != nil || !bytes.Equal(text, []byte(s)) {
			t.Fatalf("MarshalText = %q, %v", text, err)
		}
		var fromText ID
		if err := fromText.UnmarshalText(text); err != nil || fromText != id {
			t.Fatalf("text round trip = %v, %v", fromText, err)
		}
	})
}

func FuzzFromBinary(f *testing.F) {
	f.Add(bytes.Repeat([]byte{0}, BinaryLength))
	f.Add(bytes.Repeat([]byte{0xff}, BinaryLength))
	f.Add([]byte{0x12, 0x3e, 0xde, 0x3f, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff})
	f.Add(make([]byte, BinaryLength-1))
	f.Add(make([]byte, BinaryLength+1))

	f.Fuzz(func(t *testing.T, b []byte) {
		id, err := FromBinary(b)
		if err != nil {
			if len(b) == BinaryLength {
				// Only an out-of-range timestamp can reject a 17-byte buffer.
				timestamp := uint64(b[0])<<32 | uint64(b[1])<<24 | uint64(b[2])<<16 |
					uint64(b[3])<<8 | uint64(b[4])
				if timestamp <= MaxTimestamp {
					t.Fatalf("valid 17-byte buffer rejected: %x (%v)", b, err)
				}
			}
			return
		}
		if len(b) != BinaryLength {
			t.Fatalf("FromBinary accepted %d bytes", len(b))
		}

		got := id.Bytes()
		if !bytes.Equal(got[:], b) {
			t.Fatalf("Bytes() = %x, want %x", got, b)
		}
		if len(id.String()) != StringLength {
			t.Fatalf("string length = %d", len(id.String()))
		}
	})
}
