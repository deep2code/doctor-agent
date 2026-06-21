package knowledge

import "context"

// Embedder converts text to dense vector representations.
// Different implementations support Voyage AI, DeepSeek, OpenAI, or local models.
type Embedder interface {
	// Embed converts a single text to a vector.
	Embed(ctx context.Context, text string) ([]float32, error)

	// EmbedBatch converts multiple texts to vectors in a single call.
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)

	// Dimensions returns the embedding dimension (e.g., 1024 for voyage-multilingual-2).
	Dimensions() int

	// Name returns the embedder identifier for logging.
	Name() string
}
