package runneridentity

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"

	"github.com/petarnenov/bot-space/internal/security"
)

const project = "11111111-1111-4111-8111-111111111111"

func TestStartProofBindsProjectRoleAndNonce(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	nonce, err := security.Secret()
	if err != nil {
		t.Fatal(err)
	}
	message, err := StartMessage(project, Executor, nonce)
	if err != nil {
		t.Fatal(err)
	}
	proof := StartProof{ProjectID: project, Role: Executor, Nonce: nonce, PublicKey: public, Signature: ed25519.Sign(private, message)}
	if err = VerifyStart(proof); err != nil {
		t.Fatal(err)
	}
	swapped := proof
	swapped.Role = Architect
	if !errors.Is(VerifyStart(swapped), ErrUnauthenticated) {
		t.Fatal("role swap accepted")
	}
	swapped = proof
	swapped.ProjectID = "22222222-2222-4222-8222-222222222222"
	if !errors.Is(VerifyStart(swapped), ErrUnauthenticated) {
		t.Fatal("project swap accepted")
	}
	swapped = proof
	swapped.Nonce, _ = security.Secret()
	if !errors.Is(VerifyStart(swapped), ErrUnauthenticated) {
		t.Fatal("nonce swap accepted")
	}
	swapped = proof
	swapped.Role = "administrator"
	if !errors.Is(VerifyStart(swapped), ErrInvalid) {
		t.Fatal("unknown role accepted")
	}
}
func TestChallengeDomainAndResourceBinding(t *testing.T) {
	public, private, _ := ed25519.GenerateKey(rand.Reader)
	nonce, _ := security.Secret()
	message, err := ChallengeMessage(project, "claim", nonce)
	if err != nil {
		t.Fatal(err)
	}
	signature := ed25519.Sign(private, message)
	if err = VerifyChallenge(public, project, "claim", nonce, signature); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(VerifyChallenge(public, project, "refresh", nonce, signature), ErrUnauthenticated) {
		t.Fatal("claim signature reused for refresh")
	}
	other := "22222222-2222-4222-8222-222222222222"
	if !errors.Is(VerifyChallenge(public, other, "claim", nonce, signature), ErrUnauthenticated) {
		t.Fatal("challenge used on another runner")
	}
	for _, bad := range []string{"", nonce + "x", nonce[:42] + "\n"} {
		if _, err = ChallengeMessage(project, "claim", bad); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid challenge admitted")
		}
	}
}
