package contracts

import (
	"strings"
	"testing"
)

func fixture() Content {
	prefix := "openspec/changes/feature/"
	artifacts := map[string]string{}
	for _, name := range []string{"proposal.md", "design.md", "tasks.md", "specs/work/spec.md"} {
		artifacts[prefix+name] = strings.Repeat("a", 64)
	}
	return Content{ProjectID: "11111111-1111-4111-8111-111111111111", RootID: "22222222-2222-4222-8222-222222222222", RootRevision: 1, RepositoryID: 42, BaseCommit: strings.Repeat("b", 40), Change: "feature", Tasks: []string{"2.1", "1.1"}, Scenarios: []string{"second", "first"}, Artifacts: artifacts}
}
func TestContractCanonicalBindingAndStaleEvidence(t *testing.T) {
	input := fixture()
	canonical, digest, err := input.Canonical()
	if err != nil {
		t.Fatal(err)
	}
	reordered := fixture()
	reordered.Tasks = []string{"1.1", "2.1"}
	reordered.Scenarios = []string{"first", "second"}
	_, same, err := reordered.Canonical()
	if err != nil || same != digest {
		t.Fatal("order changed contract identity")
	}
	if err = canonical.Matches(digest, 1, canonical.Artifacts); err != nil {
		t.Fatal(err)
	}
	if err = canonical.Matches(digest, 2, canonical.Artifacts); err != ErrStale {
		t.Fatal("changed human input inherited approval")
	}
	changed := fixture()
	changed.Artifacts["openspec/changes/feature/design.md"] = strings.Repeat("c", 64)
	if err = canonical.Matches(digest, 1, changed.Artifacts); err != ErrStale {
		t.Fatal("changed spec inherited approval")
	}
	input.Artifacts["openspec/changes/feature/design.md"] = "tampered"
	if canonical.Artifacts["openspec/changes/feature/design.md"] == "tampered" {
		t.Fatal("caller mutated canonical contract")
	}
}
func TestContractRejectsIncompleteAndEscapingArtifacts(t *testing.T) {
	for _, name := range []string{"proposal.md", "design.md", "tasks.md", "specs/work/spec.md"} {
		c := fixture()
		delete(c.Artifacts, "openspec/changes/feature/"+name)
		if _, _, err := c.Canonical(); err != ErrInvalid {
			t.Fatal("missing required artifact accepted", name)
		}
	}
	for _, name := range []string{"/etc/passwd", "openspec/changes/feature/../../secret", "openspec/changes/other/design.md"} {
		c := fixture()
		c.Artifacts[name] = strings.Repeat("a", 64)
		if _, _, err := c.Canonical(); err != ErrInvalid {
			t.Fatal("artifact escaped change", name)
		}
	}
	c := fixture()
	c.Tasks = nil
	if _, _, err := c.Canonical(); err != ErrInvalid {
		t.Fatal("missing task references accepted")
	}
	c = fixture()
	c.BaseCommit = "main"
	if _, _, err := c.Canonical(); err != ErrInvalid {
		t.Fatal("moving branch accepted as exact base")
	}
}
