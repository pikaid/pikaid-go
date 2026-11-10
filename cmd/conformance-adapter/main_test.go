package main

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/pikaid/pikaid-go"
)

// envelope is a decoded adapter response.
type envelope struct {
	RequestID json.RawMessage `json:"request_id"`
	OK        bool            `json:"ok"`
	Value     json.RawMessage `json:"value"`
	Error     *struct {
		Category string `json:"category"`
		Message  string `json:"message"`
	} `json:"error"`
}

// exchangeRaw feeds request lines through serve and returns exactly one raw
// response line per input line.
func exchangeRaw(t *testing.T, lines ...string) []string {
	t.Helper()
	var out, diag bytes.Buffer
	if err := serve(strings.NewReader(strings.Join(lines, "\n")+"\n"), &out, &diag); err != nil {
		t.Fatalf("serve: %v", err)
	}
	if diag.Len() != 0 {
		t.Fatalf("serve wrote diagnostics to stderr: %q", diag.String())
	}
	raw := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(raw) != len(lines) {
		t.Fatalf("got %d response lines, want %d: %q", len(raw), len(lines), out.String())
	}
	return raw
}

func decodeEnvelope(t *testing.T, line string) envelope {
	t.Helper()
	var resp envelope
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		t.Fatalf("response is not JSON: %v: %q", err, line)
	}
	return resp
}

func exchange(t *testing.T, lines ...string) []envelope {
	t.Helper()
	raw := exchangeRaw(t, lines...)
	decoded := make([]envelope, len(raw))
	for i, line := range raw {
		decoded[i] = decodeEnvelope(t, line)
	}
	return decoded
}

// exchangeOne sends one request built from op and arguments.
func exchangeOne(t *testing.T, id int, op string, args any) envelope {
	t.Helper()
	payload := map[string]any{"request_id": id, "operation": op}
	if args != nil {
		payload["arguments"] = args
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return exchange(t, string(encoded))[0]
}

// keys returns the set of top-level JSON keys in raw.
func keys(t *testing.T, raw []byte) map[string]bool {
	t.Helper()
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		t.Fatalf("decode object: %v: %s", err, raw)
	}
	set := make(map[string]bool, len(object))
	for key := range object {
		set[key] = true
	}
	return set
}

func requireKeys(t *testing.T, raw []byte, want ...string) {
	t.Helper()
	got := keys(t, raw)
	if len(got) != len(want) {
		t.Fatalf("keys = %v, want %v", got, want)
	}
	for _, key := range want {
		if !got[key] {
			t.Fatalf("missing key %q in %v", key, got)
		}
	}
}

func requireSuccess(t *testing.T, resp envelope) json.RawMessage {
	t.Helper()
	if !resp.OK {
		t.Fatalf("expected success, got error %+v", resp.Error)
	}
	if resp.Error != nil {
		t.Fatalf("success envelope must not contain error: %+v", resp.Error)
	}
	return resp.Value
}

func requireError(t *testing.T, resp envelope, category string) {
	t.Helper()
	if resp.OK {
		t.Fatalf("expected %s error, got success %s", category, resp.Value)
	}
	if resp.Error == nil {
		t.Fatal("failure envelope is missing error object")
	}
	if resp.Error.Category != category {
		t.Fatalf("category = %q, want %q (message %q)", resp.Error.Category, category, resp.Error.Message)
	}
	if resp.Error.Message == "" {
		t.Fatal("error message must be nonempty")
	}
}

func decodeValue[T any](t *testing.T, raw json.RawMessage) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("decode value: %v: %s", err, raw)
	}
	return value
}

func TestHelloAdvertisesOnlyImplementedCapabilities(t *testing.T) {
	raw := exchangeRaw(t, `{"request_id":1,"operation":"hello","arguments":{}}`)
	resp := decodeEnvelope(t, raw[0])
	value := requireSuccess(t, resp)
	requireKeys(t, []byte(raw[0]), "request_id", "ok", "value")
	requireKeys(t, value, "protocol_version", "capabilities")

	var hello struct {
		ProtocolVersion int      `json:"protocol_version"`
		Capabilities    []string `json:"capabilities"`
	}
	hello = decodeValue[struct {
		ProtocolVersion int      `json:"protocol_version"`
		Capabilities    []string `json:"capabilities"`
	}](t, value)

	if hello.ProtocolVersion != 1 {
		t.Errorf("protocol_version = %d, want 1", hello.ProtocolVersion)
	}
	got := make(map[string]bool)
	for _, capability := range hello.Capabilities {
		got[capability] = true
	}
	for _, required := range []string{"generate_string", "text_to_binary", "binary_to_text"} {
		if !got[required] {
			t.Errorf("missing mandatory capability %q", required)
		}
	}
	for _, optional := range []string{
		"boolean_validation",
		"format_only_validation",
		"parse_components",
		"parse_datetime",
		"generate_with_timestamp",
		"generate_binary",
		"generate_binary_with_timestamp",
		"public_compare",
	} {
		if !got[optional] {
			t.Errorf("missing implemented optional capability %q", optional)
		}
	}
	// The bridge has no honest hook for production CSPRNG failure injection and
	// must not claim it.
	if got["csprng_failure_injection"] {
		t.Error("csprng_failure_injection must not be advertised")
	}
}

func TestFramingPreservesEveryRequestID(t *testing.T) {
	responses := exchangeRaw(t,
		`{"request_id":1,"operation":"hello","arguments":{}}`,
		`{"request_id":0,"operation":"hello","arguments":{}}`,
		`{"request_id":9007199254740993,"operation":"hello","arguments":{}}`,
	)
	want := []string{"1", "0", "9007199254740993"}
	for i, line := range responses {
		resp := decodeEnvelope(t, line)
		if string(resp.RequestID) != want[i] {
			t.Errorf("response %d request_id = %s, want %s", i, resp.RequestID, want[i])
		}
		requireKeys(t, []byte(line), "request_id", "ok", "value")
	}
}

func TestMalformedRequestsProduceOneErrorEach(t *testing.T) {
	cases := []struct {
		name  string
		line  string
		want  string
		idRaw string
	}{
		{"invalid json", `{`, "invalid_input", "null"},
		{"empty line", ``, "invalid_input", "null"},
		{"missing request id", `{"operation":"hello","arguments":{}}`, "invalid_input", "null"},
		{"fractional id", `{"request_id":1.5,"operation":"hello","arguments":{}}`, "invalid_input", "null"},
		{"string id", `{"request_id":"7","operation":"hello","arguments":{}}`, "invalid_input", "null"},
		{"overflow id", `{"request_id":9223372036854775808,"operation":"hello","arguments":{}}`, "invalid_input", "null"},
		{"unknown operation", `{"request_id":3,"operation":"nope","arguments":{}}`, "unsupported_operation", "3"},
		{"missing operation", `{"request_id":4,"arguments":{}}`, "unsupported_operation", "4"},
	}
	lines := make([]string, len(cases))
	for i, tc := range cases {
		lines[i] = tc.line
	}
	raw := exchangeRaw(t, lines...)
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := decodeEnvelope(t, raw[i])
			requireError(t, resp, tc.want)
			requireKeys(t, []byte(raw[i]), "request_id", "ok", "error")
			requireKeys(t, mustJSON(t, resp.Error), "category", "message")
			if string(resp.RequestID) != tc.idRaw {
				t.Errorf("request_id = %s, want %s", resp.RequestID, tc.idRaw)
			}
		})
	}
}

// mustJSON encodes any value for key-set assertions.
func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return encoded
}

func TestGenerateString(t *testing.T) {
	before := time.Now().Unix()
	value := requireSuccess(t, exchangeOne(t, 10, "generate_string", map[string]any{}))
	after := time.Now().Unix()

	text := decodeValue[string](t, value)
	if len(text) != pikaid.StringLength {
		t.Fatalf("length = %d, want %d", len(text), pikaid.StringLength)
	}
	for _, c := range text {
		if !(c >= '0' && c <= '9') && !(c >= 'a' && c <= 'z') {
			t.Fatalf("non-canonical character %q in %q", c, text)
		}
	}
	id, err := pikaid.Parse(text)
	if err != nil {
		t.Fatalf("generated value does not parse: %v", err)
	}
	if ts := id.Timestamp(); ts < before || ts > after {
		t.Errorf("timestamp %d outside observed window %d..%d", ts, before, after)
	}
}

func TestTextToBinaryKnownVectors(t *testing.T) {
	cases := []struct {
		text string
		hex  string
	}{
		{"0s44we8002loz3po6mcf6vsn23", "006553f10000112233445566778899aabb"},
		{"00000000000000000000000000", "0000000000000000000000000000000000"},
		{"zzzzzzz7oiylpimjg5u2ca1ypr", "123ede3fffffffffffffffffffffffffff"},
	}
	for _, tc := range cases {
		t.Run(tc.text, func(t *testing.T) {
			value := requireSuccess(t, exchangeOne(t, 11, "text_to_binary", map[string]any{"text": tc.text}))
			hexText := decodeValue[string](t, value)
			if hexText != tc.hex {
				t.Errorf("binary_hex = %q, want %q", hexText, tc.hex)
			}
			if len(hexText) != 2*pikaid.BinaryLength {
				t.Errorf("binary_hex length = %d, want %d", len(hexText), 2*pikaid.BinaryLength)
			}
			if strings.ToLower(hexText) != hexText {
				t.Errorf("binary_hex must be lowercase: %q", hexText)
			}
		})
	}
}

func TestTextToBinaryRejectsInvalidInput(t *testing.T) {
	invalid := []string{
		"",
		"0000000000000000000000000",
		"000000000000000000000000000",
		"0S44WE8002LOZ3PO6MCF6VSN23",
		"0s44we8002loz3po6mcf6vsn2!",
		"0000000zzzzzzzzzzzzzzzzzzz", // randomness exceeds 96 bits
		"😀000000000000000000000000000",
	}
	for _, text := range invalid {
		t.Run(text, func(t *testing.T) {
			requireError(t, exchangeOne(t, 12, "text_to_binary", map[string]any{"text": text}), "invalid_input")
		})
	}
}

func TestBinaryToTextRoundTrip(t *testing.T) {
	cases := []struct {
		hex  string
		text string
	}{
		{"006553f10000112233445566778899aabb", "0s44we8002loz3po6mcf6vsn23"},
		{"0000000000000000000000000000000000", "00000000000000000000000000"},
		{"123ede3fffffffffffffffffffffffffff", "zzzzzzz7oiylpimjg5u2ca1ypr"},
	}
	for _, tc := range cases {
		t.Run(tc.hex, func(t *testing.T) {
			value := requireSuccess(t, exchangeOne(t, 13, "binary_to_text", map[string]any{"binary_hex": tc.hex}))
			text := decodeValue[string](t, value)
			if text != tc.text {
				t.Errorf("text = %q, want %q", text, tc.text)
			}
		})
	}
}

func TestBinaryToTextRejectsInvalidInput(t *testing.T) {
	invalid := []string{
		"",
		"00",
		"000000000000000000000000000000000",
		"00000000000000000000000000000000000",
		"006553F10000112233445566778899AABB", // uppercase
		"006553f10000112233445566778899aabg", // non-hex
		"123ede3ffffffffffffffffffffffffff",  // odd length
	}
	for _, hexText := range invalid {
		t.Run(hexText, func(t *testing.T) {
			requireError(t, exchangeOne(t, 14, "binary_to_text", map[string]any{"binary_hex": hexText}), "invalid_input")
		})
	}
}

func TestBooleanAndFormatOnlyValidation(t *testing.T) {
	wellFormedButOversized := "0000000zzzzzzzzzzzzzzzzzzz"
	cases := []struct {
		text       string
		valid      bool
		wellFormed bool
	}{
		{"00000000000000000000000000", true, true},
		{"zzzzzzz7oiylpimjg5u2ca1ypr", true, true},
		{wellFormedButOversized, false, true},
		{"000000A0000000000000000000", false, false},
		{"", false, false},
		{"0000000000000000000000000", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.text, func(t *testing.T) {
			valid := requireSuccess(t, exchangeOne(t, 15, "boolean_validation", map[string]any{"text": tc.text}))
			if got := decodeValue[bool](t, valid); got != tc.valid {
				t.Errorf("boolean_validation = %v, want %v", got, tc.valid)
			}
			wellFormed := requireSuccess(t, exchangeOne(t, 16, "format_only_validation", map[string]any{"text": tc.text}))
			if got := decodeValue[bool](t, wellFormed); got != tc.wellFormed {
				t.Errorf("format_only_validation = %v, want %v", got, tc.wellFormed)
			}
		})
	}
}

func TestParseComponents(t *testing.T) {
	cases := []struct {
		text      string
		timestamp int64
		randomHex string
		datetime  string
	}{
		{"0s44we8002loz3po6mcf6vsn23", 1_700_000_000, "00112233445566778899aabb", "2023-11-14T22:13:20Z"},
		{"00000000000000000000000000", 0, "000000000000000000000000", "1970-01-01T00:00:00Z"},
		{"zzzzzzz7oiylpimjg5u2ca1ypr", pikaid.MaxTimestamp, "ffffffffffffffffffffffff", "4453-04-05T15:21:35Z"},
	}
	for _, tc := range cases {
		t.Run(tc.text, func(t *testing.T) {
			value := requireSuccess(t, exchangeOne(t, 17, "parse_components", map[string]any{"text": tc.text}))
			requireKeys(t, value, "timestamp_seconds", "randomness_hex", "datetime_utc")
			components := decodeValue[struct {
				TimestampSeconds int64  `json:"timestamp_seconds"`
				RandomnessHex    string `json:"randomness_hex"`
				DatetimeUTC      string `json:"datetime_utc"`
			}](t, value)
			if components.TimestampSeconds != tc.timestamp {
				t.Errorf("timestamp_seconds = %d, want %d", components.TimestampSeconds, tc.timestamp)
			}
			if components.RandomnessHex != tc.randomHex {
				t.Errorf("randomness_hex = %q, want %q", components.RandomnessHex, tc.randomHex)
			}
			if len(components.RandomnessHex) != 24 {
				t.Errorf("randomness_hex length = %d, want 24", len(components.RandomnessHex))
			}
			if components.RandomnessHex != strings.ToLower(components.RandomnessHex) {
				t.Errorf("randomness_hex must be lowercase: %q", components.RandomnessHex)
			}
			if components.DatetimeUTC != tc.datetime {
				t.Errorf("datetime_utc = %q, want %q", components.DatetimeUTC, tc.datetime)
			}
		})
	}
}

func TestGenerateWithTimestampBoundaries(t *testing.T) {
	for _, timestamp := range []int64{0, 1, pikaid.MaxTimestamp} {
		t.Run(fmt.Sprint(timestamp), func(t *testing.T) {
			value := requireSuccess(t, exchangeOne(t, 18, "generate_with_timestamp", map[string]any{"timestamp_seconds": timestamp}))
			text := decodeValue[string](t, value)
			id, err := pikaid.Parse(text)
			if err != nil {
				t.Fatalf("generated value does not parse: %v", err)
			}
			if id.Timestamp() != timestamp {
				t.Errorf("timestamp = %d, want %d", id.Timestamp(), timestamp)
			}
		})
	}
	for _, timestamp := range []json.Number{"-1", "78364164096"} {
		t.Run("reject "+timestamp.String(), func(t *testing.T) {
			requireError(t, exchangeOne(t, 19, "generate_with_timestamp", map[string]any{"timestamp_seconds": timestamp}), "invalid_input")
		})
	}
}

func TestTimestampArgumentIsStrict(t *testing.T) {
	cases := []any{
		json.Number("1.5"),
		json.Number("1e3"),
		"1700000000",
		json.Number("17000000000000000000000"),
		true,
	}
	for _, argument := range cases {
		t.Run(fmt.Sprint(argument), func(t *testing.T) {
			requireError(t, exchangeOne(t, 20, "generate_with_timestamp", map[string]any{"timestamp_seconds": argument}), "invalid_input")
		})
	}
	requireError(t, exchangeOne(t, 21, "generate_with_timestamp", map[string]any{}), "invalid_input")
}

func TestGenerateBinaryAndBinaryWithTimestamp(t *testing.T) {
	value := requireSuccess(t, exchangeOne(t, 22, "generate_binary", map[string]any{}))
	hexText := decodeValue[string](t, value)
	if len(hexText) != 2*pikaid.BinaryLength || strings.ToLower(hexText) != hexText {
		t.Fatalf("generate_binary value = %q, want %d lowercase hex digits", hexText, 2*pikaid.BinaryLength)
	}
	binary, err := hex.DecodeString(hexText)
	if err != nil {
		t.Fatalf("generate_binary is not hex: %v", err)
	}
	before := time.Now().Unix()
	id, err := pikaid.FromBinary(binary)
	if err != nil {
		t.Fatalf("generate_binary output does not parse: %v", err)
	}
	if id.Timestamp() < before-2 || id.Timestamp() > time.Now().Unix()+1 {
		t.Errorf("timestamp %d not near now", id.Timestamp())
	}

	value = requireSuccess(t, exchangeOne(t, 23, "generate_binary_with_timestamp", map[string]any{"timestamp_seconds": 42}))
	hexText = decodeValue[string](t, value)
	if len(hexText) != 2*pikaid.BinaryLength || !strings.HasPrefix(hexText, "000000002a") {
		t.Errorf("generate_binary_with_timestamp value = %q, want 5-byte timestamp prefix 000000002a", hexText)
	}
}

func TestPublicCompare(t *testing.T) {
	low := "00000000000000000000000000"
	high := "zzzzzzz7oiylpimjg5u2ca1ypr"
	cases := []struct {
		a, b string
		want int
	}{
		{low, high, -1},
		{high, low, 1},
		{low, low, 0},
		{"0s44we8002loz3po6mcf6vsn23", "0s44we8002loz3po6mcf6vsn23", 0},
	}
	for _, tc := range cases {
		t.Run(tc.a+"|"+tc.b, func(t *testing.T) {
			value := requireSuccess(t, exchangeOne(t, 24, "public_compare", map[string]any{"a": tc.a, "b": tc.b}))
			if got := decodeValue[int](t, value); got != tc.want {
				t.Errorf("public_compare = %d, want %d", got, tc.want)
			}
		})
	}
	requireError(t, exchangeOne(t, 25, "public_compare", map[string]any{"a": "bad", "b": low}), "invalid_input")
	requireError(t, exchangeOne(t, 26, "public_compare", map[string]any{"a": low}), "invalid_input")
}

func TestArgumentShapeValidation(t *testing.T) {
	cases := []struct {
		name string
		op   string
		args any
	}{
		{"missing text", "text_to_binary", map[string]any{}},
		{"non-string text", "text_to_binary", map[string]any{"text": 12}},
		{"null text", "text_to_binary", map[string]any{"text": nil}},
		{"missing binary_hex", "binary_to_text", map[string]any{}},
		{"non-string binary_hex", "binary_to_text", map[string]any{"binary_hex": []string{}}},
		{"array arguments", "text_to_binary", []string{"x"}},
		{"string arguments", "text_to_binary", "x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requireError(t, exchangeOne(t, 30, tc.op, tc.args), "invalid_input")
		})
	}
}

func TestUnsupportedOperations(t *testing.T) {
	for _, op := range []string{"inject_fault", "unknown_operation", ""} {
		t.Run(op, func(t *testing.T) {
			resp := exchangeOne(t, 31, op, map[string]any{})
			requireError(t, resp, "unsupported_operation")
		})
	}
}

func TestErrorClassification(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		category string
	}{
		{"invalid length", pikaid.ErrInvalidLength, "invalid_input"},
		{"invalid character", pikaid.ErrInvalidCharacter, "invalid_input"},
		{"timestamp out of range", pikaid.ErrTimestampOutOfRange, "invalid_input"},
		{"randomness overflow", pikaid.ErrRandomnessOverflow, "invalid_input"},
		{"random source", pikaid.ErrRandomSource, "entropy_unavailable"},
		{"unexpected", errors.New("boom"), "internal_error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bridgeErr := classify(fmt.Errorf("wrapped: %w", tc.err))
			if bridgeErr.Category != tc.category {
				t.Errorf("category = %q, want %q", bridgeErr.Category, tc.category)
			}
			if bridgeErr.Message == "" {
				t.Error("message must be nonempty")
			}
		})
	}
}

func TestResponseFlushesPerRequest(t *testing.T) {
	// Two pipes make the reader/writer loop observable: each response must be
	// written before the next request is consumed.
	inPR, inPW := io.Pipe()
	outPR, outPW := io.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- serve(inPR, outPW, &bytes.Buffer{})
		outPW.Close()
	}()

	reader := bufio.NewReader(outPR)
	requests := []string{
		`{"request_id":1,"operation":"hello","arguments":{}}`,
		`{"request_id":2,"operation":"generate_string","arguments":{}}`,
	}
	for _, request := range requests {
		if _, err := io.WriteString(inPW, request+"\n"); err != nil {
			t.Fatalf("write request: %v", err)
		}
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read response: %v", err)
		}
		resp := decodeEnvelope(t, strings.TrimSuffix(line, "\n"))
		if !resp.OK {
			t.Fatalf("unexpected error response: %+v", resp.Error)
		}
	}
	if err := inPW.Close(); err != nil {
		t.Fatalf("close input: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("serve: %v", err)
	}
}
