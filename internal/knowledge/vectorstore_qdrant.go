package knowledge

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/qdrant/go-client/qdrant"
)

// QdrantStore wraps a Qdrant vector database.
type QdrantStore struct {
	client         *qdrant.Client
	collectionName string
	dimensions     int
}

// QdrantConfig holds connection parameters for Qdrant.
type QdrantConfig struct {
	Host           string
	Port           int
	CollectionName string
	Dimensions     int
}

// NewQdrantStore creates a new Qdrant-backed vector store.
func NewQdrantStore(ctx context.Context, cfg QdrantConfig) (*QdrantStore, error) {
	client, err := qdrant.NewClient(&qdrant.Config{
		Host: cfg.Host,
		Port: cfg.Port,
	})
	if err != nil {
		return nil, fmt.Errorf("qdrant connect: %w", err)
	}

	store := &QdrantStore{
		client:         client,
		collectionName: cfg.CollectionName,
		dimensions:     cfg.Dimensions,
	}

	if err := store.ensureCollection(ctx); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("qdrant ensure collection: %w", err)
	}

	slog.Info("Qdrant store initialized",
		"collection", cfg.CollectionName,
		"dimensions", cfg.Dimensions,
		"host", cfg.Host,
	)

	return store, nil
}

func (s *QdrantStore) ensureCollection(ctx context.Context) error {
	exists, err := s.client.CollectionExists(ctx, s.collectionName)
	if err != nil {
		return fmt.Errorf("check collection: %w", err)
	}
	if exists {
		return nil
	}

	err = s.client.CreateCollection(ctx, &qdrant.CreateCollection{
		CollectionName: s.collectionName,
		VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     uint64(s.dimensions),
			Distance: qdrant.Distance_Cosine,
		}),
	})
	if err != nil {
		return fmt.Errorf("create collection: %w", err)
	}

	slog.Info("Created Qdrant collection", "name", s.collectionName)
	return nil
}

// VectorItem represents a single item to insert into the vector store.
type VectorItem struct {
	ID      string
	Vector  []float32
	Payload map[string]any
}

// Insert adds a single vector item.
func (s *QdrantStore) Insert(ctx context.Context, item VectorItem) error {
	return s.InsertBatch(ctx, []VectorItem{item})
}

// InsertBatch adds multiple vector items.
func (s *QdrantStore) InsertBatch(ctx context.Context, items []VectorItem) error {
	points := make([]*qdrant.PointStruct, 0, len(items))
	for _, item := range items {
		payload := make(map[string]*qdrant.Value)
		for k, v := range item.Payload {
			payload[k] = toQdrantValue(v)
		}

		points = append(points, &qdrant.PointStruct{
			Id:      qdrant.NewID(item.ID),
			Vectors: qdrant.NewVectors(item.Vector...),
			Payload: payload,
		})
	}

	_, err := s.client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: s.collectionName,
		Wait:           qdrant.PtrOf(true),
		Points:         points,
	})
	if err != nil {
		return fmt.Errorf("qdrant upsert: %w", err)
	}

	slog.Debug("Inserted vectors", "count", len(items))
	return nil
}

// SearchResult represents a single search hit.
type SearchResult struct {
	ID      string
	Score   float32
	Payload map[string]any
}

// Search performs a dense vector similarity search.
func (s *QdrantStore) Search(ctx context.Context, queryVector []float32, topK int) ([]SearchResult, error) {
	if topK <= 0 {
		topK = 5
	}

	limit := uint64(topK)
	results, err := s.client.Query(ctx, &qdrant.QueryPoints{
		CollectionName: s.collectionName,
		Query:          qdrant.NewQueryDense(queryVector),
		Limit:          &limit,
		WithPayload:    qdrant.NewWithPayload(true),
	})
	if err != nil {
		return nil, fmt.Errorf("qdrant query: %w", err)
	}

	searchResults := make([]SearchResult, 0, len(results))
	for _, r := range results {
		payload := make(map[string]any)
		for k, v := range r.Payload {
			payload[k] = fromQdrantValue(v)
		}
		searchResults = append(searchResults, SearchResult{
			ID:      r.Id.GetUuid(),
			Score:   r.Score,
			Payload: payload,
		})
	}

	return searchResults, nil
}

// Delete removes a vector by ID.
func (s *QdrantStore) Delete(ctx context.Context, id string) error {
	_, err := s.client.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: s.collectionName,
		Wait:           qdrant.PtrOf(true),
		Points:         qdrant.NewPointsSelector(qdrant.NewID(id)),
	})
	return err
}

// Size returns the number of vectors in the store.
func (s *QdrantStore) Size(ctx context.Context) (int, error) {
	info, err := s.client.GetCollectionInfo(ctx, s.collectionName)
	if err != nil {
		return 0, err
	}
	if info.PointsCount != nil {
		return int(*info.PointsCount), nil
	}
	return 0, nil
}

// Close releases the Qdrant connection.
func (s *QdrantStore) Close() error {
	return s.client.Close()
}

func toQdrantValue(v any) *qdrant.Value {
	switch val := v.(type) {
	case string:
		return &qdrant.Value{Kind: &qdrant.Value_StringValue{StringValue: val}}
	case int:
		return &qdrant.Value{Kind: &qdrant.Value_IntegerValue{IntegerValue: int64(val)}}
	case int64:
		return &qdrant.Value{Kind: &qdrant.Value_IntegerValue{IntegerValue: val}}
	case float64:
		return &qdrant.Value{Kind: &qdrant.Value_DoubleValue{DoubleValue: val}}
	case bool:
		return &qdrant.Value{Kind: &qdrant.Value_BoolValue{BoolValue: val}}
	default:
		return &qdrant.Value{Kind: &qdrant.Value_StringValue{StringValue: fmt.Sprintf("%v", v)}}
	}
}

func fromQdrantValue(v *qdrant.Value) any {
	switch v.Kind.(type) {
	case *qdrant.Value_StringValue:
		return v.GetStringValue()
	case *qdrant.Value_IntegerValue:
		return v.GetIntegerValue()
	case *qdrant.Value_DoubleValue:
		return v.GetDoubleValue()
	case *qdrant.Value_BoolValue:
		return v.GetBoolValue()
	default:
		return nil
	}
}
