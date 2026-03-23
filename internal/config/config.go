package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

type Duration time.Duration

func (d *Duration) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}

	if len(data) > 0 && data[0] == '"' {
		var raw string
		if err := json.Unmarshal(data, &raw); err != nil {
			return err
		}

		parsed, err := time.ParseDuration(raw)
		if err != nil {
			return fmt.Errorf("invalid duration %q: %w", raw, err)
		}

		*d = Duration(parsed)
		return nil
	}

	var raw int64
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("duration must be a string or integer nanoseconds: %w", err)
	}

	*d = Duration(time.Duration(raw))
	return nil
}

type Config struct {
	Version       string              `json:"version"`
	Server        ServerConfig        `json:"server"`
	Routing       RoutingConfig       `json:"routing"`
	Providers     ProvidersConfig     `json:"providers"`
	Observability ObservabilityConfig `json:"observability"`
}

type ServerConfig struct {
	Host         string   `json:"host"`
	Port         int      `json:"port"`
	ReadTimeout  Duration `json:"read_timeout"`
	WriteTimeout Duration `json:"write_timeout"`
}

type RoutingConfig struct {
	DefaultMode         string   `json:"default_mode"`
	StickyTTL           Duration `json:"sticky_ttl"`
	CloudDwellTime      Duration `json:"cloud_dwell_time"`
	ComplexityThreshold float64  `json:"complexity_threshold"`
	ConfidenceThreshold float64  `json:"confidence_threshold"`
	LocalContextLimit   int      `json:"local_context_limit"`
	OfflineForceLocal   bool     `json:"offline_force_local"`
}

type ProvidersConfig struct {
	Local ProviderConfig `json:"local"`
	Cloud ProviderConfig `json:"cloud"`
}

type ProviderConfig struct {
	Enabled       bool     `json:"enabled"`
	Type          string   `json:"type"`
	API           string   `json:"api"`
	BaseURL       string   `json:"base_url"`
	Model         string   `json:"model"`
	APIKey        string   `json:"api_key"`
	APIKeyEnv     string   `json:"api_key_env"`
	Timeout       Duration `json:"timeout"`
	RetryAttempts int      `json:"retry_attempts"`
	RetryBackoff  Duration `json:"retry_backoff"`
}

type ObservabilityConfig struct {
	LogLevel    string `json:"log_level"`
	DecisionLog bool   `json:"decision_log"`
}

func Load(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}

	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Version == "" {
		c.Version = "v1"
	}

	if c.Server.Host == "" {
		c.Server.Host = "0.0.0.0"
	}
	if c.Server.Port == 0 {
		c.Server.Port = 8080
	}
	if c.Server.ReadTimeout == 0 {
		c.Server.ReadTimeout = Duration(10 * time.Second)
	}
	if c.Server.WriteTimeout == 0 {
		c.Server.WriteTimeout = Duration(60 * time.Second)
	}

	if c.Routing.DefaultMode == "" {
		c.Routing.DefaultMode = "local"
	}
	if c.Routing.StickyTTL == 0 {
		c.Routing.StickyTTL = Duration(30 * time.Minute)
	}
	if c.Routing.CloudDwellTime == 0 {
		c.Routing.CloudDwellTime = Duration(15 * time.Minute)
	}
	if c.Routing.ComplexityThreshold == 0 {
		c.Routing.ComplexityThreshold = 0.75
	}
	if c.Routing.ConfidenceThreshold == 0 {
		c.Routing.ConfidenceThreshold = 0.55
	}
	if c.Routing.LocalContextLimit == 0 {
		c.Routing.LocalContextLimit = 8192
	}

	if c.Observability.LogLevel == "" {
		c.Observability.LogLevel = "info"
	}

	if c.Providers.Local.API == "" {
		c.Providers.Local.API = "chat-completions"
	}
	if c.Providers.Cloud.API == "" {
		c.Providers.Cloud.API = "chat-completions"
	}
}

func (c Config) validate() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return errors.New("server.port must be between 1 and 65535")
	}

	mode := strings.ToLower(strings.TrimSpace(c.Routing.DefaultMode))
	if mode != "local" && mode != "cloud" {
		return errors.New("routing.default_mode must be local or cloud")
	}

	if c.Routing.ComplexityThreshold < 0 || c.Routing.ComplexityThreshold > 1 {
		return errors.New("routing.complexity_threshold must be between 0 and 1")
	}

	if c.Routing.ConfidenceThreshold < 0 || c.Routing.ConfidenceThreshold > 1 {
		return errors.New("routing.confidence_threshold must be between 0 and 1")
	}

	if c.Routing.LocalContextLimit < 1 {
		return errors.New("routing.local_context_limit must be positive")
	}

	if !c.Providers.Local.Enabled && !c.Providers.Cloud.Enabled {
		return errors.New("at least one provider must be enabled")
	}

	if c.Providers.Local.Enabled && c.Providers.Local.BaseURL == "" {
		return errors.New("providers.local.base_url is required when local provider is enabled")
	}
	if c.Providers.Local.Enabled && !isValidProviderAPI(c.Providers.Local.API) {
		return errors.New("providers.local.api must be chat-completions or responses")
	}

	if c.Providers.Cloud.Enabled && c.Providers.Cloud.BaseURL == "" {
		return errors.New("providers.cloud.base_url is required when cloud provider is enabled")
	}
	if c.Providers.Cloud.Enabled && !isValidProviderAPI(c.Providers.Cloud.API) {
		return errors.New("providers.cloud.api must be chat-completions or responses")
	}
	if c.Providers.Local.RetryAttempts < 0 || c.Providers.Cloud.RetryAttempts < 0 {
		return errors.New("provider retry_attempts must be zero or positive")
	}

	return nil
}

func isValidProviderAPI(api string) bool {
	switch strings.ToLower(strings.TrimSpace(api)) {
	case "chat-completions", "responses":
		return true
	default:
		return false
	}
}

func (p ProviderConfig) ResolvedAPIKey() string {
	if strings.TrimSpace(p.APIKey) != "" {
		return p.APIKey
	}
	if strings.TrimSpace(p.APIKeyEnv) == "" {
		return ""
	}
	return os.Getenv(p.APIKeyEnv)
}
