package migrations

import (
	"testing"
	"testing/fstest"
)

func TestBundled(t *testing.T) {
	ms, err := Bundled()
	if err != nil || len(ms) != 1 || ms[0].Version != 1 || len(ms[0].Checksum) != 64 {
		t.Fatalf("invalid bundle: %v", err)
	}
}

func TestRejectMissingVersion(t *testing.T) {
	_, err := Load(fstest.MapFS{"0002_gap.sql": &fstest.MapFile{Data: []byte("SELECT 1;")}})
	if err == nil {
		t.Fatal("accepted a migration gap")
	}
}
