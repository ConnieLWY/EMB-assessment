package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
)

type Config struct {
	DatabaseURL    string
	HTTPAddr       string
	FrontendOrigin string
	APIOrigin      string
	CookieSecure   bool
}

func Load() (Config, error) {
	cfg := Config{DatabaseURL: os.Getenv("DATABASE_URL"), HTTPAddr: os.Getenv("HTTP_ADDR")}
	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = ":8080"
	}
	cfg.FrontendOrigin = os.Getenv("FRONTEND_ORIGIN")
	if cfg.FrontendOrigin == "" {
		cfg.FrontendOrigin = "http://localhost:3000"
	}
	cfg.APIOrigin = os.Getenv("API_ORIGIN")
	if cfg.APIOrigin == "" {
		cfg.APIOrigin = "http://localhost:8080"
	}
	for name, origin := range map[string]string{"FRONTEND_ORIGIN": cfg.FrontendOrigin, "API_ORIGIN": cfg.APIOrigin} {
		u, err := url.Parse(origin)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
			return Config{}, fmt.Errorf("%s must be an HTTP(S) origin without credentials, path, query, or fragment", name)
		}
	}
	u, _ := url.Parse(cfg.APIOrigin)
	cfg.CookieSecure = u.Scheme == "https"
	return cfg, nil
}
