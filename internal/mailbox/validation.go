package mailbox

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/petarnenov/bot-space/internal/security"
)

var kindPattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,64}$`)

func Encode(value any) ([]byte, error) {
	var b bytes.Buffer
	encoder := json.NewEncoder(&b)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, ErrInvalid
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}

func pageLimit(limit *int) (int, error) {
	if limit == nil {
		return DefaultPageSize, nil
	}
	if *limit < 1 || *limit > MaxPageSize {
		return 0, ErrInvalid
	}
	return *limit, nil
}

func normalizeID(value *string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	if !security.ValidUUID(*value) {
		return nil, ErrInvalid
	}
	normalized := strings.ToLower(*value)
	return &normalized, nil
}

func normalizeSend(input SendInput) (SendInput, []byte, string, error) {
	if !security.ValidUUID(input.ToAgentID) || !utf8.ValidString(input.Text) || len(input.Text) < 1 || len(input.Text) > MaxTextBytes || strings.ContainsRune(input.Text, 0) {
		return SendInput{}, nil, "", ErrInvalid
	}
	input.ToAgentID = strings.ToLower(input.ToAgentID)
	if len(input.IdempotencyKey) < 1 || len(input.IdempotencyKey) > 128 {
		return SendInput{}, nil, "", ErrInvalid
	}
	for _, c := range []byte(input.IdempotencyKey) {
		if c < 32 || c > 126 {
			return SendInput{}, nil, "", ErrInvalid
		}
	}
	kind := "message"
	if input.Kind != nil {
		kind = *input.Kind
	}
	if !kindPattern.MatchString(kind) {
		return SendInput{}, nil, "", ErrInvalid
	}
	input.Kind = &kind
	var idErr error
	input.ThreadID, idErr = normalizeID(input.ThreadID)
	if idErr != nil {
		return SendInput{}, nil, "", ErrInvalid
	}
	input.InReplyTo, idErr = normalizeID(input.InReplyTo)
	if idErr != nil {
		return SendInput{}, nil, "", ErrInvalid
	}
	if input.Metadata == nil {
		input.Metadata = map[string]any{}
	}
	metadata, err := Encode(input.Metadata)
	if err != nil || len(metadata) > MaxMetadataBytes {
		return SendInput{}, nil, "", ErrInvalid
	}
	var parsed map[string]any
	decoder := json.NewDecoder(bytes.NewReader(metadata))
	decoder.UseNumber()
	if decoder.Decode(&parsed) != nil || !validDepth(parsed, 1) {
		return SendInput{}, nil, "", ErrInvalid
	}
	input.Metadata = parsed
	canonical := canonicalValue(parsed)
	payload := struct {
		To            string
		Kind          string
		Text          string
		Metadata      any
		Thread, Reply *string
	}{input.ToAgentID, kind, input.Text, canonical, input.ThreadID, input.InReplyTo}
	encoded, err := Encode(payload)
	if err != nil {
		return SendInput{}, nil, "", ErrInvalid
	}
	hash := sha256.Sum256(encoded)
	return input, metadata, hex.EncodeToString(hash[:]), nil
}

func validDepth(value any, depth int) bool {
	switch v := value.(type) {
	case map[string]any:
		if depth > MaxMetadataDepth {
			return false
		}
		for _, child := range v {
			if !validDepth(child, depth+1) {
				return false
			}
		}
	case []any:
		if depth > MaxMetadataDepth {
			return false
		}
		for _, child := range v {
			if !validDepth(child, depth+1) {
				return false
			}
		}
	}
	return true
}

func canonicalValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := map[string]any{}
		for key, child := range v {
			out[key] = canonicalValue(child)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, child := range v {
			out[i] = canonicalValue(child)
		}
		return out
	case json.Number:
		return canonicalNumber(v)
	default:
		return v
	}
}

func canonicalNumber(value json.Number) json.Number {
	s := value.String()
	negative := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	exponent := new(big.Int)
	if pos := strings.IndexAny(s, "eE"); pos >= 0 {
		exponent.SetString(s[pos+1:], 10)
		s = s[:pos]
	}
	fraction := 0
	if pos := strings.IndexByte(s, '.'); pos >= 0 {
		fraction = len(s) - pos - 1
		s = s[:pos] + s[pos+1:]
	}
	digits := strings.TrimLeft(s, "0")
	if digits == "" {
		return json.Number("0")
	}
	trimmed := strings.TrimRight(digits, "0")
	shift := len(digits) - len(trimmed) - fraction
	exponent.Add(exponent, big.NewInt(int64(shift)))
	sign := ""
	if negative {
		sign = "-"
	}
	return json.Number(sign + trimmed + "e" + exponent.String())
}
