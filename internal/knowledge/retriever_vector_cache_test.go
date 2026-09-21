package knowledge

import (
	"context"
	"sync"
	"testing"
	"time"
)

type countingEmbedder struct {
	mu    sync.Mutex
	calls int
}

func (e *countingEmbedder) Embed(_ string) ([]float32, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls++
	return []float32{0.1}, nil
}

func (e *countingEmbedder) Dimensions() int { return 1 }

func (e *countingEmbedder) count() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.calls
}

// TestVectorRetrieverCachesQueryEmbeddings: the same query text repeated
// (base leg + branch legs send identical strings through one turn) must hit
// the embedding service only once. The store points at a closed port so
// Search fails fast — embedding happens before it, which is exactly the
// call site under test.
func TestVectorRetrieverCachesQueryEmbeddings(t *testing.T) {
	vs, err := NewVectorStore(VectorStoreConfig{Host: "127.0.0.1", Port: 1, Collection: "test", Dimensions: 1})
	if err != nil {
		t.Fatalf("NewVectorStore: %v", err)
	}
	emb := &countingEmbedder{}
	r := NewVectorRetriever(vs, emb, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for i := 0; i < 3; i++ {
		if _, err := r.Retrieve(ctx, "宝宝拉肚子发烧", 5); err == nil {
			t.Fatal("预期连不上 Qdrant 时 Search 报错")
		}
	}
	if _, err := r.RetrieveDrugs(ctx, "宝宝拉肚子发烧", 5); err == nil {
		t.Fatal("预期连不上 Qdrant 时 Search 报错")
	}
	if got := emb.count(); got != 1 {
		t.Errorf("同一查询串应只 embedding 一次，实际 %d 次", got)
	}
}

// batchEmbedder implements Embedder + BatchEmbedder and counts each surface.
type batchEmbedder struct {
	mu        sync.Mutex
	singles   int
	batchReqs int
	batchText int
}

func (e *batchEmbedder) Embed(_ string) ([]float32, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.singles++
	return []float32{0.1}, nil
}

func (e *batchEmbedder) EmbedBatch(texts []string) ([][]float32, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.batchReqs++
	e.batchText += len(texts)
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = []float32{0.2}
	}
	return out, nil
}

func (e *batchEmbedder) Dimensions() int { return 1 }

// TestPrewarmQueriesSingleBatch: prewarming dedupes (and drops cached/empty
// strings), then the retrieval legs find vectors in the cache and issue no
// individual Embed calls.
func TestPrewarmQueriesSingleBatch(t *testing.T) {
	vs, err := NewVectorStore(VectorStoreConfig{Host: "127.0.0.1", Port: 1, Collection: "test", Dimensions: 1})
	if err != nil {
		t.Fatalf("NewVectorStore: %v", err)
	}
	emb := &batchEmbedder{}
	r := NewVectorRetriever(vs, emb, nil)

	// warm once with dupes + empty
	r.PrewarmQueries([]string{"q1", "q2", "q1", ""})
	// second prewarm fully cached → no new request
	r.PrewarmQueries([]string{"q1", "q2"})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, q := range []string{"q1", "q2", "q1"} {
		if _, err := r.Retrieve(ctx, q, 5); err == nil {
			t.Fatal("预期连不上 Qdrant 时 Search 报错")
		}
	}

	if emb.batchReqs != 1 || emb.batchText != 2 {
		t.Errorf("应恰好 1 次批量请求、2 条去重文本，实际 %d 次 / %d 条", emb.batchReqs, emb.batchText)
	}
	if emb.singles != 0 {
		t.Errorf("预热命中后不应再发单条 embedding，实际 %d 次", emb.singles)
	}
}

// prewarmingRetriever records PrewarmQueries batches for the hybrid test.
type prewarmingRetriever struct {
	inner   Retriever
	mu      sync.Mutex
	batches [][]string
}

func (p *prewarmingRetriever) Retrieve(ctx context.Context, q string, k int) ([]RetrievalResult, error) {
	return p.inner.Retrieve(ctx, q, k)
}

func (p *prewarmingRetriever) RetrieveDrugs(ctx context.Context, q string, k int) ([]DrugRetrievalResult, error) {
	return p.inner.RetrieveDrugs(ctx, q, k)
}

func (p *prewarmingRetriever) Name() string { return "prewarming" }

func (p *prewarmingRetriever) PrewarmQueries(qs []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.batches = append(p.batches, append([]string(nil), qs...))
}

// TestHybridPrewarmDelegatesToVectorLeg: the hybrid retriever routes the
// batch to the vector retriever only.
func TestHybridPrewarmDelegatesToVectorLeg(t *testing.T) {
	vec := &prewarmingRetriever{inner: &mapRetriever{}}
	h := NewHybridRetriever(&mapRetriever{}, vec, 0.4)
	h.PrewarmQueries([]string{"a", "b"})
	if len(vec.batches) != 1 || len(vec.batches[0]) != 2 {
		t.Errorf("向量腿应收到 1 批 2 条查询，实际: %v", vec.batches)
	}
	// keywordRetriever field is a plain mapRetriever (no prewarm) — the
	// delegation must not panic and nothing else to assert.
}

func TestQueryVectorCacheEvictsLRU(t *testing.T) {
	c := newQueryVectorCache(2)
	c.put("a", []float32{1})
	c.put("b", []float32{2})
	if _, ok := c.get("a"); !ok {
		t.Fatal("a 应仍在缓存")
	}
	c.put("c", []float32{3}) // evicts b (a was just touched)
	if _, ok := c.get("b"); ok {
		t.Error("最久未用的 b 应已淘汰")
	}
	if _, ok := c.get("a"); !ok {
		t.Error("刚命中的 a 不应被淘汰")
	}
	if _, ok := c.get("c"); !ok {
		t.Error("c 应在缓存")
	}
}
