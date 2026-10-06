# internal/server — HTTP 服务

stdlib `net/http`，默认 `0.0.0.0:7071`（`SERVER_HOST`/`SERVER_PORT`）。`server.go` 是路由与中间件主干，三个 `*_api.go` 是按域拆出的 handler 组。

## 路由（`server.go:157-220`，按注册条件分组）

**公开 / 懒静态**（:157-172）：`/`(landing.html)、`/app`(index.html)、`/map`、`/stats`、`/share` 前端页除外、`/robots.txt`、`/sitemap.xml`、`/llms.txt`、`/favicon.ico`、`/media/*`、`/mermaid.min.js`、`/three.min.js`、`/anatomy*.js`、`/qrcode.min.js`、`/health`。

**对话**（:173-175）：`POST /chat`、`POST /chat/stream`（真 token 级 SSE：`delta`/`done`/`error`）、`POST /feedback`。

**仅当业务库存在（db != nil）才注册**（:176-186）：`/login`、`/me`、`/sessions`、`/sessions/`、`/family`、`/family/`、`/share`、`/share/`。方法在各 handler 内部判定：`auth_api.go:115` POST /login、`:152` GET /me；`family_api.go:27/39` GET+POST、`:81-110` GET/PUT/PATCH/DELETE；`share_api.go:58` POST、`:137` GET。

**admin**（:188-220，共 24 条）：`/admin`（Basic-auth 控制台）+ users / sessions / knowledge（含 stats、versions、export）/ sync + sync/status / feedback / audit-logs / config / api-stats / analytics / batch / export。

`publicPaths`（`server.go:1805-1814`，逐字）：`/health`、`/`、`/index.html`、`/map`、`/stats`、`/robots.txt`、`/sitemap.xml`、`/llms.txt`。`loginPath`（:1816-1819）只免鉴权、**不免限流**。注意 `/app` **不在** publicPaths —— 设了 `API_KEY` 时它直接 401，所以 Web UI 的设计前提是"无 API_KEY 或鉴权在反代终结"。

## 中间件顺序（:1824-1863，顺序即语义，别重排）

trusted-proxy client-IP 解析（`TRUSTED_PROXIES`，非白名单对端一律忽略 `X-Forwarded-For`）→ 登录令牌身份注入（`auth_api.go resolveCaller` → context `callerKey{}`）→ CORS 头 → OPTIONS 短路 204 → 非 public 路径：限流（`RATE_LIMIT` 默认 120/min/IP，固定窗口）→ Bearer 鉴权门 → handler → slog。

## 两个必须继续遵守的约定

1. **所有 5xx 走 `internalError`（:1293-1299），绝不把 `err.Error()` 发给客户端**。
2. **所有 JSON body 经 `decodeJSONBody`（:1318）带上限**：`jsonBodyLimit` 1MiB、`chatBodyLimit` 10MiB、`batchBodyLimit` 32MiB、`smallBodyLimit` 4KiB（feedback/share），定义在 :1306-1311；multipart 另有真实总大小上限。

`/health`（真检，:2325 一带）：app 库 ping + 知识层 reachable/seeded（`knowledge.Load()` / `store.Health()`），依赖异常时 `status:"degraded"` 并带 `checks` map，但**仍返回 HTTP 200**，方便探针保持简单。

## 归属与安全

- 凡触碰个人数据的端点，owner 只从凭证来：`s.ownerOf(r)`，**永不读请求体**。`conversation_id` 本身就是访问凭证。
- `claimConversation`（`auth_api.go:166-175`）是 `/chat`、`/chat/stream`、`/share` 的唯一入口；**别人的会话返回 404 而不是 403**，让 id 不可被探测（回归断言 `auth_api_test.go:263-295`）。
- `API_KEY` 授权的是**部署**，解析到 owner `""`（故意如此：共享密钥不该能解开某个病人的历史）；登录 token 才解析到具体的人。
- **`/admin` 没有改密入口是已定的产品决策**（2026-10-06 用户口径「不要改密端点」），不是待办：`handleAdminUser` 只有 GET/DELETE，`admin.html` 只有 `#login-wrap` 那个登录表单（:87-94，处理器 :384-394），全页再无第二处口令输入。运维文案（`main.go:404-406` 的 `ADMIN_PASSWORD` hint、`.env.example:178-181`）就建立在这个事实上，由 `admin_password_surface_test.go` 钉住（见下面「测试」）。口令丢了的重建路径确实走得通：`main.go createInitialAdmin`（:375-386）每次启动都按用户名 `admin` 查一次，不存在就带着 `ADMIN_PASSWORD` 重建。
- 个人行的 owner 列可能是 `NULL`（匿名），过滤条件必须写 `COALESCE(user_id,'') = ?` —— 直接比 `''` 匹配不到任何行，会把匿名桶交给错误的调用方（见 `internal/database`）。

## 前端

`internal/server/web/` 是单文件页面，各自独立 `//go:embed`（`server.go:35-89`）：`landing.html` / `index.html`(165KiB，咨询台) / `map.html` / `stats.html` / `share.html` / `admin.html` + `shared/{base,map,stats,bm}.css` + 懒加载 JS（`mermaid.min.js` 2.7MB、`three.min.js` 589KB、`anatomy.js`、`anatomy-anim.js`、`qrcode.min.js`）+ `favicon.ico`。零外部依赖、可离线。

`index.html` 的鉴权接线：页内一层 `window.fetch` 包装（在任何业务代码之前安装，只处理同源调用）给每个请求补 `Authorization: Bearer`，因此 `/sessions*`、`/family*`、`/share`、`/chat/stream` 无需逐个改；`/login` 与 `/me` 走捕获的 `rawFetch`，避免把过期 token 回放到登录；`/me` 404（无库部署）会整行隐藏；401 丢弃死 token；切换身份会重置成全新会话（可见列表本身是 owner-scoped）。

⚠️ `web/landing.html.bak`（110KB）是 Initial commit 遗留的**未引用文件**，不在任何 `//go:embed` 里，可删。

## 测试

`server_test.go`（含 `TestWebUIServed`）、`auth_api_test.go`（登录 + owner 隔离 + "API_KEY 不是人"）、`admin_api_test.go`（上传文件名即数据集名的两种断言）、`session_api_test.go`、`family_api.go` 相关、`pdf_export_test.go`、`admin_password_surface_test.go`（**离线**：AST 读 `handleAdminUser` 的方法集合必须只有 GET/DELETE，且 `internal/auth` 不得有导出名含 `Password` 的方法 —— 钉住「/admin 没有改密入口」这个事实，`main.go` 的 `ADMIN_PASSWORD` 告警 hint 与 `.env.example` 都靠它）。需要 MariaDB 的测试在本地要带 `MARIA_DB_PORT=3307`。
