package router

import (
	"strings"
	"time"

	"github.com/XiaBiXiang/openclaw-gateway/internal/config"
	"github.com/XiaBiXiang/openclaw-gateway/internal/policy"
	"github.com/XiaBiXiang/openclaw-gateway/internal/session"
)

type RequestContext struct {
	SessionID string
	Signals   policy.Signals
}

type Decision struct {
	policy.Decision
	Sticky    bool   `json:"sticky"`
	SessionID string `json:"session_id,omitempty"`
}

type Decider struct {
	rules          policy.Rules
	sessions       *session.Store
	stickyTTL      time.Duration
	cloudDwellTime time.Duration
}

func NewDecider(cfg config.RoutingConfig, sessions *session.Store) *Decider {
	defaultMode := policy.LocalMode
	if strings.EqualFold(cfg.DefaultMode, string(policy.CloudMode)) {
		defaultMode = policy.CloudMode
	}

	return &Decider{
		rules: policy.Rules{
			DefaultMode:         defaultMode,
			ComplexityThreshold: cfg.ComplexityThreshold,
			ConfidenceThreshold: cfg.ConfidenceThreshold,
			LocalContextLimit:   cfg.LocalContextLimit,
			OfflineForceLocal:   cfg.OfflineForceLocal,
		},
		sessions:       sessions,
		stickyTTL:      time.Duration(cfg.StickyTTL),
		cloudDwellTime: time.Duration(cfg.CloudDwellTime),
	}
}

func (d *Decider) Decide(ctx RequestContext) Decision {
	if d.sessions != nil && ctx.SessionID != "" {
		if mode, ok := d.sessions.Get(ctx.SessionID); ok {
			return Decision{
				Decision: policy.Decision{
					Mode:   policy.Mode(mode),
					Reason: "sticky-session",
				},
				Sticky:    true,
				SessionID: ctx.SessionID,
			}
		}
	}

	decision := policy.Evaluate(d.rules, ctx.Signals)
	ttl := d.stickyTTL
	if decision.Mode == policy.CloudMode && d.cloudDwellTime > ttl {
		ttl = d.cloudDwellTime
	}

	if d.sessions != nil && ctx.SessionID != "" && ttl > 0 {
		d.sessions.Put(ctx.SessionID, string(decision.Mode), ttl)
	}

	return Decision{
		Decision:  decision,
		SessionID: ctx.SessionID,
	}
}
