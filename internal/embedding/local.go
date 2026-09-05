package embedding

import (
	"hash/fnv"
	"math"
	"strings"
	"unicode"
)

// LocalConfig configures the in-process lexical embedding provider.
type LocalConfig struct {
	// Dimensions is the size of the output vector. Default 1024.
	Dimensions int
}

// LocalProvider is a dependency-free, model-free embedding provider. It encodes
// text as a hashed bag-of-words (CJK character unigrams/bigrams plus Latin word
// tokens) into a fixed-dimensional L2-normalized vector. Cosine similarity
// between two such vectors approximates lexical overlap, giving "weak semantic"
// recall with zero external services, model weights, or network calls — so the
// vector retrieval path works fully offline (e.g. against a local Qdrant).
type LocalProvider struct {
	dim int
}

// NewLocal creates a LocalProvider.
func NewLocal(cfg LocalConfig) (*LocalProvider, error) {
	dim := cfg.Dimensions
	if dim <= 0 {
		dim = 1024
	}
	return &LocalProvider{dim: dim}, nil
}

// Dimensions returns the vector size.
func (p *LocalProvider) Dimensions() int { return p.dim }

// Name identifies the provider.
func (p *LocalProvider) Name() string { return "local-hash" }

// Embed encodes a single text into a vector.
func (p *LocalProvider) Embed(text string) ([]float32, error) {
	vec := make([]float32, p.dim)
	for _, tok := range tokenizeLexical(text) {
		h := fnvHash32(tok) % uint32(p.dim)
		vec[h] += 1.0 // term frequency (sublinear TF kept simple for speed)
	}
	l2Normalize(vec)
	return vec, nil
}

// EmbedBatch encodes multiple texts.
func (p *LocalProvider) EmbedBatch(texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		v, err := p.Embed(t)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

// NewDefault selects an embedding provider: an OpenAI-compatible provider
// when baseURL is configured (apiKey optional — local services like Ollama
// need no key), otherwise the in-process local hash provider (no model, no
// network). dimensions >0 requests a specific output dimension (needed for
// Zhipu embedding-3-pro which defaults to 2048 but can return 1024).
func NewDefault(baseURL, apiKey, model string, dimensions int) (Provider, error) {
	if baseURL != "" {
		return NewOpenAICompat(Config{
			BaseURL:    baseURL,
			APIKey:     apiKey,
			Model:      model,
			Dimensions: dimensions,
		})
	}
	dim := dimensions
	if dim <= 0 {
		dim = 1024
	}
	return NewLocal(LocalConfig{Dimensions: dim})
}

func l2Normalize(v []float32) {
	var sum float32
	for _, x := range v {
		sum += x * x
	}
	if sum <= 0 {
		return
	}
	norm := float32(math.Sqrt(float64(sum)))
	for i := range v {
		v[i] /= norm
	}
}

func fnvHash32(s string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return h.Sum32()
}

// tokenizeLexical splits text into CJK character unigrams, adjacent CJK bigrams,
// and lowercased Latin/number word tokens.
func tokenizeLexical(text string) []string {
	var tokens []string
	runes := []rune(text)
	var latin strings.Builder

	flush := func() {
		if latin.Len() == 0 {
			return
		}
		for _, w := range strings.FieldsFunc(latin.String(), func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsNumber(r)
		}) {
			if w = strings.ToLower(w); w != "" {
				tokens = append(tokens, w)
			}
		}
		latin.Reset()
	}

	for i, r := range runes {
		if isCJK(r) {
			flush()
			tokens = append(tokens, string(r))
			if i+1 < len(runes) && isCJK(runes[i+1]) {
				tokens = append(tokens, string(r)+string(runes[i+1]))
			}
		} else if unicode.IsLetter(r) || unicode.IsNumber(r) {
			latin.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return tokens
}

func isCJK(r rune) bool {
	return r >= 0x3400 && r <= 0x4DBF || // CJK Extension A
		r >= 0x4E00 && r <= 0x9FFF || // CJK Unified Ideographs
		r >= 0xF900 && r <= 0xFAFF || // CJK Compatibility Ideographs
		r >= 0x3040 && r <= 0x30FF || // Hiragana + Katakana
		r >= 0xAC00 && r <= 0xD7AF // Hangul Syllables
}
