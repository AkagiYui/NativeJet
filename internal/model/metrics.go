package model

import "time"

const (
	MetricsOverview  = "overview"
	MetricsNodes     = "nodes"
	MetricsRules     = "rules"
	MetricsProviders = "providers"
)

type Metrics struct {
	PID               int              `json:"pid"`
	Memory            *int64           `json:"memory,omitempty"`
	ActiveConnections *int             `json:"activeConnections,omitempty"`
	Nodes             []NodeMetric     `json:"nodes,omitempty"`
	Rules             []RuleMetric     `json:"rules,omitempty"`
	Providers         []ProviderMetric `json:"providers,omitempty"`
	Problems          []string         `json:"problems,omitempty"`
}

type NodeMetric struct {
	Name     string        `json:"name"`
	Type     string        `json:"type"`
	Provider string        `json:"provider,omitempty"`
	TestURL  string        `json:"testUrl,omitempty"`
	Alive    *bool         `json:"alive,omitempty"`
	History  []DelaySample `json:"history,omitempty"`
}

type DelaySample struct {
	Time  time.Time `json:"time"`
	Delay uint16    `json:"delay"`
}

type RuleMetric struct {
	Index   int         `json:"index"`
	Type    string      `json:"type"`
	Payload string      `json:"payload"`
	Proxy   string      `json:"proxy"`
	Extra   *RuleCounts `json:"extra,omitempty"`
}

type RuleCounts struct {
	Disabled  bool      `json:"disabled"`
	HitCount  uint64    `json:"hitCount"`
	MissCount uint64    `json:"missCount"`
	HitAt     time.Time `json:"hitAt"`
	MissAt    time.Time `json:"missAt"`
}

type ProviderMetric struct {
	Name         string             `json:"name"`
	IsRuleSet    bool               `json:"isRuleSet,omitempty"`
	Count        int                `json:"count"`
	UpdatedAt    time.Time          `json:"updatedAt"`
	Subscription *SubscriptionQuota `json:"subscription,omitempty"`
}

type SubscriptionQuota struct {
	Upload   int64 `json:"upload"`
	Download int64 `json:"download"`
	Total    int64 `json:"total"`
	Expire   int64 `json:"expire"`
}
