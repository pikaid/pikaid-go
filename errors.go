package pikaid

import "errors"

// Sentinel errors returned by this package. Parse, FromBinary,
// UnmarshalText, UnmarshalBinary and the generation functions wrap one of
// these with contextual detail, so callers can classify failures with
// errors.Is.
var (
	// ErrInvalidLength reports a string or binary input whose length is not
	// exactly StringLength or BinaryLength bytes respectively.
	ErrInvalidLength = errors.New("pikaid: invalid length")

	// ErrInvalidCharacter reports a byte outside the lowercase Base36
	// alphabet "0123456789abcdefghijklmnopqrstuvwxyz". Uppercase letters and
	// any other byte, including UTF-8 continuation bytes, are rejected.
	ErrInvalidCharacter = errors.New("pikaid: invalid character")

	// ErrTimestampOutOfRange reports a timestamp outside 0..MaxTimestamp,
	// whether supplied for generation or decoded from a 17-byte binary value.
	ErrTimestampOutOfRange = errors.New("pikaid: timestamp out of range")

	// ErrRandomnessOverflow reports a randomness suffix whose Base36 value
	// exceeds 2^96-1 and therefore cannot be represented by 12 bytes.
	ErrRandomnessOverflow = errors.New("pikaid: randomness exceeds 96 bits")

	// ErrRandomSource reports that the cryptographic random source failed or
	// returned fewer bytes than requested. Generation fails closed: no
	// identifier is returned.
	ErrRandomSource = errors.New("pikaid: cryptographic random source failed")
)
