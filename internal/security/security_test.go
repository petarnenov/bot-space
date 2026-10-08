package security

import "testing"

func TestSafeReturn(t *testing.T) {
	for _, value := range []string{"https://evil.example", "//evil.example", "/\\evil", "/\nsecret", "/invites?token=secret", "/%2f%2fevil", "/%5cevil", "/%0asecret", "/path#secret"} {
		if SafeReturn(value) != "/" {
			t.Fatalf("unsafe return accepted: %q", value)
		}
	}
	if SafeReturn("/workspaces") != "/workspaces" {
		t.Fatal("safe return rejected")
	}
}

func TestSecrets(t *testing.T) {
	one, err := Secret()
	if err != nil {
		t.Fatal(err)
	}
	two, err := Secret()
	if err != nil {
		t.Fatal(err)
	}
	if len(one) != 43 || one == two || Equal(one, two) || !Equal(one, one) || Hash(one) == one || len(PKCE(one)) != 43 {
		t.Fatal("invalid secret properties")
	}
}
