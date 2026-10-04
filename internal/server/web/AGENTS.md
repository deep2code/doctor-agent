# internal/server/web — 内嵌前端

单文件页面，零外部依赖、可完全离线。每个文件都是一条独立的 `//go:embed` 指令（在 `internal/server/server.go:35-89`），**新增文件必须同时补 embed 指令和路由**，否则不进二进制。

## 页面

| 文件 | 路由 | 作用 |
|---|---|---|
| `landing.html` (98KiB) | `/` | 营销落地页（公开） |
| `index.html` (165KiB) | `/app` | 咨询台聊天 UI：SSE 流式、服务端会话、家庭成员档案、长辈模式、侧栏、Markdown + mermaid 渲染、3D 解剖组件、`#acct*` 账号行（页脚） |
| `map.html` (247KiB) | `/map` | 医疗机构地图页 |
| `stats.html` (34KiB) | `/stats` | 公开统计页 |
| `share.html` (7KiB) | `/share/{token}` | 只读分享页 |
| `admin.html` (31KiB) | `/admin` | Basic-auth 控制台 |
| `shared/base.css` / `bm.css` / `map.css` / `stats.css` | — | 各自页面样式 |

## 懒加载 JS（不进首屏 HTML）

`mermaid.min.js`（2.7MB）、`three.min.js`（589KB）、`anatomy.js` + `anatomy-anim.js`（3D 解剖）、`qrcode.min.js`、`favicon.ico`。`/media/*` 由磁盘目录提供（`MEDIA_DIR`）。

## 前端的三条约定

1. **鉴权靠一层 `window.fetch` 包装**：在任何业务代码之前安装，只处理同源调用，给每个请求补 `Authorization: Bearer <localStorage["auth-token"]>`，因此 `/sessions*`、`/family*`、`/share`、`/chat/stream` 都不需要逐个改。`/login` 与 `/me` 走捕获的 `rawFetch`（不把过期 token 重放到登录）；`/me` 返回 404（无库部署）就整行隐藏账号区；401 丢弃死 token；`onAuthChanged` 在切换身份时把 UI 重置成全新会话。
2. **`conversation_id` 必须由客户端用 `crypto.getRandomValues` 生成** —— 它本身就是访问凭证，不能猜测或复用他人 id（服务端对陌生 id 返回 404）。
3. **UI 文案一律中文**，不要把领域内容翻成英文。

## 已知残留

`landing.html.bak`（110KiB）是 Initial commit 遗留、未被任何 `//go:embed` 引用，可删。

改动这些页面没有构建步骤，也不该引入构建步骤；直接编辑单文件，`go build ./...` 后 `go run .` 起来在浏览器里看。UI 改动要在浏览器里实测过再算完成。
