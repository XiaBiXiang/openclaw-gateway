package local

import (
	"time"

	"github.com/asleak/openclaw-gateway/internal/config"
	"github.com/asleak/openclaw-gateway/internal/providers"
)

func NewProvider(cfg config.ProviderConfig) providers.Provider {
	return providers.NewHTTPProvider(
		"local-openclaw",
		cfg.BaseURL,
		providers.ParseAPIKind(cfg.API),
		cfg.Model,
		cfg.ResolvedAPIKey(),
		time.Duration(cfg.Timeout),
		cfg.RetryAttempts,
		time.Duration(cfg.RetryBackoff),
	)
}
