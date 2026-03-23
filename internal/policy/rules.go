package policy

import "strings"

type Mode string

const (
	LocalMode Mode = "local"
	CloudMode Mode = "cloud"
)

type Rules struct {
	DefaultMode         Mode
	ComplexityThreshold float64
	ConfidenceThreshold float64
	LocalContextLimit   int
	OfflineForceLocal   bool
}

type Signals struct {
	PrivacyLevel    string
	Offline         bool
	Complexity      float64
	LocalConfidence *float64
	EstimatedTokens int
}

type Decision struct {
	Mode   Mode   `json:"mode"`
	Reason string `json:"reason"`
}

func Evaluate(r Rules, s Signals) Decision {
	privacy := strings.ToLower(strings.TrimSpace(s.PrivacyLevel))
	if s.Offline && r.OfflineForceLocal {
		return Decision{Mode: LocalMode, Reason: "offline-forced-local"}
	}

	if privacy == "high" || privacy == "sensitive" {
		return Decision{Mode: LocalMode, Reason: "high-privacy"}
	}

	if s.EstimatedTokens > 0 && r.LocalContextLimit > 0 && s.EstimatedTokens > r.LocalContextLimit {
		return Decision{Mode: CloudMode, Reason: "context-limit-exceeded"}
	}

	if r.ComplexityThreshold > 0 && s.Complexity >= r.ComplexityThreshold {
		return Decision{Mode: CloudMode, Reason: "complexity-threshold-exceeded"}
	}

	if s.LocalConfidence != nil && *s.LocalConfidence < r.ConfidenceThreshold {
		return Decision{Mode: CloudMode, Reason: "local-confidence-below-threshold"}
	}

	if r.DefaultMode == CloudMode {
		return Decision{Mode: CloudMode, Reason: "default-cloud"}
	}

	return Decision{Mode: LocalMode, Reason: "default-local"}
}
