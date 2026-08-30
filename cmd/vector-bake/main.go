// Command vector-bake is a standalone, business-logic-free build tool.
//
// It bakes the gz knowledge sources directly into a Qdrant storage
// directory (offline, no MariaDB, no LLM required) and is used solely
// to produce the doctor-agent-qdrant RAG data image:
//
//	vector-bake [--src=internal/knowledge/gz] [--host=127.0.0.1] [--port=6334]
//	            [--collection=medical_knowledge] [--batch-size=100] [--workers=4]
//
// Kept separate from the doctor-agent application binary on purpose:
// the RAG image only needs this small tool, so building it must NOT
// require building the whole app (see docker/qdrant-context/Dockerfile).
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/doctor-agent/internal/embedding"
	"github.com/doctor-agent/internal/knowledge"
)

func main() {
	gzDir := "internal/knowledge/gz"
	host := "localhost"
	port := 6334
	collection := "medical_knowledge"
	batchSize := 100
	workers := 4

	for _, a := range os.Args[1:] {
		switch {
		case strings.HasPrefix(a, "--src="):
			gzDir = strings.TrimPrefix(a, "--src=")
		case strings.HasPrefix(a, "--host="):
			host = strings.TrimPrefix(a, "--host=")
		case strings.HasPrefix(a, "--port="):
			_ = sscanfInt(a, "--port=", &port)
		case strings.HasPrefix(a, "--collection="):
			collection = strings.TrimPrefix(a, "--collection=")
		case strings.HasPrefix(a, "--batch-size="):
			_ = sscanfInt(a, "--batch-size=", &batchSize)
		case strings.HasPrefix(a, "--workers="):
			_ = sscanfInt(a, "--workers=", &workers)
		}
	}

	// Remote embedding is optional: with no base URL/key configured the
	// local deterministic hash embedder (1024 dims) is used — fully offline.
	baseURL := os.Getenv("EMBEDDING_BASE_URL")
	apiKey := os.Getenv("EMBEDDING_API_KEY")
	model := os.Getenv("EMBEDDING_MODEL")

	fmt.Printf("🧱 离线烘焙知识库 gz -> Qdrant\n")
	fmt.Printf("   gz 源:       %s\n", gzDir)
	fmt.Printf("   Qdrant:      %s:%d (%s)\n", host, port, collection)
	fmt.Printf("   embedding:   %s\n", embedderName(baseURL, apiKey, model))

	vecStore, err := knowledge.NewVectorStore(knowledge.VectorStoreConfig{
		Host:       host,
		Port:       port,
		Collection: collection,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 连接 Qdrant 失败: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = vecStore.Close() }()

	fmt.Printf("   waiting for Qdrant at %s:%d ...\n", host, port)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	if err := vecStore.WaitReady(ctx, 120*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Qdrant 未就绪: %v\n", err)
		os.Exit(1)
	}

	embedder, err := embedding.NewDefault(baseURL, apiKey, model)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 初始化 embedding 失败: %v\n", err)
		os.Exit(1)
	}

	res, err := knowledge.Bake(context.Background(), vecStore, embedder, knowledge.BakeConfig{
		GzDir:      gzDir,
		Collection: collection,
		BatchSize:  batchSize,
		Workers:    workers,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 烘焙失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ 烘焙完成: %d 个数据集, %d 个向量点, 用时 %s\n", res.Datasets, res.Points, res.Duration)
	if len(res.Errors) > 0 {
		fmt.Fprintf(os.Stderr, "⚠️  %d 个批次错误(可重跑覆盖):\n", len(res.Errors))
		for _, e := range res.Errors[:min(10, len(res.Errors))] {
			fmt.Fprintf(os.Stderr, "   - %s\n", e)
		}
	}
}

// sscanfInt parses --key=VAL into *dst; returns error on failure.
func sscanfInt(arg, prefix string, dst *int) error {
	v := strings.TrimPrefix(arg, prefix)
	_, err := fmt.Sscanf(v, "%d", dst)
	return err
}

// embedderName reports which embedder will be used for bake.
func embedderName(baseURL, apiKey, model string) string {
	if baseURL != "" && apiKey != "" {
		return model + " (remote)"
	}
	return "local-hash (offline, 1024d)"
}
