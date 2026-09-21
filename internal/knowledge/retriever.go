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

// QueryPrewarmer is an optional interface for retrievers that can
// pre-resolve expensive per-query resources (embedding service round-trips)
// for a whole batch of upcoming query strings in a single provider call.
// Callers fan out several Retrieve legs per turn (query-understanding
// branches, follow-up entity queries); prewarming collapses their per-query
// embedding calls into one. Implementations must be safe to call when some
// queries are already resolved, and non-fatal on failure (legs then embed
// their own query lazily, exactly as before).
type QueryPrewarmer interface {
	PrewarmQueries(queries []string)
}
