package config

import (
	"cmp"
	"os"
	"strings"

	"github.com/charmbracelet/log"
)

type Config struct {
	ListenAddress string
	LogLevel      log.Level
	LogFormat     string // "json" | "text"

	Provider string // "ollama" | "gemini"

	OllamaModel string

	GeminiAPIKey string
	GeminiModel  string
}

func parseAddress(addr string) string {
	addr = strings.TrimSpace(addr)
	if strings.Contains(addr, ":") {
		return addr
	}
	return ":" + addr
}

func Load() Config {
	logLevel, err := log.ParseLevel(os.Getenv("LOG_LEVEL"))
	if err != nil {
		logLevel = log.InfoLevel
	}

	logFormat := strings.ToLower(strings.TrimSpace(os.Getenv("LOG_FORMAT")))
	if logFormat != "json" && logFormat != "text" {
		logFormat = "text"
	}

	provider := strings.ToLower(strings.TrimSpace(os.Getenv("PROVIDER")))
	if provider != "gemini" {
		provider = "ollama"
	}

	cfg := Config{
		ListenAddress: parseAddress(cmp.Or(os.Getenv("LISTEN_ADDRESS"), "127.0.0.1:55556")),
		LogLevel:      logLevel,
		LogFormat:     logFormat,
		Provider:      provider,
		OllamaModel:   cmp.Or(os.Getenv("OLLAMA_MODEL"), "qwen2.5vl:3b"),
		GeminiAPIKey:  os.Getenv("GOOGLE_API_KEY"),
		GeminiModel:   cmp.Or(os.Getenv("GEMINI_MODEL"), "gemini-2.0-flash"),
	}

	if provider == "gemini" && cfg.GeminiAPIKey == "" {
		panic("GOOGLE_API_KEY is required when PROVIDER=gemini")
	}

	return cfg
}
