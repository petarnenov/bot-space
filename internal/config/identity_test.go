package config

import "testing"

func TestIdentityConfig(t *testing.T) {
	for _, tc := range []struct {
		base          string
		valid, secure bool
	}{{"http://localhost:8080", true, false}, {"http://127.0.0.1:8080", true, false}, {"https://example.com", true, true}, {"http://example.com", false, false}, {"https://user:secret@example.com", false, false}, {"https://example.com/path", false, false}, {"https://example.com?x=secret", false, false}, {"http://localhost:70000", false, false}} {
		t.Run(tc.base, func(t *testing.T) {
			env := map[string]string{"PUBLIC_BASE_URL": tc.base, "GITHUB_CLIENT_ID": "placeholder", "GITHUB_CLIENT_SECRET": "placeholder"}
			c, err := LoadIdentity(func(k string) string { return env[k] })
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
			if tc.valid && (!c.Enabled || c.SecureCookies != tc.secure) {
				t.Fatal("invalid cookie mode")
			}
		})
	}
	if c, err := LoadIdentity(func(string) string { return "" }); err != nil || c.Enabled {
		t.Fatal("absent identity config not disabled")
	}
	if _, err := LoadIdentity(func(k string) string {
		if k == "GITHUB_CLIENT_ID" {
			return "only-one"
		}
		return ""
	}); err == nil {
		t.Fatal("partial identity config accepted")
	}
}
