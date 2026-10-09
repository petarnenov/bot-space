// Package contracts binds executable work to immutable OpenSpec input.
package contracts

import (
	"encoding/json"
	"errors"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/petarnenov/bot-space/internal/security"
)

var ErrInvalid = errors.New("invalid OpenSpec contract")
var ErrStale = errors.New("stale OpenSpec contract")
var changeName = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)
var taskName = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)+$`)
var hash = regexp.MustCompile(`^[0-9a-f]{64}$`)
var commit = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

type Content struct {
	ProjectID    string            `json:"project_id"`
	RootID       string            `json:"root_id"`
	RootRevision int               `json:"root_revision"`
	RepositoryID int64             `json:"repository_id"`
	BaseCommit   string            `json:"base_commit"`
	Change       string            `json:"change"`
	Tasks        []string          `json:"tasks"`
	Scenarios    []string          `json:"scenarios"`
	Artifacts    map[string]string `json:"artifacts"`
}

func (c Content) Canonical() (Content, string, error) {
	c.ProjectID = strings.ToLower(c.ProjectID)
	c.RootID = strings.ToLower(c.RootID)
	c.BaseCommit = strings.ToLower(c.BaseCommit)
	if !security.ValidUUID(c.ProjectID) || !security.ValidUUID(c.RootID) || c.RootRevision < 1 || c.RepositoryID < 1 || !commit.MatchString(c.BaseCommit) || len(c.Change) > 100 || !changeName.MatchString(c.Change) || len(c.Tasks) == 0 || len(c.Tasks) > 128 || len(c.Scenarios) == 0 || len(c.Scenarios) > 128 || len(c.Artifacts) > 256 {
		return Content{}, "", ErrInvalid
	}
	c.Tasks = slices.Clone(c.Tasks)
	c.Scenarios = slices.Clone(c.Scenarios)
	slices.Sort(c.Tasks)
	slices.Sort(c.Scenarios)
	for i, t := range c.Tasks {
		if !taskName.MatchString(t) || len(t) > 32 || i > 0 && t == c.Tasks[i-1] {
			return Content{}, "", ErrInvalid
		}
	}
	for i, s := range c.Scenarios {
		if strings.TrimSpace(s) == "" || len(s) > 256 || strings.ContainsAny(s, "\r\n\x00") || i > 0 && s == c.Scenarios[i-1] {
			return Content{}, "", ErrInvalid
		}
	}
	prefix := "openspec/changes/" + c.Change + "/"
	artifacts := make(map[string]string, len(c.Artifacts))
	specs := 0
	for name, digest := range c.Artifacts {
		if path.Clean(name) != name || !strings.HasPrefix(name, prefix) || strings.ContainsAny(name, "\\\x00") || !hash.MatchString(digest) {
			return Content{}, "", ErrInvalid
		}
		relative := strings.TrimPrefix(name, prefix)
		if strings.HasPrefix(relative, "specs/") && strings.HasSuffix(relative, "/spec.md") {
			specs++
		} else if relative != "proposal.md" && relative != "design.md" && relative != "tasks.md" {
			return Content{}, "", ErrInvalid
		}
		artifacts[name] = digest
	}
	for _, name := range []string{"proposal.md", "design.md", "tasks.md"} {
		if !hash.MatchString(artifacts[prefix+name]) {
			return Content{}, "", ErrInvalid
		}
	}
	if specs == 0 {
		return Content{}, "", ErrInvalid
	}
	c.Artifacts = artifacts
	raw, err := json.Marshal(c)
	if err != nil || len(raw) > 65536 {
		return Content{}, "", ErrInvalid
	}
	return c, security.Hash(string(raw)), nil
}

// Matches rejects an old decision/package after any bound input changes.
func (c Content) Matches(expectedHash string, currentRootRevision int, currentArtifacts map[string]string) error {
	canonical, digest, err := c.Canonical()
	if err != nil {
		return err
	}
	if digest != expectedHash || currentRootRevision != canonical.RootRevision || len(currentArtifacts) != len(canonical.Artifacts) {
		return ErrStale
	}
	for name, value := range canonical.Artifacts {
		if currentArtifacts[name] != value {
			return ErrStale
		}
	}
	return nil
}
