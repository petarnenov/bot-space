package config

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"github.com/petarnenov/bot-space/internal/githubapp"
	"net"
	"strconv"
	"strings"
)

// RunnerIdentity enables automatic machine enrollment separately from the
// unfinished orchestration execution runtime. Secrets remain server-side.
type RunnerIdentity struct {
	Enabled         bool
	AppTokens       *githubapp.Tokens
	ControlEndpoint string
	ControlCA       string
}

func LoadRunnerIdentity(getenv func(string) string) (RunnerIdentity, error) {
	var c RunnerIdentity
	switch getenv("RUNNER_IDENTITY_ENABLED") {
	case "", "false":
		return c, nil
	case "true":
		c.Enabled = true
	default:
		return c, errors.New("RUNNER_IDENTITY_ENABLED must be true or false")
	}
	installation, err := strconv.ParseInt(getenv("GITHUB_APP_INSTALLATION_ID"), 10, 64)
	if err != nil {
		return RunnerIdentity{}, errors.New("valid GITHUB_APP_INSTALLATION_ID is required")
	}
	c.AppTokens, err = githubapp.New(getenv("GITHUB_APP_CLIENT_ID"), installation, []byte(getenv("GITHUB_APP_PRIVATE_KEY")))
	if err != nil {
		return RunnerIdentity{}, errors.New("complete GitHub App verification configuration is required")
	}
	c.ControlEndpoint = getenv("CONTROL_ENDPOINT")
	host, port, err := net.SplitHostPort(c.ControlEndpoint)
	n, e := strconv.Atoi(port)
	if err != nil || host == "" || strings.ContainsAny(host, "/\\ \t\r\n") || e != nil || n < 1 || n > 65535 {
		return RunnerIdentity{}, errors.New("CONTROL_ENDPOINT must be a native TLS host:port")
	}
	cert, key := getenv("CONTROL_TLS_CERT"), getenv("CONTROL_TLS_KEY")
	pair, err := tls.X509KeyPair([]byte(cert), []byte(key))
	if err != nil || len(cert) > 65536 {
		return RunnerIdentity{}, errors.New("valid control TLS configuration is required")
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil || leaf.VerifyHostname(host) != nil {
		return RunnerIdentity{}, errors.New("control certificate must match CONTROL_ENDPOINT")
	}
	c.ControlCA = cert
	return c, nil
}
