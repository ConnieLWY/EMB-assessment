package config

import "testing"

func TestLoad(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	if _, err := Load(); err == nil {
		t.Fatal("missing database URL must fail startup")
	}
	t.Setenv("DATABASE_URL", "postgres://example.invalid/ev_charger")
	t.Setenv("HTTP_ADDR", "")
	cfg, err := Load()
	if err != nil || cfg.DatabaseURL != "postgres://example.invalid/ev_charger" || cfg.HTTPAddr != ":8080" {
		t.Fatalf("config=%+v error=%v", cfg, err)
	}
	t.Setenv("HTTP_ADDR", "127.0.0.1:9090")
	cfg, err = Load()
	if err != nil || cfg.HTTPAddr != "127.0.0.1:9090" {
		t.Fatalf("config=%+v error=%v", cfg, err)
	}
}
