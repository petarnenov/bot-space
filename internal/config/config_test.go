package config

import (
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	for _, tc := range []struct {
		name  string
		env   map[string]string
		valid bool
		port  int
		conns int32
	}{
		{"defaults", map[string]string{"DATABASE_URL": "postgres://user:password@localhost/db"}, true, 8080, 10},
		{"configured", map[string]string{"DATABASE_URL": "postgresql://user:password@localhost/db", "PORT": "9000", "DB_MAX_CONNS": "2"}, true, 9000, 2},
		{"missing", nil, false, 0, 0},
		{"invalid secret URL", map[string]string{"DATABASE_URL": "secret-sentinel"}, false, 0, 0},
		{"port zero", map[string]string{"DATABASE_URL": "postgres://localhost/db", "PORT": "0"}, false, 0, 0},
		{"port overflow", map[string]string{"DATABASE_URL": "postgres://localhost/db", "PORT": "65536"}, false, 0, 0},
		{"port text", map[string]string{"DATABASE_URL": "postgres://localhost/db", "PORT": "secret-sentinel"}, false, 0, 0},
		{"pool zero", map[string]string{"DATABASE_URL": "postgres://localhost/db", "DB_MAX_CONNS": "0"}, false, 0, 0},
		{"pool overflow", map[string]string{"DATABASE_URL": "postgres://localhost/db", "DB_MAX_CONNS": "1001"}, false, 0, 0},
		{"invalid driver query", map[string]string{"DATABASE_URL": "postgres://u:secret-sentinel@localhost/db?connect_timeout=secret-sentinel"}, false, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := Load(func(k string) string { return tc.env[k] })
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
			if err != nil && strings.Contains(err.Error(), "secret-sentinel") {
				t.Fatal("secret leaked")
			}
			if tc.valid && (c.Port != tc.port || c.MaxConns != tc.conns) {
				t.Fatalf("unexpected defaults: port=%d pool=%d", c.Port, c.MaxConns)
			}
		})
	}
}
