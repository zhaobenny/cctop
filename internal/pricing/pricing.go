package pricing

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/zhaobenny/cctop/internal/model"
)

const liteLLMPricingURL = "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json"

// liteLLMModel represents the pricing structure from LiteLLM
type liteLLMModel struct {
	InputCostPerToken         float64 `json:"input_cost_per_token"`
	OutputCostPerToken        float64 `json:"output_cost_per_token"`
	CacheCreationCost         float64 `json:"cache_creation_input_token_cost"`
	CacheReadCost             float64 `json:"cache_read_input_token_cost"`
	LiteLLMProvider           string  `json:"litellm_provider"`
}

// pricingCache caches the pricing data
var pricingCache map[string]model.ModelPricing
var cacheTime time.Time
var cacheDuration = 1 * time.Hour

// FetchPricing fetches pricing data from LiteLLM
func FetchPricing() (map[string]model.ModelPricing, error) {
	// Return cached data if fresh
	if pricingCache != nil && time.Since(cacheTime) < cacheDuration {
		return pricingCache, nil
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(liteLLMPricingURL)
	if err != nil {
		return GetEmbeddedPricing(), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return GetEmbeddedPricing(), nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return GetEmbeddedPricing(), nil
	}

	var rawPricing map[string]liteLLMModel
	if err := json.Unmarshal(body, &rawPricing); err != nil {
		return GetEmbeddedPricing(), nil
	}

	pricing := make(map[string]model.ModelPricing)
	for name, data := range rawPricing {
		// Only include Anthropic provider models
		if data.LiteLLMProvider != "anthropic" {
			continue
		}
		pricing[name] = model.ModelPricing{
			InputCostPerToken:         data.InputCostPerToken,
			OutputCostPerToken:        data.OutputCostPerToken,
			CacheCreationCostPerToken: data.CacheCreationCost,
			CacheReadCostPerToken:     data.CacheReadCost,
		}
	}

	pricingCache = pricing
	cacheTime = time.Now()
	return pricing, nil
}

// GetEmbeddedPricing returns fallback embedded pricing data
func GetEmbeddedPricing() map[string]model.ModelPricing {
	return map[string]model.ModelPricing{
		// Opus 4.5
		"claude-opus-4-5-20251101": {
			InputCostPerToken:         5e-06,
			OutputCostPerToken:        2.5e-05,
			CacheCreationCostPerToken: 6.25e-06,
			CacheReadCostPerToken:     5e-07,
		},
		"claude-opus-4-5": {
			InputCostPerToken:         5e-06,
			OutputCostPerToken:        2.5e-05,
			CacheCreationCostPerToken: 6.25e-06,
			CacheReadCostPerToken:     5e-07,
		},
		// Opus 4.1
		"claude-opus-4-1-20250805": {
			InputCostPerToken:         1.5e-05,
			OutputCostPerToken:        7.5e-05,
			CacheCreationCostPerToken: 1.875e-05,
			CacheReadCostPerToken:     1.5e-06,
		},
		"claude-opus-4-1": {
			InputCostPerToken:         1.5e-05,
			OutputCostPerToken:        7.5e-05,
			CacheCreationCostPerToken: 1.875e-05,
			CacheReadCostPerToken:     1.5e-06,
		},
		// Opus 4
		"claude-opus-4-20250514": {
			InputCostPerToken:         1.5e-05,
			OutputCostPerToken:        7.5e-05,
			CacheCreationCostPerToken: 1.875e-05,
			CacheReadCostPerToken:     1.5e-06,
		},
		"claude-4-opus-20250514": {
			InputCostPerToken:         1.5e-05,
			OutputCostPerToken:        7.5e-05,
			CacheCreationCostPerToken: 1.875e-05,
			CacheReadCostPerToken:     1.5e-06,
		},
		// Sonnet 4.5
		"claude-sonnet-4-5-20250929": {
			InputCostPerToken:         3e-06,
			OutputCostPerToken:        1.5e-05,
			CacheCreationCostPerToken: 3.75e-06,
			CacheReadCostPerToken:     3e-07,
		},
		"claude-sonnet-4-5": {
			InputCostPerToken:         3e-06,
			OutputCostPerToken:        1.5e-05,
			CacheCreationCostPerToken: 3.75e-06,
			CacheReadCostPerToken:     3e-07,
		},
		// Sonnet 4
		"claude-sonnet-4-20250514": {
			InputCostPerToken:         3e-06,
			OutputCostPerToken:        1.5e-05,
			CacheCreationCostPerToken: 3.75e-06,
			CacheReadCostPerToken:     3e-07,
		},
		"claude-4-sonnet-20250514": {
			InputCostPerToken:         3e-06,
			OutputCostPerToken:        1.5e-05,
			CacheCreationCostPerToken: 3.75e-06,
			CacheReadCostPerToken:     3e-07,
		},
		// Sonnet 3.7
		"claude-3-7-sonnet-20250219": {
			InputCostPerToken:         3e-06,
			OutputCostPerToken:        1.5e-05,
			CacheCreationCostPerToken: 3.75e-06,
			CacheReadCostPerToken:     3e-07,
		},
		// Sonnet 3.5
		"claude-3-5-sonnet-20241022": {
			InputCostPerToken:         3e-06,
			OutputCostPerToken:        1.5e-05,
			CacheCreationCostPerToken: 3.75e-06,
			CacheReadCostPerToken:     3e-07,
		},
		"claude-3-5-sonnet-20240620": {
			InputCostPerToken:         3e-06,
			OutputCostPerToken:        1.5e-05,
			CacheCreationCostPerToken: 3.75e-06,
			CacheReadCostPerToken:     3e-07,
		},
		// Haiku 4.5
		"claude-haiku-4-5-20251001": {
			InputCostPerToken:         1e-06,
			OutputCostPerToken:        5e-06,
			CacheCreationCostPerToken: 1.25e-06,
			CacheReadCostPerToken:     1e-07,
		},
		"claude-haiku-4-5": {
			InputCostPerToken:         1e-06,
			OutputCostPerToken:        5e-06,
			CacheCreationCostPerToken: 1.25e-06,
			CacheReadCostPerToken:     1e-07,
		},
		// Haiku 3.5
		"claude-3-5-haiku-20241022": {
			InputCostPerToken:         8e-07,
			OutputCostPerToken:        4e-06,
			CacheCreationCostPerToken: 1e-06,
			CacheReadCostPerToken:     8e-08,
		},
		// Haiku 3
		"claude-3-haiku-20240307": {
			InputCostPerToken:         2.5e-07,
			OutputCostPerToken:        1.25e-06,
			CacheCreationCostPerToken: 3e-07,
			CacheReadCostPerToken:     3e-08,
		},
		// Opus 3
		"claude-3-opus-20240229": {
			InputCostPerToken:         1.5e-05,
			OutputCostPerToken:        7.5e-05,
			CacheCreationCostPerToken: 1.875e-05,
			CacheReadCostPerToken:     1.5e-06,
		},
	}
}

// GetPricing returns pricing for a model, trying online first then falling back to embedded
func GetPricing(modelName string, offline bool) model.ModelPricing {
	var pricing map[string]model.ModelPricing
	var err error

	if offline {
		pricing = GetEmbeddedPricing()
	} else {
		pricing, err = FetchPricing()
		if err != nil {
			pricing = GetEmbeddedPricing()
		}
	}

	// Try exact match first
	if p, ok := pricing[modelName]; ok {
		return p
	}

	// Try to find a matching model by normalizing the name
	normalized := normalizeModelName(modelName)
	for name, p := range pricing {
		if normalizeModelName(name) == normalized {
			return p
		}
	}

	// Fall back to a default pricing (Sonnet 4 pricing as a reasonable default)
	fmt.Printf("Warning: Unknown model %s, using default pricing\n", modelName)
	return model.ModelPricing{
		InputCostPerToken:         3e-06,
		OutputCostPerToken:        1.5e-05,
		CacheCreationCostPerToken: 3.75e-06,
		CacheReadCostPerToken:     3e-07,
	}
}

// normalizeModelName normalizes model names for matching
func normalizeModelName(name string) string {
	// Remove common prefixes/suffixes and normalize
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, "-", "")
	name = strings.ReplaceAll(name, "_", "")
	return name
}

// CalculateCost calculates the cost for a usage record
func CalculateCost(usage model.TokenUsage, pricing model.ModelPricing) float64 {
	cost := float64(usage.InputTokens) * pricing.InputCostPerToken
	cost += float64(usage.OutputTokens) * pricing.OutputCostPerToken
	cost += float64(usage.CacheCreationInputTokens) * pricing.CacheCreationCostPerToken
	cost += float64(usage.CacheReadInputTokens) * pricing.CacheReadCostPerToken
	return cost
}
