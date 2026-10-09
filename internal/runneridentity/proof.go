// Package runneridentity binds GitHub-backed runner enrollment to machine keys.
package runneridentity

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"strings"

	"github.com/petarnenov/bot-space/internal/security"
)

var ErrInvalid = errors.New("invalid runner identity request")
var ErrUnauthenticated = errors.New("runner key proof rejected")
var ErrUnavailable = errors.New("runner identity unavailable")

type Role string

const (
	Architect Role = "architect"
	Executor  Role = "executor"
)

func validRole(role Role) bool { return role == Architect || role == Executor }

// StartProof is signed by the newly generated machine key. Identity is still
// unverified until the server binds an actual GitHub OAuth callback.
type StartProof struct {
	ProjectID string `json:"project_id"`
	Role      Role   `json:"role"`
	Nonce     string `json:"nonce"`
	PublicKey []byte `json:"public_key"`
	Signature []byte `json:"signature"`
}

func StartMessage(projectID string, role Role, nonce string) ([]byte, error) {
	if !security.ValidUUID(projectID) || !validRole(role) || !validNonce(nonce) {
		return nil, ErrInvalid
	}
	body, _ := json.Marshal(struct {
		Project string
		Role    Role
		Nonce   string
	}{strings.ToLower(projectID), role, nonce})
	return append([]byte("bot-space/runner-start/v1\n"), body...), nil
}
func VerifyStart(p StartProof) error {
	message, err := StartMessage(p.ProjectID, p.Role, p.Nonce)
	if err != nil {
		return err
	}
	if len(p.PublicKey) != ed25519.PublicKeySize || !ed25519.Verify(ed25519.PublicKey(p.PublicKey), message, p.Signature) {
		return ErrUnauthenticated
	}
	return nil
}

// ChallengeMessage binds a one-use server challenge to resource and operation.
// The store, not this stateless helper, enforces challenge expiry/consumption.
func ChallengeMessage(resourceID, purpose, nonce string) ([]byte, error) {
	if !security.ValidUUID(resourceID) || (purpose != "claim" && purpose != "refresh") || !validNonce(nonce) {
		return nil, ErrInvalid
	}
	return []byte("bot-space/runner-challenge/v1\n" + strings.ToLower(resourceID) + "\n" + purpose + "\n" + nonce), nil
}
func validNonce(value string) bool {
	if len(value) != 43 {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}
func VerifyChallenge(key []byte, resourceID, purpose, nonce string, signature []byte) error {
	message, err := ChallengeMessage(resourceID, purpose, nonce)
	if err != nil {
		return err
	}
	if len(key) != ed25519.PublicKeySize || !ed25519.Verify(ed25519.PublicKey(key), message, signature) {
		return ErrUnauthenticated
	}
	return nil
}
