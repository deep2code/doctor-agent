package knowledge

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/qdrant/go-client/qdrant"
)

// VectorStore manages vector storage and retrieval using Qdrant.
type VectorStore struct {
	client     *qdrant.Client
	collection string
	dimensions int
}

// VectorStoreConfig holds Qdrant configuration.
type VectorStoreConfig struct {
	Host       string
	Port       int
	Collection string
	Dimensions int
}

// NewVectorStore creates a new Qdrant vector store.
func NewVectorStore(cfg VectorStoreConfig) (*VectorStore, error) {
	if cfg.Collection == "" {
		cfg.Collection = "medical_knowledge"
	}
	if cfg.Dimensions == 0 {
		cfg.Dimensions = 1024
	}

	client, err := qdrant.NewClient(&qdrant.Config{
		Host: cfg.Host,
		Port: cfg.Port,
	})
	if err != nil {
		return nil, fmt.Errorf("creating qdrant client: %w", err)
	}

	store := &VectorStore{
		client:     client,
		collection: cfg.Collection,
		dimensions: cfg.Dimensions,
	}

	// Connection is lazy: the collection is created on demand via EnsureCollection
	// (called by the syncer before upserting) so that agent startup never blocks
	// on Qdrant being reachable. When Qdrant is absent, retrieval simply falls
	// back to keyword search.
	slog.Info("Vector store client created (lazy)", "host", cfg.Host, "port", cfg.Port, "collection", cfg.Collection)
	return store, nil
}

// EnsureCollection creates the collection if it doesn't exist. Called by the
// syncer before upserting; safe to call even when the collection already exists.
func (s *VectorStore) EnsureCollection(ctx context.Context) error {
	return s.ensureCollection(ctx)
}

// ensureCollection creates the collection if it doesn't exist.
func (s *VectorStore) ensureCollection(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Check if collection exists
	exists, err := s.client.CollectionExists(ctx, s.collection)
	if err != nil {
		return fmt.Errorf("checking collection: %w", err)
	}

	if exists {
		return nil
	}

	// Create collection
	err = s.client.CreateCollection(ctx, &qdrant.CreateCollection{
		CollectionName: s.collection,
		VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     uint64(s.dimensions),
			Distance: qdrant.Distance_Cosine,
		}),
	})
	if err != nil {
		return fmt.Errorf("creating collection: %w", err)
	}

	slog.Info("Created Qdrant collection", "collection", s.collection, "dimensions", s.dimensions)
	return nil
}

// VectorPoint represents a point in vector space.
type VectorPoint struct {
	ID      string            `json:"id"`
	Vector  []float32         `json:"vector"`
	Payload map[string]string `json:"payload"`
}

// Upsert adds or updates vectors in the store.
func (s *VectorStore) Upsert(ctx context.Context, points []VectorPoint) error {
	if len(points) == 0 {
		return nil
	}

	qdrantPoints := make([]*qdrant.PointStruct, len(points))
	for i, p := range points {
		payload := make(map[string]*qdrant.Value)
		for k, v := range p.Payload {
			payload[k] = qdrant.NewValueString(v)
		}

		qdrantPoints[i] = &qdrant.PointStruct{
			Id:      qdrant.NewIDUUID(p.ID),
			Vectors: qdrant.NewVectors(p.Vector...),
			Payload: payload,
		}
	}

	_, err := s.client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: s.collection,
		Points:         qdrantPoints,
	})
	if err != nil {
		return fmt.Errorf("upserting points: %w", err)
	}

	return nil
}

// SearchQuery represents a search query.
type SearchQuery struct {
	Vector    []float32
	TopK      int
	Filter    map[string]string
	Threshold float64 // Minimum similarity score (0-1)
}

// SearchResult represents a search result.
type SearchResult struct {
	ID      string            `json:"id"`
	Score   float64           `json:"score"`
	Payload map[string]string `json:"payload"`
}

// Search performs vector similarity search.
func (s *VectorStore) Search(ctx context.Context, query SearchQuery) ([]SearchResult, error) {
	if query.TopK <= 0 {
		query.TopK = 5
	}
	if query.Threshold == 0 {
		query.Threshold = 0.5
	}

	// Build filter
	var filter *qdrant.Filter
	if len(query.Filter) > 0 {
		must := make([]*qdrant.Condition, 0)
		for k, v := range query.Filter {
			must = append(must, qdrant.NewMatchKeyword(k, v))
		}
		filter = &qdrant.Filter{Must: must}
	}

	// Convert threshold to float32
	threshold := float32(query.Threshold)

	req := &qdrant.QueryPoints{
		CollectionName: s.collection,
		Query:          qdrant.NewQuery(query.Vector...),
		Limit:          qdrant.PtrOf(uint64(query.TopK)),
		ScoreThreshold: &threshold,
		Filter:         filter,
		WithPayload:    qdrant.NewWithPayload(true),
	}

	resp, err := s.client.Query(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("searching vectors: %w", err)
	}

	results := make([]SearchResult, len(resp))
	for i, point := range resp {
		payload := make(map[string]string)
		if point.Payload != nil {
			for k, v := range point.Payload {
				if v.Kind != nil {
					if strVal, ok := v.Kind.(*qdrant.Value_StringValue); ok {
						payload[k] = strVal.StringValue
					}
				}
			}
		}

		results[i] = SearchResult{
			ID:      point.Id.GetUuid(),
			Score:   float64(point.Score),
			Payload: payload,
		}
	}

	return results, nil
}

// Delete removes points by IDs.
func (s *VectorStore) Delete(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	pointIDs := make([]*qdrant.PointId, len(ids))
	for i, id := range ids {
		pointIDs[i] = qdrant.NewIDUUID(id)
	}

	_, err := s.client.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: s.collection,
		Points: &qdrant.PointsSelector{
			PointsSelectorOneOf: &qdrant.PointsSelector_Points{
				Points: &qdrant.PointsIdsList{
					Ids: pointIDs,
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("deleting points: %w", err)
	}

	return nil
}

// Count returns the number of points in the collection.
func (s *VectorStore) Count(ctx context.Context) (int, error) {
	count, err := s.client.Count(ctx, &qdrant.CountPoints{
		CollectionName: s.collection,
	})
	if err != nil {
		return 0, fmt.Errorf("counting points: %w", err)
	}
	return int(count), nil
}

// WaitReady polls until Qdrant is reachable (or timeout). It retries a cheap
// gRPC healthcheck via the client connection so callers (e.g. the Dockerfile
// bake stage, which starts qdrant in the same build step) don't have to guess
// a sleep duration.
func (s *VectorStore) WaitReady(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		// CollectionExists is a cheap round trip; failing means Qdrant isn't
		// accepting requests yet.
		_, err := s.client.CollectionExists(ctx, s.collection)
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("qdrant not ready after %s: %w", timeout, err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

// Close closes the client connection.
func (s *VectorStore) Close() error {
	return s.client.Close()
}

// DeleteBySource removes all points with a specific source in their payload.
func (s *VectorStore) DeleteBySource(ctx context.Context, source string) error {
	filter := &qdrant.Filter{
		Must: []*qdrant.Condition{
			qdrant.NewMatchKeyword("source", source),
		},
	}

	_, err := s.client.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: s.collection,
		Points: &qdrant.PointsSelector{
			PointsSelectorOneOf: &qdrant.PointsSelector_Filter{
				Filter: filter,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("deleting points by source: %w", err)
	}

	return nil
}

// BatchUpsert performs batch upsert with progress callback.
func (s *VectorStore) BatchUpsert(ctx context.Context, points []VectorPoint, onProgress func(processed, total int)) error {
	if len(points) == 0 {
		return nil
	}

	const batchSize = 100
	total := len(points)

	for i := 0; i < total; i += batchSize {
		end := i + batchSize
		if end > total {
			end = total
		}
		batch := points[i:end]

		if err := s.Upsert(ctx, batch); err != nil {
			return fmt.Errorf("batch upsert at %d: %w", i, err)
		}

		if onProgress != nil {
			onProgress(end, total)
		}
	}

	return nil
}

// CountBySource returns the number of points with a specific source.
func (s *VectorStore) CountBySource(ctx context.Context, source string) (int, error) {
	filter := &qdrant.Filter{
		Must: []*qdrant.Condition{
			qdrant.NewMatchKeyword("source", source),
		},
	}

	count, err := s.client.Count(ctx, &qdrant.CountPoints{
		CollectionName: s.collection,
		Filter:         filter,
	})
	if err != nil {
		return 0, fmt.Errorf("counting points by source: %w", err)
	}

	return int(count), nil
}

// GetAllSources returns a list of all unique sources in the collection.
func (s *VectorStore) GetAllSources(ctx context.Context) ([]string, error) {
	// Scroll through all points to collect unique sources
	var sources []string
	sourceSet := make(map[string]bool)

	scrollReq := &qdrant.ScrollPoints{
		CollectionName: s.collection,
		Limit:          qdrant.PtrOf(uint32(1000)),
		WithPayload:    qdrant.NewWithPayload(true),
	}

	for {
		resp, nextOffset, err := s.client.ScrollAndOffset(ctx, scrollReq)
		if err != nil {
			return nil, fmt.Errorf("scrolling points: %w", err)
		}

		for _, point := range resp {
			if point.Payload != nil {
				if sourceVal, ok := point.Payload["source"]; ok {
					if sourceVal.Kind != nil {
						if strVal, ok := sourceVal.Kind.(*qdrant.Value_StringValue); ok {
							source := strVal.StringValue
							if !sourceSet[source] {
								sourceSet[source] = true
								sources = append(sources, source)
							}
						}
					}
				}
			}
		}

		if nextOffset == nil {
			break
		}

		// Move to next page
		scrollReq.Offset = nextOffset
	}

	return sources, nil
}

// GetSyncStats returns statistics about synced data.
func (s *VectorStore) GetSyncStats(ctx context.Context) (map[string]int, error) {
	sources, err := s.GetAllSources(ctx)
	if err != nil {
		return nil, err
	}

	stats := make(map[string]int)
	for _, source := range sources {
		count, err := s.CountBySource(ctx, source)
		if err != nil {
			return nil, err
		}
		stats[source] = count
	}

	return stats, nil
}
