package pikaid

import (
	"encoding/binary"
	"math/bits"
)

// base36 is the radix of the pikaid alphabet.
const base36 = 36

// alphabet is the canonical pikaid alphabet, in ascending digit order.
// Only these bytes are accepted; uppercase letters are not normalized.
const alphabet = "0123456789abcdefghijklmnopqrstuvwxyz"

// isBase36 reports whether c belongs to the lowercase Base36 alphabet.
func isBase36(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z')
}

// digitValue returns the numeric value of a Base36 digit.
func digitValue(c byte) (uint64, bool) {
	switch {
	case c >= '0' && c <= '9':
		return uint64(c - '0'), true
	case c >= 'a' && c <= 'z':
		return uint64(c-'a') + 10, true
	default:
		return 0, false
	}
}

// decodeTimestamp decodes exactly TimestampLength Base36 digits. Seven digits
// cannot exceed 36^7-1, so the result is always a valid pikaid timestamp.
// The input must already be known to contain only Base36 bytes.
func decodeTimestamp(s string) uint64 {
	var v uint64
	for i := 0; i < len(s); i++ {
		d, _ := digitValue(s[i])
		v = v*base36 + d
	}
	return v
}

// decodeRandomness decodes exactly RandomnessLength Base36 digits, rejecting
// values that do not fit in 96 bits. The input must already be known to
// contain only Base36 bytes.
func decodeRandomness(s string) ([RandomnessBytes]byte, error) {
	var hi, lo uint64

	for i := 0; i < len(s); i++ {
		d, _ := digitValue(s[i])

		// (hi:lo) = (hi:lo)*36 + d, tracking the high 64 bits explicitly so
		// that an oversized suffix is detected instead of wrapping.
		mulHi, mulLo := bits.Mul64(lo, base36)
		sum, carry := bits.Add64(mulLo, d, 0)
		lo = sum
		hi = hi*base36 + mulHi + carry

		// A 96-bit value has at most 32 significant high bits. Anything above
		// that is outside the representable randomness range.
		if hi > 0xffff_ffff {
			return [RandomnessBytes]byte{}, randomnessOverflowError(s)
		}
	}

	var out [RandomnessBytes]byte
	binary.BigEndian.PutUint32(out[0:4], uint32(hi))
	binary.BigEndian.PutUint64(out[4:12], lo)
	return out, nil
}

// encodeTimestamp encodes a timestamp as exactly TimestampLength Base36 digits,
// left-padded with '0'.
func encodeTimestamp(timestamp uint64) [TimestampLength]byte {
	var out [TimestampLength]byte
	for i := TimestampLength - 1; i >= 0; i-- {
		out[i] = alphabet[timestamp%base36]
		timestamp /= base36
	}
	return out
}

// encodeRandomness encodes 12 randomness bytes as exactly RandomnessLength
// Base36 digits, left-padded with '0'.
func encodeRandomness(randomness [RandomnessBytes]byte) [RandomnessLength]byte {
	hi := uint64(binary.BigEndian.Uint32(randomness[0:4]))
	lo := binary.BigEndian.Uint64(randomness[4:12])

	var out [RandomnessLength]byte
	for i := RandomnessLength - 1; i >= 0; i-- {
		// Divide the 96-bit (hi:lo) value by the radix. bits.Div64 requires
		// its high argument to be less than the divisor; the remainder of the
		// high half always is.
		qhi, rem := hi/base36, hi%base36
		qlo, rem := bits.Div64(rem, lo, base36)
		hi, lo = qhi, qlo
		out[i] = alphabet[rem]
	}
	return out
}
