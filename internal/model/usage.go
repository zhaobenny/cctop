package model

import "time"

// UsageRecord represents a single usage entry from Claude Code JSONL
type UsageRecord struct {
	Timestamp   time.Time
	SessionID   string
	ProjectPath string
	Model       string
	Usage       TokenUsage
}

// TokenUsage contains token counts from a Claude API response
type TokenUsage struct {
	InputTokens              int64
	OutputTokens             int64
	CacheCreationInputTokens int64
	CacheReadInputTokens     int64
}

// AggregatedUsage represents usage aggregated by some key (day, month, session, etc.)
type AggregatedUsage struct {
	Key         string     // The grouping key (date, session ID, etc.)
	Usage       TokenUsage // Aggregated token counts
	Cost        float64    // Total cost in USD
	Models      []string   // Models used in this period
	RecordCount int        // Number of records aggregated
}

// ModelPricing contains pricing info for a model (per token, not per million)
type ModelPricing struct {
	InputCostPerToken       float64
	OutputCostPerToken      float64
	CacheCreationCostPerToken float64
	CacheReadCostPerToken   float64
}
