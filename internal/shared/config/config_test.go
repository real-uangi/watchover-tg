package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestConfigurationAndCanonicalOrigin(t *testing.T) {
	t.Setenv("DB_DSN", "postgres://example")
	t.Setenv("WATCHOVER_PUBLIC_URL", "https://WATCH.EXAMPLE:443/")
	t.Setenv("WATCHOVER_ADMIN_PASSWORD", "private")
	t.Setenv("WATCHOVER_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString(make([]byte, 32)))
	t.Setenv("RUN_MODE", "release")
	cfg, err := Load()
	if err != nil || cfg.Origin != "https://watch.example" || !cfg.SecureCookies {
		t.Fatal(cfg, err)
	}
	for _, raw := range []string{"https://u:p@watch.example", "https://watch.example/path", "https://watch.example?token=x", "https://watch.example#fragment"} {
		if _, err := CanonicalOrigin(raw); err == nil {
			t.Fatal("accepted non-origin", raw)
		}
	}
	t.Setenv("WATCHOVER_PUBLIC_URL", "http://watch.example")
	if _, err := Load(); err == nil {
		t.Fatal("release accepted insecure cookies")
	}
	t.Setenv("WATCHOVER_PUBLIC_URL", "https://watch.example")
	t.Setenv("WATCHOVER_ENCRYPTION_KEY", "secret-value")
	if _, err := Load(); err == nil || strings.Contains(err.Error(), "secret-value") {
		t.Fatal("invalid key accepted or exposed")
	}
}
