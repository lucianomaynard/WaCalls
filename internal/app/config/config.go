package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Addr           string
	DBPath         string
	StaticDir      string
	Version        string
	Debug          bool
	MaxCalls       int
	DatabaseURL    string
	APIToken       string
	AdminUser      string
	AdminPassword  string
	CORSOrigins    string
	RateLimit      float64
	WebRTCUDPPort  int
	PublicIPs      []string
	WebhookURL     string
	WebhookSecret  string
	TrustedProxies string
	DiagDir        string
	STUNServers    []string
	// Gravação das chamadas (perfex_calls): pasta e retenção local em horas (padrão 168 = 7 dias).
	RecordDir       string
	RecordRetention int
}

func LoadConfig(addr, dbPath, staticDir string, debug bool, maxCalls int) Config {
	return Config{
		Addr:            addr,
		DBPath:          dbPath,
		StaticDir:       staticDir,
		Debug:           debug,
		MaxCalls:        maxCalls,
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		APIToken:        os.Getenv("WACALLS_API_TOKEN"),
		AdminUser:       strings.TrimSpace(os.Getenv("WACALLS_ADMIN_USER")),
		AdminPassword:   os.Getenv("WACALLS_ADMIN_PASSWORD"),
		CORSOrigins:     os.Getenv("WACALLS_CORS_ORIGINS"),
		RateLimit:       parseRateLimit(os.Getenv("WACALLS_RATE_LIMIT")),
		WebRTCUDPPort:   parseUDPPort(os.Getenv("WACALLS_WEBRTC_UDP_PORT")),
		PublicIPs:       parsePublicIPs(os.Getenv("WACALLS_PUBLIC_IP")),
		WebhookURL:      strings.TrimSpace(os.Getenv("WACALLS_WEBHOOK_URL")),
		WebhookSecret:   os.Getenv("WACALLS_WEBHOOK_SECRET"),
		TrustedProxies:  os.Getenv("WACALLS_TRUSTED_PROXIES"),
		DiagDir:         strings.TrimSpace(os.Getenv("WACALLS_DIAG_DIR")),
		STUNServers:     parseSTUNServers(os.Getenv("WACALLS_STUN_SERVER")),
		RecordDir:       strings.TrimSpace(os.Getenv("WACALLS_RECORD_DIR")),
		RecordRetention: parseRetentionHours(os.Getenv("WACALLS_RECORD_RETENTION_HOURS")),
	}
}

func Validate(cfg Config) error {
	if cfg.WebhookURL != "" && cfg.WebhookSecret == "" {
		return errors.New("WACALLS_WEBHOOK_URL requires WACALLS_WEBHOOK_SECRET")
	}
	if (cfg.AdminUser == "") != (cfg.AdminPassword == "") {
		return errors.New("WACALLS_ADMIN_USER and WACALLS_ADMIN_PASSWORD must be set together")
	}
	return nil
}

const defaultRateLimitRPS = 20

func parseRateLimit(raw string) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultRateLimitRPS
	}
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return defaultRateLimitRPS
	}
	if n < 0 {
		return 0
	}
	return n
}

func parseRetentionHours(raw string) int {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < 0 {
		return 168
	}
	return n
}

func parseUDPPort(raw string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(raw))
	return n
}

func parsePublicIPs(raw string) []string {
	var out []string
	for p := range strings.SplitSeq(raw, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

var defaultSTUNServers = []string{"stun.l.google.com:19302", "stun.cloudflare.com:3478"}

func parseSTUNServers(raw string) []string {
	out := parsePublicIPs(raw)
	if len(out) == 0 {
		return defaultSTUNServers
	}
	return out
}
