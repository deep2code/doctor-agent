// Command vector-bake is a standalone, business-logic-free build tool.
//
// It bakes the gz knowledge sources directly into a Qdrant storage
// directory (offline, no MariaDB, no LLM required) and is used solely
// to produce the doctor-agent-qdrant RAG data image:
//
//	vector-bake [--src=internal/knowledge/gz] [--host=127.0.0.1] [--port=6334]
//	            [--collection=medical_knowledge] [--batch-size=100] [--workers=4]
//	            [--max-text-chars=1024] [--recreate] [--wait-green=600]
//
// Kept separate from the doctor-agent application binary on purpose:
// the RAG image only needs this small tool, so building it must NOT
// require building the whole app (see docker/qdrant-context/Dockerfile).
package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
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
	maxTextChars := 0 // 0 = bake default (1024 runes)
	recreate := false
	waitGreen := 0 // seconds to wait for collection status=green (0 = skip)

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
		case strings.HasPrefix(a, "--max-text-chars="):
			_ = sscanfInt(a, "--max-text-chars=", &maxTextChars)
		case a == "--recreate":
			recreate = true
		case strings.HasPrefix(a, "--wait-green="):
			_ = sscanfInt(a, "--wait-green=", &waitGreen)
		}
	}

	// Remote embedding is optional: with no base URL/key configured the
	// local deterministic hash embedder (1024 dims) is used — fully offline.
	baseURL := os.Getenv("EMBEDDING_BASE_URL")
	apiKey := os.Getenv("EMBEDDING_API_KEY")
	model := os.Getenv("EMBEDDING_MODEL")
	dimensions := 0
	if d := os.Getenv("EMBEDDING_DIMENSIONS"); d != "" {
		if v, err := strconv.Atoi(d); err == nil {
			dimensions = v
		}
	}

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

	embedder, err := embedding.NewDefault(baseURL, apiKey, model, dimensions)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 初始化 embedding 失败: %v\n", err)
		os.Exit(1)
	}

	// --recreate: drop the collection first so re-bakes start clean (stale
	// points from skipped/shrunk datasets would otherwise survive upserts).
	if recreate {
		fmt.Printf("   --recreate: deleting collection %s ...\n", collection)
		if err := vecStore.DeleteCollection(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  删除 collection 失败(可能不存在,继续): %v\n", err)
		}
	}

	res, err := knowledge.Bake(context.Background(), vecStore, embedder, knowledge.BakeConfig{
		GzDir:        gzDir,
		Collection:   collection,
		BatchSize:    batchSize,
		Workers:      workers,
		MaxTextChars: maxTextChars,
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

	// --wait-green: block until qdrant reports the collection green, i.e.
	// all WAL entries are flushed into segments. Best-effort: on timeout we
	// warn and continue — the hard gate is the point-count verification
	// below. (Under emulated builds optimization can exceed the timeout
	// while the data is in fact complete.)
	if waitGreen > 0 {
		fmt.Printf("   waiting up to %ds for collection to turn green ...\n", waitGreen)
		if err := vecStore.WaitGreen(context.Background(), time.Duration(waitGreen)*time.Second); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  %v (continuing — count verification follows)\n", err)
		} else {
			fmt.Printf("   collection is green — WAL fully flushed into segments\n")
		}
	}

	// Verify the upserts actually landed (ACKs alone are not proof of
	// durability — FUSE-mounted storage lost tail points in testing) and
	// re-run the idempotent upserts once if points went missing. A second
	// shortfall aborts the build: no crippled images.
	ensureBakedCount(context.Background(), vecStore, embedder, knowledge.BakeConfig{
		GzDir:        gzDir,
		Collection:   collection,
		BatchSize:    batchSize,
		Workers:      workers,
		MaxTextChars: maxTextChars,
	}, res.Points)
}

// ensureBakedCount verifies the stored point count matches what Bake
// reported and re-runs the (idempotent, deterministic-UUID) upserts once if
// points went missing. Still short after the re-bake -> exit 1.
func ensureBakedCount(ctx context.Context, vecStore *knowledge.VectorStore, embedder embedding.Provider, cfg knowledge.BakeConfig, baked int) {
	n, err := vecStore.Count(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  count check skipped: %v\n", err)
		return
	}
	if n >= baked {
		fmt.Printf("   verified: %d points stored\n", n)
		return
	}
	fmt.Printf("⚠️  stored %d < baked %d points — re-running idempotent upserts once\n", n, baked)
	if _, err := knowledge.Bake(ctx, vecStore, embedder, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "❌ re-bake: %v\n", err)
		os.Exit(1)
	}
	if n, err := vecStore.Count(ctx); err != nil || n < baked {
		fmt.Fprintf(os.Stderr, "❌ stored=%d baked=%d after re-bake — aborting (FUSE-mounted storage is a known cause of lost points)\n", n, baked)
		os.Exit(1)
	}
	fmt.Printf("   verified after re-bake: %d points stored\n", n)
}

// sscanfInt parses --key=VAL into *dst; returns error on failure.
func sscanfInt(arg, prefix string, dst *int) error {
	v := strings.TrimPrefix(arg, prefix)
	_, err := fmt.Sscanf(v, "%d", dst)
	return err
}

// embedderName reports which embedder will be used for bake.
func embedderName(baseURL, apiKey, model string) string {
	if baseURL != "" {
		name := model
		if name == "" {
			name = "text-embedding-v3"
		}
		if apiKey != "" {
			return name + " (remote)"
		}
		return name + " (local API)"
	}
	return "local-hash (offline, 1024d)"
}
