package config

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
)

type Identity struct {
	ClientID      string
	ClientSecret  string
	BaseURL       string
	SecureCookies bool
	Enabled       bool
}

func LoadIdentity(getenv func(string) string) (Identity, error) {
	c := Identity{ClientID: getenv("GITHUB_CLIENT_ID"), ClientSecret: getenv("GITHUB_CLIENT_SECRET"), BaseURL: strings.TrimRight(getenv("PUBLIC_BASE_URL"), "/")}
	if c.ClientID == "" && c.ClientSecret == "" && c.BaseURL == "" {
		return c, nil
	}
	if c.ClientID == "" || c.ClientSecret == "" || c.BaseURL == "" {
		return Identity{}, errors.New("complete GitHub identity configuration is required")
	}
	u, err := url.Parse(c.BaseURL)
	if err != nil || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return Identity{}, errors.New("PUBLIC_BASE_URL must be an origin URL")
	}
	loopback := u.Hostname() == "localhost"
	if ip := net.ParseIP(u.Hostname()); ip != nil && ip.IsLoopback() {
		loopback = true
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && loopback) {
		return Identity{}, errors.New("PUBLIC_BASE_URL requires HTTPS outside loopback development")
	}
	if p := u.Port(); p != "" {
		port, err := strconv.Atoi(p)
		if err != nil || port < 1 || port > 65535 {
			return Identity{}, errors.New("PUBLIC_BASE_URL has an invalid port")
		}
	}
	c.Enabled = true
	c.SecureCookies = u.Scheme == "https"
	return c, nil
}
