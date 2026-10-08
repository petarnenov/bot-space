// Package config validates environment configuration without exposing secrets.
package config

import (
	"errors"
	"net/url"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Config struct {
	DatabaseURL string
	Port        int
	MaxConns    int32
}

func Load(getenv func(string) string) (Config, error) {
	c := Config{DatabaseURL: getenv("DATABASE_URL"), Port: 8080, MaxConns: 10}
	u, err := url.Parse(c.DatabaseURL)
	if err != nil || u == nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Hostname() == "" || strings.Trim(u.Path, "/") == "" {
		return Config{}, errors.New("DATABASE_URL must be a PostgreSQL URL with a host and database")
	}
	if _, err := pgxpool.ParseConfig(c.DatabaseURL); err != nil {
		return Config{}, errors.New("DATABASE_URL is invalid")
	}
	if value := getenv("PORT"); value != "" {
		c.Port, err = strconv.Atoi(value)
		if err != nil || c.Port < 1 || c.Port > 65535 {
			return Config{}, errors.New("PORT must be an integer between 1 and 65535")
		}
	}
	if value := getenv("DB_MAX_CONNS"); value != "" {
		n, err := strconv.ParseInt(value, 10, 32)
		if err != nil || n < 1 || n > 1000 {
			return Config{}, errors.New("DB_MAX_CONNS must be an integer between 1 and 1000")
		}
		c.MaxConns = int32(n)
	}
	return c, nil
}
