package config

import "errors"

var (
	ErrMissingAnthropicKey = errors.New("ANTHROPIC_API_KEY environment variable is required for anthropic provider")
	ErrMissingDeepSeekKey  = errors.New("DEEPSEEK_API_KEY environment variable is required for deepseek provider")
	ErrUnknownProvider     = errors.New("LLM_PROVIDER must be 'anthropic' or 'deepseek'")
	ErrMissingEmbedding    = errors.New("EMBEDDING_PROVIDER is required when using vector database")
)
