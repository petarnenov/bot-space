package mailbox

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
)

type CursorScope struct {
	Purpose      string `json:"purpose"`
	Agent        string `json:"agent"`
	Workspace    string `json:"workspace"`
	Acknowledged string `json:"acknowledged"`
	Thread       string `json:"thread"`
	Kind         string `json:"kind"`
}
type Cursor struct {
	Version    int         `json:"v"`
	Scope      CursorScope `json:"scope"`
	Last       int64       `json:"last"`
	Upper      int64       `json:"upper"`
	AfterAgent string      `json:"after_agent,omitempty"`
}
type Signer struct{ key []byte }

func NewSigner(key []byte) (*Signer, error) {
	if len(key) != 32 {
		return nil, ErrInvalid
	}
	return &Signer{key: append([]byte(nil), key...)}, nil
}

func (s *Signer) Sign(cursor Cursor) (string, error) {
	cursor.Version = 1
	body, err := Encode(cursor)
	if err != nil {
		return "", ErrInvalid
	}
	mac := hmac.New(sha256.New, s.key)
	_, _ = mac.Write([]byte("bot-space-cursor-v1\x00"))
	_, _ = mac.Write(body)
	out := base64.RawURLEncoding.EncodeToString(body) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if len(out) > MaxCursorBytes {
		return "", ErrInvalid
	}
	return out, nil
}

func (s *Signer) Parse(value string, scope CursorScope) (Cursor, error) {
	if len(value) > MaxCursorBytes {
		return Cursor{}, ErrInvalid
	}
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return Cursor{}, ErrInvalid
	}
	body, err := base64.RawURLEncoding.Strict().DecodeString(parts[0])
	if err != nil {
		return Cursor{}, ErrInvalid
	}
	signature, err := base64.RawURLEncoding.Strict().DecodeString(parts[1])
	if err != nil {
		return Cursor{}, ErrInvalid
	}
	mac := hmac.New(sha256.New, s.key)
	_, _ = mac.Write([]byte("bot-space-cursor-v1\x00"))
	_, _ = mac.Write(body)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return Cursor{}, ErrInvalid
	}
	var cursor Cursor
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&cursor) != nil || cursor.Version != 1 || cursor.Scope != scope || cursor.Last < 0 || cursor.Upper < cursor.Last {
		return Cursor{}, ErrInvalid
	}
	return cursor, nil
}
