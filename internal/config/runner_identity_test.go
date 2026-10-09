package config

import (
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRunnerIdentityConfiguration(t *testing.T) {
	source := httptest.NewTLSServer(http.NotFoundHandler())
	pair := source.TLS.Certificates[0]
	source.Close()
	cert := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: pair.Certificate[0]}))
	key, err := x509.MarshalPKCS8PrivateKey(pair.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]string{"RUNNER_IDENTITY_ENABLED": "true", "GITHUB_REPOSITORY_TOKEN": "synthetic-token", "CONTROL_ENDPOINT": "127.0.0.1:9090", "CONTROL_TLS_CERT": cert, "CONTROL_TLS_KEY": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}))}
	get := func(name string) string { return values[name] }
	c, err := LoadRunnerIdentity(get)
	if err != nil || !c.Enabled || c.ControlCA != cert {
		t.Fatal("valid identity configuration rejected", err)
	}
	for _, name := range []string{"GITHUB_REPOSITORY_TOKEN", "CONTROL_ENDPOINT", "CONTROL_TLS_CERT", "CONTROL_TLS_KEY"} {
		old := values[name]
		values[name] = ""
		if _, err := LoadRunnerIdentity(get); err == nil {
			t.Fatal("missing configuration accepted", name)
		}
		values[name] = old
	}
	values["CONTROL_ENDPOINT"] = "attacker.example:9090"
	if _, err := LoadRunnerIdentity(get); err == nil {
		t.Fatal("hostname mismatch accepted")
	}
	values["RUNNER_IDENTITY_ENABLED"] = "false"
	if c, err := LoadRunnerIdentity(get); err != nil || c.Enabled {
		t.Fatal("disabled configuration altered legacy startup")
	}
	values["RUNNER_IDENTITY_ENABLED"] = "yes"
	if _, err := LoadRunnerIdentity(get); err == nil {
		t.Fatal("invalid flag accepted")
	}
}
