package router

import (
	"testing"
	"time"

	"github.com/asleak/openclaw-gateway/internal/config"
	"github.com/asleak/openclaw-gateway/internal/policy"
	"github.com/asleak/openclaw-gateway/internal/session"
)

func TestDecideUsesStickySession(t *testing.T) {
	decider := NewDecider(config.RoutingConfig{
		DefaultMode:         "local",
		StickyTTL:           config.Duration(30 * time.Minute),
		CloudDwellTime:      config.Duration(15 * time.Minute),
		ComplexityThreshold: 0.75,
		ConfidenceThreshold: 0.55,
		LocalContextLimit:   8192,
	}, session.NewStore())

	first := decider.Decide(RequestContext{
		SessionID: "sticky-1",
		Signals: policy.Signals{
			Complexity:      0.95,
			EstimatedTokens: 32,
		},
	})

	if first.Mode != policy.CloudMode {
		t.Fatalf("expected first decision to route to cloud, got %s", first.Mode)
	}

	second := decider.Decide(RequestContext{
		SessionID: "sticky-1",
		Signals: policy.Signals{
			Complexity:      0.10,
			EstimatedTokens: 8,
		},
	})

	if !second.Sticky {
		t.Fatal("expected second decision to use sticky session")
	}

	if second.Mode != policy.CloudMode {
		t.Fatalf("expected sticky decision to remain on cloud, got %s", second.Mode)
	}
}
