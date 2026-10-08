package mailbox

import (
	"bytes"
	"testing"
)

func TestCursorSigningScopeAndReconstruction(t *testing.T) {
	key := bytes.Repeat([]byte{1}, 32)
	signer, err := NewSigner(key)
	if err != nil {
		t.Fatal(err)
	}
	scope := CursorScope{Purpose: "inbox", Agent: testUUID, Workspace: "12345678-1234-1234-1234-123456789abc", Acknowledged: "unacknowledged"}
	value, err := signer.Sign(Cursor{Scope: scope, Last: 1, Upper: 10})
	if err != nil {
		t.Fatal(err)
	}
	recreated, _ := NewSigner(key)
	if cursor, err := recreated.Parse(value, scope); err != nil || cursor.Last != 1 || cursor.Upper != 10 {
		t.Fatal("cursor did not survive signer reconstruction")
	}
	for _, change := range []func(*CursorScope){func(v *CursorScope) { v.Agent = "other" }, func(v *CursorScope) { v.Workspace = "other" }, func(v *CursorScope) { v.Purpose = "agents" }, func(v *CursorScope) { v.Acknowledged = "all" }, func(v *CursorScope) { v.Kind = "reply" }} {
		other := scope
		change(&other)
		if _, err := signer.Parse(value, other); err == nil {
			t.Fatal("foreign cursor accepted")
		}
	}
	if _, err := signer.Parse(value[:len(value)-2]+"xx", scope); err == nil {
		t.Fatal("tampered cursor accepted")
	}
	wrong, _ := NewSigner(bytes.Repeat([]byte{2}, 32))
	if _, err := wrong.Parse(value, scope); err == nil {
		t.Fatal("wrong-key cursor accepted")
	}
}
