# internal/rerank — 交叉编码器重排（默认关闭）

| 文件 | 作用 |
|---|---|
| `rerank.go` | `Provider`、`New`、`Rerank`（外加 `UnmarshalJSON` 兼容多种响应形状） |

## 行为

向 TEI 风格端点 `POST {model, query, texts}`，返回按位置排序的分数；同时吸收 Cohere / vLLM 的 `{"results":[…]}` 形状。**客户端超时 2s**。满足 `knowledge.Reranker`。

## 接线与降级（两条都要保住）

- 只在 `RERANK_ENABLED` **且** `RERANK_BASE_URL` 都设置时，`agent.New` 才注入；否则传 nil，融合结果保持 RRF 原序。
- **失败静默降级**：reranker 挂了绝不能挡住回答。`retrieveWithUnderstanding` 在每个出口（基础 / QU 跳过 / 合并分支）都重排并截到 `KNOWLEDGE_TOP_K`；`Knowledge.RerankCandidates` 负责重排融合池。存在 reranker 时**过取 2×topK** 才有东西可排。

回归门：`internal/agent/rerank_wiring_test.go`（接线条件）、本包 `rerank_test.go`（响应形状解析）。
