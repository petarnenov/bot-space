// Package runner supervises machine-local agents and their durable sessions.
package runner

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/petarnenov/bot-space/internal/security"
)

type Config struct {
	Endpoint    string        `json:"endpoint"`
	StateDir    string        `json:"state_dir"`
	Concurrency int           `json:"concurrency"`
	Agents      []AgentConfig `json:"agents"`
}
type AgentConfig struct {
	ID             string   `json:"agent_id"`
	TokenEnv       string   `json:"token_env"`
	Provider       string   `json:"provider"`
	Executable     string   `json:"executable,omitempty"`
	Project        string   `json:"project"`
	Model          string   `json:"model,omitempty"`
	Effort         string   `json:"effort,omitempty"`
	Policy         string   `json:"policy,omitempty"`
	AllowedSenders []string `json:"allowed_senders"`
}

var envName = regexp.MustCompile("^[A-Z][A-Z0-9_]{0,127}$")

func LoadConfig(path string) (Config, error) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return Config{}, errors.New("runner configuration unavailable")
	}
	if info.Mode().Perm()&0077 != 0 {
		return Config{}, errors.New("runner configuration must be owner-only (chmod 600)")
	}
	f, err := os.Open(path)
	if err != nil {
		return Config{}, errors.New("runner configuration unavailable")
	}
	defer f.Close()
	decoder := json.NewDecoder(io.LimitReader(f, 65537))
	decoder.DisallowUnknownFields()
	var c Config
	if decoder.Decode(&c) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return Config{}, errors.New("invalid runner configuration")
	}
	if err = c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

func (c *Config) Validate() error {
	u, err := url.Parse(c.Endpoint)
	if err != nil || u.Host == "" || u.User != nil || u.Path != "/mcp" || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("invalid MCP endpoint")
	}
	local := u.Hostname() == "localhost"
	if ip := net.ParseIP(u.Hostname()); ip != nil && ip.IsLoopback() {
		local = true
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && local) {
		return errors.New("remote MCP requires HTTPS")
	}
	if !filepath.IsAbs(c.StateDir) {
		return errors.New("runner state directory must be absolute")
	}
	state, err := canonicalFuturePath(c.StateDir)
	if err != nil {
		return errors.New("runner state directory cannot be resolved")
	}
	c.StateDir = state
	if c.Concurrency == 0 {
		c.Concurrency = 1
	}
	if c.Concurrency != 1 {
		return errors.New("executor concurrency must be exactly 1")
	}
	if len(c.Agents) < 1 || len(c.Agents) > 32 {
		return errors.New("configure 1 to 32 local agents")
	}
	seen := map[string]bool{}
	for i := range c.Agents {
		a := &c.Agents[i]
		if !security.ValidUUID(a.ID) || seen[strings.ToLower(a.ID)] || !envName.MatchString(a.TokenEnv) {
			return errors.New("invalid or duplicate agent identity/token reference")
		}
		a.ID = strings.ToLower(a.ID)
		seen[a.ID] = true
		if a.Provider != "codex" && a.Provider != "claude" && a.Provider != "copilot" {
			return errors.New("unsupported local provider")
		}
		if len(a.Model) > 128 || strings.TrimSpace(a.Model) != a.Model || strings.ContainsAny(a.Model, "\r\n\x00") {
			return errors.New("invalid provider model")
		}
		if a.Effort != "" && !validEffort(a.Provider, a.Effort) {
			return errors.New("unsupported provider reasoning effort")
		}
		if a.Executable == "" {
			a.Executable = a.Provider
		}
		if a.Policy == "" {
			a.Policy = "analysis"
		}
		if a.Policy != "analysis" && a.Policy != "workspace-write" {
			return errors.New("unsupported local execution policy")
		}
		project, err := filepath.EvalSymlinks(a.Project)
		if err != nil || !filepath.IsAbs(project) {
			return errors.New("project must be an existing absolute directory")
		}
		info, err := os.Stat(project)
		if err != nil || !info.IsDir() {
			return errors.New("project directory unavailable")
		}
		a.Project = filepath.Clean(project)
		state := filepath.Clean(c.StateDir)
		if state == a.Project || strings.HasPrefix(state, a.Project+string(filepath.Separator)) {
			return errors.New("runner state must be outside managed project source")
		}
		if len(a.AllowedSenders) == 0 {
			return errors.New("configure explicit allowed sender agent IDs")
		}
		for j, id := range a.AllowedSenders {
			if !security.ValidUUID(id) {
				return errors.New("invalid permitted sender")
			}
			a.AllowedSenders[j] = strings.ToLower(id)
		}
	}
	return nil
}

// Resolve existing ancestors too, so /var aliases and parent symlinks cannot
// place a supposedly external journal inside the managed source tree.
func canonicalFuturePath(path string) (string, error) {
	probe := filepath.Clean(path)
	var missing []string
	for {
		resolved, err := filepath.EvalSymlinks(probe)
		if err == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return resolved, nil
		}
		if !os.IsNotExist(err) || probe == filepath.Dir(probe) {
			return "", err
		}
		missing = append(missing, filepath.Base(probe))
		probe = filepath.Dir(probe)
	}
}

func (a AgentConfig) permits(sender string) bool {
	for _, allowed := range a.AllowedSenders {
		if allowed == sender {
			return true
		}
	}
	return false
}

func validEffort(provider, effort string) bool {
	levels := "|low|medium|high|xhigh|max|"
	if provider != "claude" {
		levels += "none|minimal|"
	}
	if provider == "codex" {
		levels += "ultra|"
	}
	return effort != "" && !strings.Contains(effort, "|") && strings.Contains(levels, "|"+effort+"|")
}
