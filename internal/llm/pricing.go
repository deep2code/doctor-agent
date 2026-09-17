package llm

import "strings"

// USDToCNY converts USD cost to RMB for display purposes.
const USDToCNY = 7.3

// ModelPrice holds per-million-token prices in USD for one model family.
type ModelPrice struct {
	InputPerM  float64
	OutputPerM float64
}

// modelPrices maps model-name prefixes to list prices (USD per 1M tokens).
// Prefix matching keeps env-configurable model names working (e.g.
// "deepseek-v4-flash", "glm-4.6", "claude-sonnet-4-20250514").
// Longest matching prefix wins.
var modelPrices = []struct {
	prefix string
	price  ModelPrice
}{
	{"deepseek-reasoner", ModelPrice{InputPerM: 0.55, OutputPerM: 2.19}},
	{"deepseek", ModelPrice{InputPerM: 0.28, OutputPerM: 0.42}},
	{"glm", ModelPrice{InputPerM: 0.60, OutputPerM: 2.20}},
	{"claude-opus", ModelPrice{InputPerM: 15.0, OutputPerM: 75.0}},
	{"claude-sonnet", ModelPrice{InputPerM: 3.0, OutputPerM: 15.0}},
	{"claude-haiku", ModelPrice{InputPerM: 1.0, OutputPerM: 5.0}},
	{"gpt-4o", ModelPrice{InputPerM: 2.50, OutputPerM: 10.0}},
	{"gpt-4.1", ModelPrice{InputPerM: 2.0, OutputPerM: 8.0}},
}

// PriceForModel looks up the price for a model name; ok=false when unknown
// (cost reporting then degrades to tokens-only).
func PriceForModel(model string) (ModelPrice, bool) {
	m := strings.ToLower(model)
	var best ModelPrice
	found := false
	bestLen := 0
	for _, mp := range modelPrices {
		if strings.HasPrefix(m, mp.prefix) && len(mp.prefix) >= bestLen {
			best, found, bestLen = mp.price, true, len(mp.prefix)
		}
	}
	return best, found
}

// CostUSD computes the USD cost of one LLM call (or accumulated usage).
// Returns 0 when the model has no known pricing.
func CostUSD(model string, u TokenUsage) float64 {
	p, ok := PriceForModel(model)
	if !ok {
		return 0
	}
	return float64(u.PromptTokens)/1e6*p.InputPerM + float64(u.CompletionTokens)/1e6*p.OutputPerM
}
