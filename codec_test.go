package pikaid_test

import (
	"encoding"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	pikaid "github.com/pikaid/pikaid-go"
)

// Compile-time contract: the identifier interoperates with the standard
// encoding interfaces and is a comparable value type.
var (
	_ encoding.TextMarshaler     = pikaid.ID{}
	_ encoding.TextUnmarshaler   = (*pikaid.ID)(nil)
	_ encoding.BinaryMarshaler   = pikaid.ID{}
	_ encoding.BinaryUnmarshaler = (*pikaid.ID)(nil)

	_ = map[pikaid.ID]struct{}{}
)

const codecID = "0s44we8002loz3po6mcf6vsn23"

func TestJSONRoundTrip(t *testing.T) {
	id, err := pikaid.Parse(codecID)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	encoded, err := json.Marshal(id)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if want := `"` + codecID + `"`; string(encoded) != want {
		t.Errorf("json.Marshal = %s, want %s", encoded, want)
	}

	var decoded pikaid.ID
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if decoded != id {
		t.Errorf("json round trip = %v, want %v", decoded, id)
	}

	// Struct fields and map keys use the same text form.
	payload, err := json.Marshal(struct {
		ID pikaid.ID `json:"id"`
	}{id})
	if err != nil {
		t.Fatalf("json.Marshal(struct): %v", err)
	}
	if want := `{"id":"` + codecID + `"}`; string(payload) != want {
		t.Errorf("json.Marshal(struct) = %s, want %s", payload, want)
	}

	keys, err := json.Marshal(map[pikaid.ID]int{id: 1})
	if err != nil {
		t.Fatalf("json.Marshal(map): %v", err)
	}
	if want := `{"` + codecID + `":1}`; string(keys) != want {
		t.Errorf("json.Marshal(map) = %s, want %s", keys, want)
	}
}

func TestJSONRejectsInvalidAndLeavesReceiverUnchanged(t *testing.T) {
	id, err := pikaid.Parse(codecID)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	upper := []byte(codecID)
	upper[7] = 'A'

	inputs := []string{
		`"` + codecID[:len(codecID)-1] + `"`,       // too short
		`"` + codecID + `0"`,                       // too long
		`"` + string(upper) + `"`,                  // uppercase
		`"0000000` + strings.Repeat("z", 19) + `"`, // randomness > 2^96-1
		`123`, // not a JSON string
	}

	for _, input := range inputs {
		decoded := id
		if err := json.Unmarshal([]byte(input), &decoded); err == nil {
			t.Errorf("json.Unmarshal(%s) returned no error", input)
		}
		if decoded != id {
			t.Errorf("json.Unmarshal(%s) modified the receiver: %v", input, decoded)
		}
	}
}

func TestFromBinaryCopiesInput(t *testing.T) {
	raw, err := hex.DecodeString("006553f10000112233445566778899aabb")
	if err != nil {
		t.Fatalf("bad fixture: %v", err)
	}

	id, err := pikaid.FromBinary(raw)
	if err != nil {
		t.Fatalf("FromBinary: %v", err)
	}
	want, err := pikaid.Parse(codecID)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if id != want {
		t.Fatalf("FromBinary = %v, want %v", id, want)
	}

	// The identifier is a value; later mutation of the input slice must not
	// change it.
	for i := range raw {
		raw[i] = 0xff
	}
	if id != want {
		t.Errorf("FromBinary retained a reference to its input: %v", id)
	}
	if id.Bytes() != want.Bytes() {
		t.Errorf("Bytes() = %x, want %x", id.Bytes(), want.Bytes())
	}
}
