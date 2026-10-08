package mailbox

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

func ptr[T any](value T) *T { return &value }

const testUUID = "abcdef01-1234-5678-9abc-abcdef012345"

func TestCanonicalPayloadAndMetadataPrecision(t *testing.T) {
	one := SendInput{ToAgentID: strings.ToUpper(testUUID), IdempotencyKey: "key", Text: "text", Metadata: map[string]any{"n": json.Number("9007199254740993"), "equivalent": json.Number("1.00e+0")}}
	two := SendInput{ToAgentID: testUUID, IdempotencyKey: "key", Text: "text", Kind: ptr("message"), Metadata: map[string]any{"equivalent": json.Number("1"), "n": json.Number("9007199254740993")}}
	_, raw, first, err := normalizeSend(one)
	if err != nil {
		t.Fatal(err)
	}
	_, _, second, err := normalizeSend(two)
	if err != nil || first != second {
		t.Fatal("equivalent payload conflicted")
	}
	if !strings.Contains(string(raw), "9007199254740993") {
		t.Fatal("number precision lost")
	}
	two.Text = "text "
	_, _, third, err := normalizeSend(two)
	if err != nil || third == first {
		t.Fatal("meaningful text difference lost")
	}
	for _, pair := range [][2]string{{"100", "1e2"}, {"0.0100", "1e-2"}, {"-0", "0"}, {"12.50e+001", "125"}, {"1e1000000000000000000000", "10e999999999999999999999"}} {
		if canonicalNumber(json.Number(pair[0])) != canonicalNumber(json.Number(pair[1])) {
			t.Fatalf("not equivalent: %v", pair)
		}
	}
}

func TestPayloadBoundaries(t *testing.T) {
	base := SendInput{ToAgentID: testUUID, IdempotencyKey: "key", Text: strings.Repeat("x", MaxTextBytes)}
	if _, _, _, err := normalizeSend(base); err != nil {
		t.Fatal("maximum text rejected")
	}
	for _, change := range []func(*SendInput){
		func(v *SendInput) { v.Text += "x" }, func(v *SendInput) { v.Text = "" }, func(v *SendInput) { v.Text = "a\x00b" }, func(v *SendInput) { v.Text = string([]byte{0xff}) },
		func(v *SendInput) { v.ToAgentID = "not-a-uuid" }, func(v *SendInput) { v.IdempotencyKey = "" }, func(v *SendInput) { v.IdempotencyKey = strings.Repeat("x", 129) },
		func(v *SendInput) { v.IdempotencyKey = "bad\nkey" }, func(v *SendInput) { v.Kind = ptr("bad kind") }, func(v *SendInput) { v.Metadata = map[string]any{"x": strings.Repeat("x", MaxMetadataBytes)} },
		func(v *SendInput) { v.ThreadID = ptr("bad") },
	} {
		v := base
		change(&v)
		if _, _, _, err := normalizeSend(v); err == nil {
			t.Fatal("invalid payload accepted")
		}
	}
	var value any = "value"
	for i := 0; i < MaxMetadataDepth; i++ {
		value = map[string]any{"nested": value}
	}
	base.Metadata = value.(map[string]any)
	if _, _, _, err := normalizeSend(base); err != nil {
		t.Fatal("valid depth rejected")
	}
	base.Metadata = map[string]any{"extra": value}
	if _, _, _, err := normalizeSend(base); err == nil {
		t.Fatal("excess depth accepted")
	}
}

func TestNormalizationDoesNotMutateSharedInputs(t *testing.T) {
	thread := strings.ToUpper(testUUID)
	input := SendInput{ToAgentID: testUUID, IdempotencyKey: "key", Text: "text", ThreadID: &thread}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, _, err := normalizeSend(input); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if thread != strings.ToUpper(testUUID) {
		t.Fatal("caller input changed")
	}
}
