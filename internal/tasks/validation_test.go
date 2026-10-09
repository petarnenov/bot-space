package tasks

import (
	"strings"
	"testing"
)

func TestSubmissionBoundsAndCanonicalDefaults(t *testing.T) {
	base := SubmitInput{ToAgentID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", IdempotencyKey: "key", Instruction: "synthetic"}
	_, hash, err := normalize(base)
	if err != nil {
		t.Fatal(err)
	}
	explicit := base
	explicit.TimeoutSeconds = DefaultTimeoutSeconds
	explicit.ToAgentID = strings.ToUpper(base.ToAgentID)
	_, same, err := normalize(explicit)
	if err != nil || same != hash {
		t.Fatal("equivalent UUID/default timeout changed key fingerprint")
	}
	for _, change := range []func(*SubmitInput){
		func(x *SubmitInput) { x.Instruction = "" },
		func(x *SubmitInput) { x.Instruction = strings.Repeat("x", MaxInstructionBytes+1) },
		func(x *SubmitInput) { x.Instruction = "bad\x00text" },
		func(x *SubmitInput) { x.ToAgentID = "invalid" },
		func(x *SubmitInput) { x.IdempotencyKey = "bad\nkey" },
		func(x *SubmitInput) { x.TimeoutSeconds = MaxTimeoutSeconds + 1 },
		func(x *SubmitInput) { x.TimeoutSeconds = -1 },
		func(x *SubmitInput) { x.ParentGeneration = 1 },
		func(x *SubmitInput) { id := base.ToAgentID; x.ParentTaskID = &id },
	} {
		x := base
		change(&x)
		if _, _, err := normalize(x); err != ErrInvalid {
			t.Fatal("invalid submission was not rejected")
		}
	}
}
