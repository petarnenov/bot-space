package config

import (
	"encoding/hex"
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
)

type Mailbox struct {
	Enabled        bool
	CursorKey      []byte
	AllowedOrigins []string
}

// Origin validates and normalizes an HTTP origin, without userinfo/path/query.
func Origin(value string) (string, error) {
	u, err := url.Parse(value)
	if err != nil || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("invalid origin")
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", errors.New("invalid origin")
	}
	if p := u.Port(); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return "", errors.New("invalid origin")
		}
	}
	if strings.ContainsAny(u.Host, "* ,\t\r\n") {
		return "", errors.New("invalid origin")
	}
	return u.Scheme + "://" + u.Host, nil
}

func LoadMailbox(getenv func(string) string, publicOrigin string) (Mailbox, error) {
	key, origins := getenv("CURSOR_SIGNING_KEY"), getenv("MCP_ALLOWED_ORIGINS")
	if key == "" {
		if origins != "" {
			return Mailbox{}, errors.New("CURSOR_SIGNING_KEY is required with MCP origins")
		}
		return Mailbox{}, nil
	}
	raw, err := hex.DecodeString(key)
	if err != nil || len(raw) != 32 {
		return Mailbox{}, errors.New("CURSOR_SIGNING_KEY must encode 32 bytes as 64 hex characters")
	}
	c := Mailbox{Enabled: true, CursorKey: raw}
	seen := map[string]bool{}
	items := []string{}
	if publicOrigin != "" {
		items = append(items, publicOrigin)
	}
	if origins != "" {
		items = append(items, strings.Split(origins, ",")...)
	}
	for _, item := range items {
		normalized, err := Origin(strings.TrimSpace(item))
		if err != nil {
			return Mailbox{}, errors.New("MCP_ALLOWED_ORIGINS must contain exact HTTP origins")
		}
		u, _ := url.Parse(normalized)
		loopback := u.Hostname() == "localhost"
		if ip := net.ParseIP(u.Hostname()); ip != nil && ip.IsLoopback() {
			loopback = true
		}
		if u.Scheme != "https" && !loopback {
			return Mailbox{}, errors.New("MCP origins require HTTPS outside loopback")
		}
		if !seen[normalized] {
			seen[normalized] = true
			c.AllowedOrigins = append(c.AllowedOrigins, normalized)
		}
	}
	return c, nil
}
