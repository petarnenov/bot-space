// Package mcpclient connects native bearer-configured clients through the official SDK.
package mcpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Client struct{ Session *mcp.ClientSession }
type ApplicationError struct{ Code string }

func (e *ApplicationError) Error() string { return "mailbox tool error: " + e.Code }

type credentialTransport struct {
	token, origin string
	next          http.RoundTripper
}

func (t credentialTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if strings.ToLower(r.URL.Scheme+"://"+r.URL.Host) != t.origin {
		return nil, errors.New("refusing credential forwarding to another origin")
	}
	copy := r.Clone(r.Context())
	copy.Header = r.Header.Clone()
	if copy.Header == nil {
		copy.Header = make(http.Header)
	}
	copy.Header.Set("Authorization", "Bearer "+t.token)
	return t.next.RoundTrip(copy)
}

func Connect(ctx context.Context, endpoint, token string) (*Client, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || u.Path != "/mcp" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || token == "" || strings.ContainsAny(token, "\r\n") {
		return nil, errors.New("invalid MCP endpoint or credential configuration")
	}
	loopback := u.Hostname() == "localhost"
	if ip := net.ParseIP(u.Hostname()); ip != nil && ip.IsLoopback() {
		loopback = true
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && loopback) {
		return nil, errors.New("MCP endpoint requires HTTPS outside loopback development")
	}
	client := &http.Client{Timeout: 15 * time.Second, Transport: credentialTransport{token: token, origin: strings.ToLower(u.Scheme + "://" + u.Host), next: http.DefaultTransport}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	sdk := mcp.NewClient(&mcp.Implementation{Name: "bot-space-native-client", Version: "0.1.0"}, nil)
	session, err := sdk.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: endpoint, HTTPClient: client, DisableStandaloneSSE: true, MaxRetries: -1}, nil)
	if err != nil {
		return nil, errors.New("MCP connection failed")
	}
	return &Client{Session: session}, nil
}

func (c *Client) Close() error { return c.Session.Close() }

func (c *Client) Call(ctx context.Context, name string, arguments any, destination any) error {
	result, err := c.Session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		return errors.New("MCP tool request failed")
	}
	if len(result.Content) != 1 {
		return errors.New("unexpected MCP result format")
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		return errors.New("unexpected MCP content type")
	}
	if result.IsError {
		var payload struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if json.Unmarshal([]byte(text.Text), &payload) != nil || payload.Error.Code == "" {
			return &ApplicationError{Code: "tool_error"}
		}
		switch payload.Error.Code {
		case "invalid_argument", "not_found", "forbidden", "idempotency_conflict", "rate_limited", "temporarily_unavailable":
		default:
			return &ApplicationError{Code: "tool_error"}
		}
		return &ApplicationError{Code: payload.Error.Code}
	}
	decoder := json.NewDecoder(bytes.NewReader([]byte(text.Text)))
	decoder.UseNumber()
	if decoder.Decode(destination) != nil {
		return errors.New("invalid MCP result data")
	}
	return nil
}
