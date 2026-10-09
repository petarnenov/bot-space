package config

import (
	"strings"
	"testing"
)

func TestMailboxConfiguration(t *testing.T) {
	for _, flag := range []string{"true", "false", "invalid"} {
		c, err := LoadMailbox(func(k string) string {
			if k == "TASKS_ENABLED" {
				return flag
			}
			if k == "CURSOR_SIGNING_KEY" {
				return strings.Repeat("01", 32)
			}
			return ""
		}, "")
		if flag == "invalid" {
			if err == nil {
				t.Fatal("invalid task feature flag accepted")
			}
			continue
		}
		if err != nil || c.TasksEnabled != (flag == "true") {
			t.Fatal("task feature flag differs from configuration")
		}
	}
	if c, err := LoadMailbox(func(string) string { return "" }, ""); err != nil || c.Enabled {
		t.Fatal("absent mailbox enabled")
	}
	for _, key := range []string{"invalid", strings.Repeat("a", 63), strings.Repeat("x", 64)} {
		if _, err := LoadMailbox(func(k string) string {
			if k == "CURSOR_SIGNING_KEY" {
				return key
			}
			return ""
		}, ""); err == nil {
			t.Fatal("invalid cursor key accepted")
		}
	}
	if _, err := LoadMailbox(func(k string) string {
		if k == "MCP_ALLOWED_ORIGINS" {
			return "https://example.com"
		}
		return ""
	}, ""); err == nil {
		t.Fatal("origins without key accepted")
	}
	for _, origin := range []string{"*", "null", "http://external.example", "https://example.com/path", "https://user:secret@example.com"} {
		if _, err := LoadMailbox(func(k string) string {
			if k == "CURSOR_SIGNING_KEY" {
				return strings.Repeat("01", 32)
			}
			if k == "MCP_ALLOWED_ORIGINS" {
				return origin
			}
			return ""
		}, ""); err == nil {
			t.Fatal("unsafe origin accepted")
		}
	}
	c, err := LoadMailbox(func(k string) string {
		if k == "CURSOR_SIGNING_KEY" {
			return strings.Repeat("01", 32)
		}
		if k == "MCP_ALLOWED_ORIGINS" {
			return "https://example.com,http://localhost:8080"
		}
		return ""
	}, "https://example.com")
	if err != nil || !c.Enabled || len(c.CursorKey) != 32 || len(c.AllowedOrigins) != 2 {
		t.Fatal("valid mailbox config rejected")
	}
}
