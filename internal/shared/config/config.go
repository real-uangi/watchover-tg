package config

import (
	"encoding/base64"
	"errors"
	"net/url"
	"os"
	"strings"
)

// Config contains startup settings. Never log this value: it contains credentials.
type Config struct {
	Origin        string
	SecureCookies bool
	AdminPassword string
	EncryptionKey []byte
}

func Load() (*Config, error) {
	if os.Getenv("DB_DSN") == "" {
		return nil, errors.New("DB_DSN is required")
	}
	origin, err := CanonicalOrigin(os.Getenv("WATCHOVER_PUBLIC_URL"))
	if err != nil {
		return nil, errors.New("WATCHOVER_PUBLIC_URL must be an HTTP(S) origin")
	}
	password := os.Getenv("WATCHOVER_ADMIN_PASSWORD")
	if password == "" {
		return nil, errors.New("WATCHOVER_ADMIN_PASSWORD is required")
	}
	key, err := base64.StdEncoding.DecodeString(os.Getenv("WATCHOVER_ENCRYPTION_KEY"))
	if err != nil || len(key) != 32 {
		return nil, errors.New("WATCHOVER_ENCRYPTION_KEY must encode 32 bytes in standard Base64")
	}
	cfg := &Config{Origin: origin, SecureCookies: strings.HasPrefix(origin, "https://"), AdminPassword: password, EncryptionKey: key}
	if os.Getenv("RUN_MODE") == "release" && !cfg.SecureCookies {
		return nil, errors.New("WATCHOVER_PUBLIC_URL must use HTTPS in release mode")
	}
	return cfg, nil
}

// CanonicalOrigin normalizes default ports so browser and configured origins agree.
func CanonicalOrigin(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return "", errors.New("invalid origin")
	}
	u.Host = strings.ToLower(u.Host)
	if (u.Scheme == "https" && u.Port() == "443") || (u.Scheme == "http" && u.Port() == "80") {
		u.Host = u.Hostname()
		if strings.Contains(u.Host, ":") {
			u.Host = "[" + u.Host + "]"
		}
	}
	return u.Scheme + "://" + u.Host, nil
}
