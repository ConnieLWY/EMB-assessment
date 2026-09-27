package config

import "testing"

func TestLoad(t *testing.T) {
	t.Setenv("FRONTEND_ORIGIN", "")
	t.Setenv("API_ORIGIN", "")
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

func TestSecurityConfiguration(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example.invalid/ev_charger")
	t.Setenv("FRONTEND_ORIGIN", "")
	t.Setenv("API_ORIGIN", "")
	cfg, err := Load()
	if err != nil || cfg.FrontendOrigin != "http://localhost:3000" || cfg.APIOrigin != "http://localhost:8080" || cfg.CookieSecure {
		t.Fatalf("local configuration=%+v error=%v", cfg, err)
	}
	t.Setenv("FRONTEND_ORIGIN", "https://app.example.com")
	t.Setenv("API_ORIGIN", "https://api.example.com")
	cfg, err = Load()
	if err != nil || !cfg.CookieSecure || cfg.FrontendOrigin != "https://app.example.com" {
		t.Fatalf("HTTPS configuration=%+v error=%v", cfg, err)
	}
	for _, key := range []string{"API_ORIGIN", "FRONTEND_ORIGIN"} {
		for _, value := range []string{"*", "null", "https://example.com/path", "https://user:pass@example.com", "https://example.com?x=1", "https://example.com#fragment"} {
			t.Run(key+"/"+value, func(t *testing.T) {
				t.Setenv(key, value)
				if _, err := Load(); err == nil {
					t.Fatalf("accepted invalid origin %q", value)
				}
			})
		}
	}
}
