// Package migrations embeds versioned SQL in the application binary.
package migrations

import (
	"crypto/sha256"
	"embed"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
)

//go:embed *.sql
var sqlFiles embed.FS

type Migration struct {
	Version  int64
	SQL      string
	Checksum string
}

func Bundled() ([]Migration, error) { return Load(sqlFiles) }

func Load(files fs.FS) ([]Migration, error) {
	names, err := fs.Glob(files, "*.sql")
	if err != nil || len(names) == 0 {
		return nil, fmt.Errorf("no migrations available")
	}
	var result []Migration
	for _, name := range names {
		version, err := strconv.ParseInt(strings.SplitN(name, "_", 2)[0], 10, 64)
		if err != nil || version != int64(len(result)+1) {
			return nil, fmt.Errorf("migration versions must be contiguous from 1")
		}
		data, err := fs.ReadFile(files, name)
		if err != nil {
			return nil, fmt.Errorf("cannot load migration %d", version)
		}
		result = append(result, Migration{Version: version, SQL: string(data), Checksum: fmt.Sprintf("%x", sha256.Sum256(data))})
	}
	return result, nil
}
