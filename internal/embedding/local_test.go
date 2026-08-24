package embedding

import (
	"math"
	"testing"
)

func TestLocalProviderDimensions(t *testing.T) {
	p, err := NewLocal(LocalConfig{Dimensions: 512})
	if err != nil {
		t.Fatalf("NewLocal: %v", err)
	}
	if p.Dimensions() != 512 {
		t.Fatalf("expected 512 dims, got %d", p.Dimensions())
	}
}

func TestLocalProviderNormalized(t *testing.T) {
	p, _ := NewLocal(LocalConfig{Dimensions: 1024})
	v, err := p.Embed("我喝牛奶就拉肚子 lactose intolerance")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(v) != 1024 {
		t.Fatalf("len = %d, want 1024", len(v))
	}
	var norm float64
	for _, x := range v {
		norm += float64(x) * float64(x)
	}
	if math.Abs(math.Sqrt(norm)-1) > 1e-4 {
		t.Fatalf("vector not L2-normalized: norm=%f", math.Sqrt(norm))
	}
}

func TestLocalProviderDeterministic(t *testing.T) {
	p, _ := NewLocal(LocalConfig{Dimensions: 1024})
	a, _ := p.Embed("地贫 基因检测")
	b, _ := p.Embed("地贫 基因检测")
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("embeddings not deterministic at dim %d", i)
		}
	}
}

func TestLocalProviderSimilarity(t *testing.T) {
	p, _ := NewLocal(LocalConfig{Dimensions: 1024})
	a, _ := p.Embed("我一喝牛奶就拉肚子")
	b, _ := p.Embed("喝牛奶之后腹泻")
	c, _ := p.Embed("今天天气真好适合爬山")

	sim := func(x, y []float32) float64 {
		var d float64
		for i := range x {
			d += float64(x[i]) * float64(y[i])
		}
		return d
	}

	if sim(a, b) <= sim(a, c) {
		t.Fatalf("expected related medical phrases to be closer than unrelated: ab=%.4f ac=%.4f", sim(a, b), sim(a, c))
	}
}

func TestNewDefaultFallsBackToLocal(t *testing.T) {
	// No remote credentials -> local provider.
	p, err := NewDefault("", "", "")
	if err != nil {
		t.Fatalf("NewDefault: %v", err)
	}
	if p.Name() != "local-hash" {
		t.Fatalf("expected local-hash, got %s", p.Name())
	}
}
