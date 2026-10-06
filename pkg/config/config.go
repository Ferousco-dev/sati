package config

import (
	"errors"
	"fmt"
	"net/netip"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	AppEnv string
	Port   string

	DatabaseURL string
	RedisURL    string

	SessionSecret string

	PaystackSecretKey     string
	PaystackWebhookSecret string

	ResendAPIKey string
	EmailFrom    string

	R2AccountID       string
	R2AccessKeyID     string
	R2SecretAccessKey string
	R2Bucket          string
	R2PublicURL       string

	StorageDriver string
	UploadDir     string

	TrustedProxies     []netip.Prefix
	CORSAllowedOrigins []string
}

func (c Config) IsProduction() bool {
	return c.AppEnv == "production"
}

func Load() (Config, error) {
	_ = godotenv.Load()

	proxies, err := parsePrefixes(os.Getenv("TRUSTED_PROXIES"))
	if err != nil {
		return Config{}, fmt.Errorf("TRUSTED_PROXIES: %w", err)
	}

	cfg := Config{
		AppEnv: getenv("APP_ENV", "development"),
		Port:   getenv("PORT", "8080"),

		DatabaseURL: os.Getenv("DATABASE_URL"),
		RedisURL:    os.Getenv("REDIS_URL"),

		SessionSecret: os.Getenv("SESSION_SECRET"),

		PaystackSecretKey:     os.Getenv("PAYSTACK_SECRET_KEY"),
		PaystackWebhookSecret: os.Getenv("PAYSTACK_WEBHOOK_SECRET"),

		ResendAPIKey: os.Getenv("RESEND_API_KEY"),
		EmailFrom:    os.Getenv("EMAIL_FROM"),

		R2AccountID:       os.Getenv("R2_ACCOUNT_ID"),
		R2AccessKeyID:     os.Getenv("R2_ACCESS_KEY_ID"),
		R2SecretAccessKey: os.Getenv("R2_SECRET_ACCESS_KEY"),
		R2Bucket:          os.Getenv("R2_BUCKET"),
		R2PublicURL:       os.Getenv("R2_PUBLIC_URL"),

		StorageDriver: getenv("STORAGE_DRIVER", "local"),
		UploadDir:     getenv("UPLOAD_DIR", "uploads"),

		TrustedProxies:     proxies,
		CORSAllowedOrigins: splitList(os.Getenv("CORS_ALLOWED_ORIGINS")),
	}

	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) validate() error {
	if c.DatabaseURL == "" {
		return errors.New("DATABASE_URL is required")
	}
	if c.StorageDriver != "local" && c.StorageDriver != "r2" {
		return errors.New("STORAGE_DRIVER must be local or r2")
	}
	if !c.IsProduction() {
		return nil
	}

	required := map[string]string{
		"SESSION_SECRET":          c.SessionSecret,
		"PAYSTACK_SECRET_KEY":     c.PaystackSecretKey,
		"PAYSTACK_WEBHOOK_SECRET": c.PaystackWebhookSecret,
		"RESEND_API_KEY":          c.ResendAPIKey,
		"EMAIL_FROM":              c.EmailFrom,
		"R2_ACCOUNT_ID":           c.R2AccountID,
		"R2_ACCESS_KEY_ID":        c.R2AccessKeyID,
		"R2_SECRET_ACCESS_KEY":    c.R2SecretAccessKey,
		"R2_BUCKET":               c.R2Bucket,
		"R2_PUBLIC_URL":           c.R2PublicURL,
		"CORS_ALLOWED_ORIGINS":    strings.Join(c.CORSAllowedOrigins, ","),
	}

	var missing []string
	for name, value := range required {
		if value == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required production settings: %s", strings.Join(missing, ", "))
	}
	return nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func splitList(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func parsePrefixes(raw string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, part := range splitList(raw) {
		prefix, err := netip.ParsePrefix(part)
		if err != nil {
			addr, addrErr := netip.ParseAddr(part)
			if addrErr != nil {
				return nil, fmt.Errorf("invalid entry %q", part)
			}
			prefix = netip.PrefixFrom(addr, addr.BitLen())
		}
		out = append(out, prefix)
	}
	return out, nil
}
