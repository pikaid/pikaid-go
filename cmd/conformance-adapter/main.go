// Command conformance-adapter is a test-only JSON-Lines bridge used by the
// official cross-language PikaID conformance suite.
//
// It is not part of the public library API and is not a production service. It
// reads one UTF-8 JSON request object per line from standard input, writes
// exactly one JSON response object per request to standard output, and writes
// diagnostics only to standard error.
//
// Every operation delegates to the exported github.com/pikaid/pikaid-go API.
// The bridge performs no encoding, generation, validation or randomness
// production of its own; it only converts between the wire protocol and the
// library's types. See the pinned suite documentation at
// conformance/docs/JSONL_ADAPTER_PROTOCOL.md in pikaid/pikaid-specs.
package main

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pikaid/pikaid-go"
)

// protocolVersion is the only JSON-Lines adapter protocol version.
const protocolVersion = 1

// maxRequestLine bounds a single request line; the runner's own output limit
// is 1 MiB, and requests are far smaller than that.
const maxRequestLine = 1 << 20

// capabilities are advertised only for operations this bridge maps onto real
// exported library APIs.
//
// csprng_failure_injection is deliberately absent: the library validates
// entropy failures through internal test-only readers, and this bridge has no
// honest way to make every exported generation path fail without adding a
// production entropy hook. The P-GENERATE-RNG-FAILURE audit is therefore
// reported as not applicable by the runner and remains manual evidence.
var capabilities = []string{
	"generate_string",
	"text_to_binary",
	"binary_to_text",
	"boolean_validation",
	"format_only_validation",
	"parse_components",
	"parse_datetime",
	"generate_with_timestamp",
	"generate_binary",
	"generate_binary_with_timestamp",
	"public_compare",
}

// Error categories from the adapter protocol.
const (
	categoryInvalidInput       = "invalid_input"
	categoryEntropyUnavailable = "entropy_unavailable"
	categoryInternalError      = "internal_error"
	categoryUnsupported        = "unsupported_operation"
)

// request is one JSON-Lines request. RequestID is kept raw so that a JSON
// number is distinguished from a quoted string, and so that an integer ID is
// echoed exactly instead of being coerced through a float.
type request struct {
	RequestID json.RawMessage `json:"request_id"`
	Operation string          `json:"operation"`
	Arguments json.RawMessage `json:"arguments"`
}

type successResponse struct {
	RequestID json.RawMessage `json:"request_id"`
	OK        bool            `json:"ok"`
	Value     any             `json:"value"`
}

type errorResponse struct {
	RequestID json.RawMessage `json:"request_id"`
	OK        bool            `json:"ok"`
	Error     bridgeError     `json:"error"`
}

type bridgeError struct {
	Category string `json:"category"`
	Message  string `json:"message"`
}

type helloValue struct {
	ProtocolVersion int      `json:"protocol_version"`
	Capabilities    []string `json:"capabilities"`
}

type componentsValue struct {
	TimestampSeconds int64  `json:"timestamp_seconds"`
	RandomnessHex    string `json:"randomness_hex"`
	DatetimeUTC      string `json:"datetime_utc"`
}

func main() {
	if err := serve(os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "conformance-adapter:", err)
		os.Exit(1)
	}
}

// serve runs the request/response loop. Every scanned line produces exactly
// one newline-terminated JSON response, and every response is flushed before
// the next request is read.
func serve(in io.Reader, out io.Writer, diag io.Writer) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 64*1024), maxRequestLine)
	writer := bufio.NewWriter(out)
	for scanner.Scan() {
		line := scanner.Bytes()
		if _, err := writer.Write(respond(line)); err != nil {
			return fmt.Errorf("write response: %w", err)
		}
		if err := writer.WriteByte('\n'); err != nil {
			return fmt.Errorf("write response terminator: %w", err)
		}
		if err := writer.Flush(); err != nil {
			return fmt.Errorf("flush response: %w", err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read request: %w", err)
	}
	return nil
}

// respond decodes one request line and encodes exactly one response.
func respond(line []byte) []byte {
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		return marshalError(nil, invalidInput("malformed JSON request: "+err.Error()))
	}

	requestID, requestErr := integerRequestID(req.RequestID)
	if requestErr != nil {
		return marshalError(requestID, requestErr)
	}

	value, bridgeErr := dispatch(req)
	if bridgeErr != nil {
		return marshalError(requestID, bridgeErr)
	}
	return marshalSuccess(requestID, value)
}

// integerRequestID preserves an integer request ID exactly. A fractional,
// quoted, missing or out-of-range ID is rejected rather than coerced; the
// error response then carries a JSON null request_id.
func integerRequestID(raw json.RawMessage) (json.RawMessage, *bridgeError) {
	text, ok := integerToken(raw)
	if !ok {
		return json.RawMessage("null"), invalidInput("request_id must be an integer")
	}
	if _, err := strconv.ParseInt(text, 10, 64); err != nil {
		return json.RawMessage("null"), invalidInput("request_id must be a 64-bit integer: " + text)
	}
	return json.RawMessage(text), nil
}

// integerToken accepts exactly a JSON integer literal (an optional minus sign
// followed by decimal digits, without a leading zero). Quoted strings,
// fractions and exponents are rejected even though encoding/json would accept
// a quoted string in json.Number.
func integerToken(raw json.RawMessage) (string, bool) {
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return "", false
	}
	i := 0
	if text[0] == '-' {
		i++
	}
	if i >= len(text) {
		return "", false
	}
	if text[i] == '0' && len(text) > i+1 {
		return "", false
	}
	for ; i < len(text); i++ {
		if text[i] < '0' || text[i] > '9' {
			return "", false
		}
	}
	return text, true
}

func marshalSuccess(requestID json.RawMessage, value any) []byte {
	data, err := json.Marshal(successResponse{RequestID: requestID, OK: true, Value: value})
	if err != nil {
		return marshalError(requestID, internalError("encode response: "+err.Error()))
	}
	return data
}

func marshalError(requestID json.RawMessage, bridgeErr *bridgeError) []byte {
	data, err := json.Marshal(errorResponse{RequestID: requestID, OK: false, Error: *bridgeErr})
	if err != nil {
		// The error struct itself cannot fail to marshal; this guard keeps the
		// loop's "one response per line" contract even if that ever changes.
		return []byte(`{"request_id":null,"ok":false,"error":{"category":"internal_error","message":"cannot encode error response"}}`)
	}
	return data
}

// dispatch executes one operation and converts an unexpected panic into an
// internal_error, never into an input validation rejection.
func dispatch(req request) (value any, bridgeErr *bridgeError) {
	defer func() {
		if recovered := recover(); recovered != nil {
			value = nil
			bridgeErr = internalError(fmt.Sprintf("panic in %q: %v", req.Operation, recovered))
		}
	}()

	args, err := decodeArguments(req.Arguments)
	if err != nil {
		return nil, err
	}

	switch req.Operation {
	case "hello":
		return helloValue{ProtocolVersion: protocolVersion, Capabilities: capabilities}, nil

	case "generate_string":
		id, genErr := pikaid.Generate()
		if genErr != nil {
			return nil, classify(genErr)
		}
		return id.String(), nil

	case "text_to_binary":
		text, argErr := stringArgument(args, "text")
		if argErr != nil {
			return nil, argErr
		}
		id, parseErr := pikaid.Parse(text)
		if parseErr != nil {
			return nil, classify(parseErr)
		}
		binary := id.Bytes()
		return hex.EncodeToString(binary[:]), nil

	case "binary_to_text":
		text, argErr := stringArgument(args, "binary_hex")
		if argErr != nil {
			return nil, argErr
		}
		binary, decodeErr := decodeBinaryHex(text)
		if decodeErr != nil {
			return nil, decodeErr
		}
		id, fromErr := pikaid.FromBinary(binary)
		if fromErr != nil {
			return nil, classify(fromErr)
		}
		return id.String(), nil

	case "boolean_validation":
		text, argErr := stringArgument(args, "text")
		if argErr != nil {
			return nil, argErr
		}
		return pikaid.IsValid(text), nil

	case "format_only_validation":
		text, argErr := stringArgument(args, "text")
		if argErr != nil {
			return nil, argErr
		}
		return pikaid.IsWellFormed(text), nil

	case "parse_components":
		text, argErr := stringArgument(args, "text")
		if argErr != nil {
			return nil, argErr
		}
		id, parseErr := pikaid.Parse(text)
		if parseErr != nil {
			return nil, classify(parseErr)
		}
		return componentsValue{
			TimestampSeconds: id.Timestamp(),
			RandomnessHex:    id.RandomnessHex(),
			DatetimeUTC:      id.TimestampTime().UTC().Format(time.RFC3339),
		}, nil

	case "generate_with_timestamp":
		timestamp, argErr := timestampArgument(args)
		if argErr != nil {
			return nil, argErr
		}
		id, genErr := pikaid.GenerateWithTimestamp(timestamp)
		if genErr != nil {
			return nil, classify(genErr)
		}
		return id.String(), nil

	case "generate_binary":
		binary, genErr := pikaid.GenerateBinary()
		if genErr != nil {
			return nil, classify(genErr)
		}
		return hex.EncodeToString(binary[:]), nil

	case "generate_binary_with_timestamp":
		timestamp, argErr := timestampArgument(args)
		if argErr != nil {
			return nil, argErr
		}
		binary, genErr := pikaid.GenerateBinaryWithTimestamp(timestamp)
		if genErr != nil {
			return nil, classify(genErr)
		}
		return hex.EncodeToString(binary[:]), nil

	case "public_compare":
		first, argErr := stringArgument(args, "a")
		if argErr != nil {
			return nil, argErr
		}
		second, argErr := stringArgument(args, "b")
		if argErr != nil {
			return nil, argErr
		}
		a, parseErr := pikaid.Parse(first)
		if parseErr != nil {
			return nil, classify(parseErr)
		}
		b, parseErr := pikaid.Parse(second)
		if parseErr != nil {
			return nil, classify(parseErr)
		}
		return a.Compare(b), nil
	}

	return nil, &bridgeError{
		Category: categoryUnsupported,
		Message:  fmt.Sprintf("operation %q is not supported", req.Operation),
	}
}

func decodeArguments(raw json.RawMessage) (map[string]json.RawMessage, *bridgeError) {
	if len(raw) == 0 || string(raw) == "null" {
		return map[string]json.RawMessage{}, nil
	}
	var args map[string]json.RawMessage
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, invalidInput("arguments must be a JSON object: " + err.Error())
	}
	if args == nil {
		args = map[string]json.RawMessage{}
	}
	return args, nil
}

func stringArgument(args map[string]json.RawMessage, name string) (string, *bridgeError) {
	raw, ok := args[name]
	if !ok {
		return "", invalidInput(fmt.Sprintf("missing string argument %q", name))
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", invalidInput(fmt.Sprintf("argument %q must be a string", name))
	}
	return value, nil
}

// timestampArgument parses timestamp_seconds losslessly as a decimal integer.
// Fractions, exponents, strings and values outside int64 are rejected instead
// of being rounded or truncated.
func timestampArgument(args map[string]json.RawMessage) (int64, *bridgeError) {
	raw, ok := args["timestamp_seconds"]
	if !ok {
		return 0, invalidInput("missing argument \"timestamp_seconds\"")
	}
	text, ok := integerToken(raw)
	if !ok {
		return 0, invalidInput("timestamp_seconds must be an integer")
	}
	value, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0, invalidInput("timestamp_seconds must be a 64-bit integer: " + text)
	}
	return value, nil
}

// decodeBinaryHex accepts exactly 17 bytes encoded as 34 lowercase hex digits.
// Uppercase hex, odd lengths and out-of-range values are rejected by the
// library's own FromBinary check rather than normalized.
func decodeBinaryHex(text string) ([]byte, *bridgeError) {
	if len(text) != 2*pikaid.BinaryLength {
		return nil, invalidInput(fmt.Sprintf("binary_hex must be exactly %d hex digits, got %d",
			2*pikaid.BinaryLength, len(text)))
	}
	for i := 0; i < len(text); i++ {
		c := text[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return nil, invalidInput("binary_hex must be lowercase hexadecimal")
		}
	}
	binary, err := hex.DecodeString(text)
	if err != nil {
		return nil, invalidInput("binary_hex is not hexadecimal: " + err.Error())
	}
	return binary, nil
}

// classify maps documented library errors to protocol categories. Anything
// unexpected stays internal_error so a crash is never disguised as a
// deliberate input rejection.
func classify(err error) *bridgeError {
	switch {
	case errors.Is(err, pikaid.ErrInvalidLength),
		errors.Is(err, pikaid.ErrInvalidCharacter),
		errors.Is(err, pikaid.ErrTimestampOutOfRange),
		errors.Is(err, pikaid.ErrRandomnessOverflow):
		return invalidInput(err.Error())
	case errors.Is(err, pikaid.ErrRandomSource):
		return &bridgeError{Category: categoryEntropyUnavailable, Message: err.Error()}
	default:
		return internalError(err.Error())
	}
}

func invalidInput(message string) *bridgeError {
	return &bridgeError{Category: categoryInvalidInput, Message: message}
}

func internalError(message string) *bridgeError {
	return &bridgeError{Category: categoryInternalError, Message: message}
}
