package knowledge

import "context"

// Retriever is the interface that all knowledge retrieval backends must implement.
// Different implementations provide keyword-based, vector-based, or hybrid retrieval.
type Retriever interface {
	// Retrieve searches the knowledge base for entries matching the query.
	// Returns up to topK results sorted by relevance score (descending).
	Retrieve(ctx context.Context, query string, topK int) ([]RetrievalResult, error)

	// RetrieveDrugs searches drug entries matching the query.
	RetrieveDrugs(ctx context.Context, query string, topK int) ([]DrugRetrievalResult, error)

	// Name returns a human-readable identifier for this retriever.
	Name() string
}
