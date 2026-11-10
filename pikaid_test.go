package pikaid

import (
	"bytes"
	"encoding/hex"
	"errors"
	"math/big"
	"strings"
	"testing"
)

// knownAnswers was computed independently with Python integers, not by this
// package. It covers zero values, both maxima, zero padding, the 36^6 and
// 2^95 boundaries, and mixed high/low bytes.
var knownAnswers = []struct {
	timestamp int64
	entropy   string
	id        string
	binary    string
}{
	{0, "000000000000000000000000", "00000000000000000000000000", "0000000000000000000000000000000000"},
	{1, "000000000000000000000001", "00000010000000000000000001", "0000000001000000000000000000000001"},
	{1700000000, "00112233445566778899aabb", "0s44we8002loz3po6mcf6vsn23", "006553f10000112233445566778899aabb"},
	{78364164095, "ffffffffffffffffffffffff", "zzzzzzz7oiylpimjg5u2ca1ypr", "123ede3fffffffffffffffffffffffffff"},
	{78364164095, "000000000000000000000000", "zzzzzzz0000000000000000000", "123ede3fff000000000000000000000000"},
	{0, "ffffffffffffffffffffffff", "00000007oiylpimjg5u2ca1ypr", "0000000000ffffffffffffffffffffffff"},
	{123456789, "deadbeefcafebabe01234567", "021i3v96oj7k6n2wcap1xa965j", "00075bcd15deadbeefcafebabe01234567"},
	{2176782336, "000000000000000000000001", "10000000000000000000000001", "0081bf1000000000000000000000000001"},
	{1234567890, "800000000000000000000000", "0kf12oi3u9haurb9q2x1650zcw", "00499602d2800000000000000000000000"},
	{78364164094, "7fffffffffffffffffffffff", "zzzzzzy3u9haurb9q2x1650zcv", "123ede3ffe7fffffffffffffffffffffff"},
	{2, "0123456789abcdef01234567", "00000020188skr4f4jv458xrhj", "00000000020123456789abcdef01234567"},
}

func mustEntropy(t *testing.T, s string) [RandomnessBytes]byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != RandomnessBytes {
		t.Fatalf("bad entropy fixture %q: %v", s, err)
	}
	var out [RandomnessBytes]byte
	copy(out[:], b)
	return out
}

func TestKnownAnswers(t *testing.T) {
	for _, tc := range knownAnswers {
		tc := tc
		t.Run(tc.id, func(t *testing.T) {
			id, err := Parse(tc.id)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if id.Timestamp() != tc.timestamp {
				t.Errorf("Timestamp() = %d, want %d", id.Timestamp(), tc.timestamp)
			}
			if id.RandomnessHex() != tc.entropy {
				t.Errorf("RandomnessHex() = %s, want %s", id.RandomnessHex(), tc.entropy)
			}
			b := id.Bytes()
			if got := hex.EncodeToString(b[:]); got != tc.binary {
				t.Errorf("Bytes() = %s, want %s", got, tc.binary)
			}

			bin, _ := hex.DecodeString(tc.binary)
			fromBin, err := FromBinary(bin)
			if err != nil {
				t.Fatalf("FromBinary: %v", err)
			}
			if fromBin != id || fromBin.String() != tc.id {
				t.Errorf("FromBinary round trip = %q, want %q", fromBin, tc.id)
			}

			ent := mustEntropy(t, tc.entropy)
			if got := newID(uint64(tc.timestamp), ent).String(); got != tc.id {
				t.Errorf("encoding fields = %q, want %q", got, tc.id)
			}
		})
	}
}

// TestStringBinaryAgainstBigIntOracle validates encoding and decoding against
// an independent implementation using math/big rather than this package's
// fixed-width integer arithmetic.
func TestStringBinaryAgainstBigIntOracle(t *testing.T) {
	values := make([]struct {
		timestamp uint64
		entropy   [RandomnessBytes]byte
	}, 0, 512)

	// Deterministic sweep, including all single-byte high and low values.
	for i := 0; i < 256; i++ {
		var e [RandomnessBytes]byte
		e[0] = byte(i)
		e[11] = byte(255 - i)
		e[5] = byte(i * 7)
		values = append(values, struct {
			timestamp uint64
			entropy   [RandomnessBytes]byte
		}{uint64(i) * 306_783_353, e})
	}

	// Additional boundaries.
	for _, ts := range []uint64{0, 1, MaxTimestamp - 1, MaxTimestamp} {
		values = append(values, struct {
			timestamp uint64
			entropy   [RandomnessBytes]byte
		}{ts, [RandomnessBytes]byte{}})
		values = append(values, struct {
			timestamp uint64
			entropy   [RandomnessBytes]byte
		}{ts, [RandomnessBytes]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}})
	}

	for _, v := range values {
		id := newID(v.timestamp, v.entropy)

		tsValue := new(big.Int).SetUint64(v.timestamp)
		entValue := new(big.Int).SetBytes(v.entropy[:])
		wantString := padBase36(tsValue, TimestampLength) + padBase36(entValue, RandomnessLength)

		if got := id.String(); got != wantString {
			t.Fatalf("String() = %q, want %q (ts=%d entropy=%x)", got, wantString, v.timestamp, v.entropy)
		}

		wantBinary := append(tsValue.FillBytes(make([]byte, TimestampBytes)), v.entropy[:]...)
		gotBinary := id.Bytes()
		if !bytes.Equal(gotBinary[:], wantBinary) {
			t.Fatalf("Bytes() = %x, want %x", gotBinary, wantBinary)
		}

		parsed, err := Parse(wantString)
		if err != nil {
			t.Fatalf("Parse(%q): %v", wantString, err)
		}
		if parsed.Timestamp() != int64(v.timestamp) || parsed.RandomnessHex() != hex.EncodeToString(v.entropy[:]) {
			t.Fatalf("Parse(%q) = (%d, %s), want (%d, %x)",
				wantString, parsed.Timestamp(), parsed.RandomnessHex(), v.timestamp, v.entropy)
		}
		if parsed != id {
			t.Fatalf("Parse(%q) != original ID", wantString)
		}
	}
}

// padBase36 renders v in Base36 with exactly width lowercase digits. big.Int
// uses the same alphabet and emits no leading zeros.
func padBase36(v *big.Int, width int) string {
	s := v.Text(36)
	if len(s) > width {
		return strings.Repeat("!", len(s)-width) + s
	}
	return strings.Repeat("0", width-len(s)) + s
}

func TestTimestampBoundaries(t *testing.T) {
	tests := []struct {
		name      string
		timestamp int64
		wantOK    bool
	}{
		{"zero", 0, true},
		{"one", 1, true},
		{"epoch-plus-1e9", 1_000_000_000, true},
		{"max", MaxTimestamp, true},
		{"max-minus-one", MaxTimestamp - 1, true},
		{"max-plus-one", MaxTimestamp + 1, false},
		{"negative", -1, false},
		{"min-int64", -1 << 63, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			id, err := GenerateWithTimestamp(tc.timestamp)
			if tc.wantOK {
				if err != nil {
					t.Fatalf("GenerateWithTimestamp(%d): %v", tc.timestamp, err)
				}
				if id.Timestamp() != tc.timestamp {
					t.Errorf("timestamp = %d, want %d", id.Timestamp(), tc.timestamp)
				}
			} else {
				if err == nil {
					t.Fatalf("GenerateWithTimestamp(%d) accepted an invalid timestamp", tc.timestamp)
				}
				if !errors.Is(err, ErrTimestampOutOfRange) {
					t.Errorf("error = %v, want ErrTimestampOutOfRange", err)
				}
			}

			bin, err := GenerateBinaryWithTimestamp(tc.timestamp)
			if tc.wantOK != (err == nil) {
				t.Fatalf("GenerateBinaryWithTimestamp(%d) err = %v, wantOK %v", tc.timestamp, err, tc.wantOK)
			}
			if tc.wantOK {
				parsed, perr := FromBinary(bin[:])
				if perr != nil {
					t.Fatalf("FromBinary generated value: %v", perr)
				}
				if parsed.Timestamp() != tc.timestamp {
					t.Errorf("binary timestamp = %d, want %d", parsed.Timestamp(), tc.timestamp)
				}
			} else if !errors.Is(err, ErrTimestampOutOfRange) {
				t.Errorf("binary error = %v, want ErrTimestampOutOfRange", err)
			}
		})
	}
}

func TestEntropyBoundaries(t *testing.T) {
	const maxSuffix = "7oiylpimjg5u2ca1ypr"      // 2^96-1
	const overflowSuffix = "7oiylpimjg5u2ca1yps" // 2^96

	if !IsValid("0000000" + maxSuffix) {
		t.Error("maximum 96-bit randomness must be valid")
	}
	id, err := Parse("0000000" + maxSuffix)
	if err != nil {
		t.Fatalf("Parse(max randomness): %v", err)
	}
	if id.RandomnessHex() != "ffffffffffffffffffffffff" {
		t.Errorf("max randomness = %s, want ffffffffffffffffffffffff", id.RandomnessHex())
	}

	for _, suffix := range []string{
		maxSuffix,
		"zzzzzzzzzzzzzzzzzzz",
		"7oiylpimjg5u2ca1yps",
		strings.Repeat("z", RandomnessLength),
	} {
		s := "0000000" + suffix
		want := suffix == maxSuffix
		if got := IsValid(s); got != want {
			t.Errorf("IsValid(%q) = %v, want %v", s, got, want)
		}
		if _, err := Parse(s); (err == nil) != want {
			t.Errorf("Parse(%q) err = %v, want success %v", s, err, want)
		}
		if suffix != maxSuffix && (!errors.Is(errFromParse(s), ErrRandomnessOverflow)) {
			t.Errorf("Parse(%q) error = %v, want ErrRandomnessOverflow", s, errFromParse(s))
		}
	}

	// The 19-character encoding space is larger than 96 bits: only suffixes
	// up to 2^96-1 are valid, even though they look well formed.
	if !IsWellFormed("0000000" + strings.Repeat("z", RandomnessLength)) {
		t.Error("19 z characters should be well formed")
	}
}

func errFromParse(s string) error {
	_, err := Parse(s)
	return err
}

func TestZeroPadding(t *testing.T) {
	zeros := strings.Repeat("0", RandomnessLength)
	tests := []struct {
		timestamp int64
		entropy   string
		want      string
	}{
		{0, "000000000000000000000000", "0000000" + zeros},
		{1, "000000000000000000000000", "0000001" + zeros},
		{0, "000000000000000000000001", "0000000" + zeros[:len(zeros)-1] + "1"},
		{35, "000000000000000000000000", "000000z" + zeros},
		{36, "000000000000000000000000", "0000010" + zeros},
	}

	for _, tc := range tests {
		ent := mustEntropy(t, tc.entropy)
		if got := newID(uint64(tc.timestamp), ent).String(); got != tc.want {
			t.Errorf("newID(%d, %s) = %q, want %q", tc.timestamp, tc.entropy, got, tc.want)
		}
		if len(tc.want) != StringLength {
			t.Errorf("fixture %q has length %d", tc.want, len(tc.want))
		}
	}
}

func TestParseRejectsMalformed(t *testing.T) {
	zeros := func(n int) string { return strings.Repeat("0", n) }
	tests := []struct {
		name string
		in   string
		want error
	}{
		{"empty", "", ErrInvalidLength},
		{"too-short", zeros(25), ErrInvalidLength},
		{"too-long", zeros(27), ErrInvalidLength},
		{"uppercase-timestamp", "000000A" + zeros(19), ErrInvalidCharacter},
		{"uppercase-suffix", zeros(25) + "A", ErrInvalidCharacter},
		{"dash", "000000-" + zeros(19), ErrInvalidCharacter},
		{"underscore", "000000_" + zeros(19), ErrInvalidCharacter},
		{"space", "000000 " + zeros(19), ErrInvalidCharacter},
		{"prefix", "pikaid_" + zeros(19), ErrInvalidCharacter},
		{"0X-prefix", "0X" + zeros(24), ErrInvalidCharacter},
		{"newline", zeros(26) + "\n", ErrInvalidLength},
		{"trailing-space", zeros(25) + " ", ErrInvalidCharacter},
		{"nul", "000000\x00" + zeros(19), ErrInvalidCharacter},
		{"accented", "é" + zeros(24), ErrInvalidCharacter},
		{"emoji", "😀" + zeros(22), ErrInvalidCharacter},
		{"entropy-overflow", "0000000" + strings.Repeat("z", RandomnessLength), ErrRandomnessOverflow},
		{"entropy-overflow-threshold", "0000000" + "7oiylpimjg5u2ca1yps", ErrRandomnessOverflow},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			id, err := Parse(tc.in)
			if err == nil {
				t.Fatalf("Parse(%q) = %v, want error", tc.in, id)
			}
			if !errors.Is(err, tc.want) {
				t.Errorf("error = %v, want errors.Is(_, %v)", err, tc.want)
			}
			if IsValid(tc.in) {
				t.Errorf("IsValid(%q) = true", tc.in)
			}
		})
	}
}

func TestParseErrorMessagesCarryDetail(t *testing.T) {
	for _, in := range []string{"", "000000A0000000000000000000", "0000000" + strings.Repeat("z", RandomnessLength)} {
		_, err := Parse(in)
		if err == nil {
			t.Fatalf("Parse(%q) succeeded", in)
		}
		if len(err.Error()) <= len("pikaid: ") {
			t.Errorf("error %q lacks contextual detail", err)
		}
	}
}

func TestIsWellFormedVersusIsValid(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		wellFormd bool
		valid     bool
	}{
		{"canonical", "00000000000000000000000000", true, true},
		{"overflowing-suffix", "0000000" + strings.Repeat("z", RandomnessLength), true, false},
		{"uppercase", "000000A0000000000000000000", false, false},
		{"short", "0000000000000000000000000", false, false},
		{"long", "000000000000000000000000000", false, false},
	}
	for _, tc := range tests {
		if got := IsWellFormed(tc.in); got != tc.wellFormd {
			t.Errorf("%s: IsWellFormed = %v, want %v", tc.name, got, tc.wellFormd)
		}
		if got := IsValid(tc.in); got != tc.valid {
			t.Errorf("%s: IsValid = %v, want %v", tc.name, got, tc.valid)
		}
	}
}

func TestCompare(t *testing.T) {
	pairs := []struct {
		name string
		a, b string
		want int
	}{
		{"equal", "00000000000000000000000000", "00000000000000000000000000", 0},
		{"different-seconds", "0000001" + strings.Repeat("0", 19), "0000002" + strings.Repeat("0", 19), -1},
		{"different-seconds-reversed", "0000002" + strings.Repeat("0", 19), "0000001" + strings.Repeat("0", 19), 1},
		{"same-second-entropy-low-high", "0000001" + strings.Repeat("0", 19), "0000001" + "7oiylpimjg5u2ca1ypr", -1},
		{"same-second-entropy-high-low", "0000001" + "7oiylpimjg5u2ca1ypr", "0000001" + strings.Repeat("0", 19), 1},
		{"same-second-middle", "0000001" + "0000000000000000000", "0000001" + "0000000000000000001", -1},
		{"timestamp-dominates-suffix", "0000001" + "7oiylpimjg5u2ca1ypr", "0000002" + strings.Repeat("0", 19), -1},
		{"max-versus-zero", "zzzzzzz7oiylpimjg5u2ca1ypr", "00000000000000000000000000", 1},
	}

	for _, tc := range pairs {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			a, err := Parse(tc.a)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tc.a, err)
			}
			b, err := Parse(tc.b)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tc.b, err)
			}
			if got := a.Compare(b); got != tc.want {
				t.Errorf("Compare = %d, want %d", got, tc.want)
			}
			wantReverse := -tc.want
			if got := b.Compare(a); got != wantReverse {
				t.Errorf("reverse Compare = %d, want %d", got, wantReverse)
			}
			if got := a.Compare(a); got != 0 {
				t.Errorf("self Compare = %d, want 0", got)
			}

			// String and binary order must agree with Compare.
			stringSign := sign(strings.Compare(tc.a, tc.b))
			if stringSign != tc.want {
				t.Errorf("strings.Compare = %d, want %d", stringSign, tc.want)
			}
			ab, bb := a.Bytes(), b.Bytes()
			if bytesSign := sign(bytes.Compare(ab[:], bb[:])); bytesSign != tc.want {
				t.Errorf("bytes.Compare = %d, want %d", bytesSign, tc.want)
			}
		})
	}
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	default:
		return 0
	}
}

func TestStringBinaryRoundTripProperties(t *testing.T) {
	// A deterministic LCG supplies inputs; correctness is judged by
	// independent oracles elsewhere, here we check lossless round trips.
	state := uint64(0x243f6a8885a308d3)
	next := func() uint64 {
		state = state*6364136223846793005 + 1442695040888963407
		return state
	}

	for i := 0; i < 2000; i++ {
		timestamp := next() % (MaxTimestamp + 1)

		var entropy [RandomnessBytes]byte
		for j := 0; j < RandomnessBytes; j += 8 {
			v := next()
			for k := 0; k < 8 && j+k < RandomnessBytes; k++ {
				entropy[j+k] = byte(v >> (8 * k))
			}
		}

		id := newID(timestamp, entropy)
		text := id.String()
		if len(text) != StringLength {
			t.Fatalf("len(String()) = %d", len(text))
		}
		if !IsValid(text) || !IsWellFormed(text) {
			t.Fatalf("generated text %q failed validation", text)
		}

		reparsed, err := Parse(text)
		if err != nil {
			t.Fatalf("Parse(%q): %v", text, err)
		}
		if reparsed != id {
			t.Fatalf("text round trip lost data: %q", text)
		}

		binary := id.Bytes()
		fromBin, err := FromBinary(binary[:])
		if err != nil {
			t.Fatalf("FromBinary(%x): %v", binary, err)
		}
		if fromBin != id {
			t.Fatalf("binary round trip lost data: %x", binary)
		}

		if other := newID(next()%(MaxTimestamp+1), entropy); other != id {
			if sign(strings.Compare(text, other.String())) != id.Compare(other) {
				t.Fatalf("Compare disagrees with string order for %q and %q", text, other.String())
			}
		}
	}
}

func TestMarshalText(t *testing.T) {
	id, err := Parse("00000010000000000000000001")
	if err != nil {
		t.Fatal(err)
	}

	text, err := id.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText: %v", err)
	}
	if string(text) != id.String() {
		t.Errorf("MarshalText = %q, want %q", text, id.String())
	}

	var decoded ID
	if err := decoded.UnmarshalText(text); err != nil {
		t.Fatalf("UnmarshalText: %v", err)
	}
	if decoded != id {
		t.Errorf("UnmarshalText = %v, want %v", decoded, id)
	}

	for _, bad := range [][]byte{
		nil,
		[]byte("short"),
		[]byte("000000A0000000000000000000"),
		[]byte("0000000" + strings.Repeat("z", RandomnessLength)),
	} {
		sentinel := decoded
		if err := decoded.UnmarshalText(bad); err == nil {
			t.Errorf("UnmarshalText(%q) returned no error", bad)
		}
		if decoded != sentinel {
			t.Errorf("UnmarshalText(%q) modified the receiver on error", bad)
		}
	}
}

func TestMarshalBinary(t *testing.T) {
	id, err := Parse("zzzzzzz7oiylpimjg5u2ca1ypr")
	if err != nil {
		t.Fatal(err)
	}

	data, err := id.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary: %v", err)
	}
	if len(data) != BinaryLength {
		t.Fatalf("MarshalBinary length = %d", len(data))
	}

	var decoded ID
	if err := decoded.UnmarshalBinary(data); err != nil {
		t.Fatalf("UnmarshalBinary: %v", err)
	}
	if decoded != id {
		t.Errorf("UnmarshalBinary = %v, want %v", decoded, id)
	}

	tooShort := data[:BinaryLength-1]
	if err := decoded.UnmarshalBinary(tooShort); !errors.Is(err, ErrInvalidLength) {
		t.Errorf("short UnmarshalBinary error = %v, want ErrInvalidLength", err)
	}
	if decoded != id {
		t.Error("UnmarshalBinary modified the receiver on error")
	}

	overflow, _ := hex.DecodeString("123ede4000000000000000000000000000")
	if err := decoded.UnmarshalBinary(overflow); !errors.Is(err, ErrTimestampOutOfRange) {
		t.Errorf("overflow UnmarshalBinary error = %v, want ErrTimestampOutOfRange", err)
	}
}

func TestFromBinaryRejectsInvalid(t *testing.T) {
	tests := []struct {
		name string
		hex  string
		want error
	}{
		{"empty", "", ErrInvalidLength},
		{"too-short", "00000000000000000000000000000000", ErrInvalidLength},
		{"too-long", "000000000000000000000000000000000000", ErrInvalidLength},
		{"timestamp-max-plus-one", "123ede4000000000000000000000000000", ErrTimestampOutOfRange},
		{"timestamp-40-bit-max", "ffffffffff000000000000000000000000", ErrTimestampOutOfRange},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			b, err := hex.DecodeString(tc.hex)
			if err != nil {
				t.Fatalf("bad fixture: %v", err)
			}
			if _, err := FromBinary(b); !errors.Is(err, tc.want) {
				t.Errorf("error = %v, want %v", err, tc.want)
			}
		})
	}

	// The largest valid binary timestamp still decodes.
	b, _ := hex.DecodeString("123ede3fffffffffffffffffffffffffff")
	id, err := FromBinary(b)
	if err != nil {
		t.Fatalf("FromBinary(max): %v", err)
	}
	if id.Timestamp() != MaxTimestamp {
		t.Errorf("timestamp = %d, want %d", id.Timestamp(), int64(MaxTimestamp))
	}
}

func TestZeroValueID(t *testing.T) {
	var id ID
	if got := id.String(); got != "00000000000000000000000000" {
		t.Errorf("zero ID String() = %q", got)
	}
	if id.Timestamp() != 0 || id.RandomnessHex() != "000000000000000000000000" {
		t.Errorf("zero ID fields = (%d, %s)", id.Timestamp(), id.RandomnessHex())
	}
	if !IsValid(id.String()) {
		t.Error("zero ID string is not valid")
	}
	if id.Compare(ID{}) != 0 {
		t.Error("zero IDs do not compare equal")
	}

	// The zero value is the all-zero pikaid, so the all-zero binary value is
	// valid rather than an error.
	b := id.Bytes()
	if b != [BinaryLength]byte{} {
		t.Errorf("zero ID Bytes() = %x", b)
	}
	if _, err := FromBinary(b[:]); err != nil {
		t.Errorf("FromBinary(all zero): %v", err)
	}
}

func TestRandomnessReturnsCopy(t *testing.T) {
	id, err := Parse("00000010000000000000000001")
	if err != nil {
		t.Fatal(err)
	}
	r := id.Randomness()
	r[0] = 0xff
	if id.Randomness()[0] == 0xff {
		t.Error("Randomness exposed the internal buffer")
	}
}

func TestTimestampTimeIsUTC(t *testing.T) {
	id, err := Parse("0s44we8002loz3po6mcf6vsn23")
	if err != nil {
		t.Fatal(err)
	}
	tm := id.TimestampTime()
	if tm.Location().String() != "UTC" {
		t.Errorf("location = %s, want UTC", tm.Location())
	}
	if tm.Unix() != 1_700_000_000 {
		t.Errorf("unix = %d, want 1700000000", tm.Unix())
	}
	if tm.Nanosecond() != 0 {
		t.Errorf("nanosecond = %d, want 0", tm.Nanosecond())
	}
}
