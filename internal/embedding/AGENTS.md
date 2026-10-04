# internal/embedding — 查询侧向量化

| 文件 | 作用 |
|---|---|
| `embedding.go` | `Provider` 接口、`Config`、`OpenAICompatProvider`、`NewOpenAICompat`、`NewDefault`、`Embed` / `EmbedBatch` / `Dimensions` / `Name` |

## 三条不可协商的事实

1. **没有任何离线回退**。`NewDefault` 要求 `EMBEDDING_BASE_URL`；未配置时 `agent.New` 打 warn 并把检索降级为 keyword-only，而 `sync-knowledge` 与 `vector-bake` 直接报错退出。
2. **查询端模型必须与 Qdrant 里烘焙向量同模型**（bge-m3）。模型不一致是**静默**摧毁语义召回的那种错 —— 检索照样返回结果，只是结果没有意义。
3. `EmbedBatch` 是批量入口：一轮内多条查询（QU 分支、追问实体）由 `knowledge.QueryPrewarmer` 走一次批量预热，避免逐条往返。

服务端口径：默认部署里查询侧 embedding 是**必需的编排组件**（compose 的 `embed` 服务，`EMBEDDING_BASE_URL=http://embed:18080/v1`），不是可选项。

## 测试

`embedding_test.go`。
