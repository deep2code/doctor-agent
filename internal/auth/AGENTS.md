# internal/auth — 登录、口令与令牌

| 文件 | 作用 |
|---|---|
| `auth.go` | `Service`；`NewService` / `AdminCreateUser`（**仅管理员可建用户**）/ `Validate` / `Login` / `IssueToken` / `VerifyToken` / `GetUserByToken` / `GetUserByID` / `DeleteUser`；口令哈希 :282-307，旧哈希升级 :346-347 |

## 口令

PBKDF2-HMAC-SHA256，**210000 次迭代**，keyLen 32，自描述格式：

```
pbkdf2$sha256$<iters>$<saltHex>$<dkHex>
```

`verifyPassword` 对格式错误和 `iter<=0` 直接拒绝（不当作免费通过）。**旧格式行（16 hex salt + sha256(salt||password)）仍可登录**，首次成功登录时原地升级为 PBKDF2；升级写失败绝不能反过来拒绝这次登录。

**没有改密/重置口令的 API 是已定的产品决策**（2026-10-06，用户口径「不要改密端点」），不是待办项：`Service` 的导出面里 `AdminCreateUser` 只能**建**用户，`hashPassword`/`verifyPassword` 都是包内私有，全仓库没有任何一条"把已有用户的口令换成新口令"的路径。运维出路只有两条，`main.go` 的 `ADMIN_PASSWORD` 告警 hint 与 `.env.example:178-181` 说的就是这两条：设了 `ADMIN_PASSWORD` 再重启，或者删掉 `admin` 用户再重启（`createInitialAdmin` 每次启动按用户名 `admin` 查，缺了就重建）。

钉这条事实的门在**别的包**：`internal/server/admin_password_surface_test.go`（离线 AST，不需要 MariaDB）同时读 `handleAdminUser` 的方法集合与本包的导出名，所以它是**决策守卫**而不是缺口提醒 —— 谁补了改密路由或本包的改密方法它就红，红的意思是"你在推翻上面那个决定"。要推翻就先推翻决定，并连带翻 `main.go` hint、`.env.example` 与 `AGENTS.md`/`internal/{config,server}/AGENTS.md` 里引用这同一件事的口径，而不是删门。守卫故意比决定更严（任何导出名含 `Password` 都算，宽松版会放过 `RotateSecret` 这类改名实现）。

## 令牌

无状态 HMAC-SHA256 bearer：

```
base64url("<userID>|<expiryUnixSeconds>") + "." + hex(hmac(secret, payload))
```

`TokenTTL = 7d`（:31，构造 :163-170），比较用 `subtle.ConstantTimeCompare`（:193），签名密钥 = `config.AuthSecret`（`AUTH_SECRET`）。为空时随机生成每进程密钥（启动告警 + `SecurityWarnings` 一条）—— 本地开发可用，生产等于重启登出所有人、多实例互不认账。

`ErrInvalidToken` 同时覆盖坏签名、坏形状、过期三种情况，不给探测者区分线索。`GetUserByToken` 把**已删除账号**映射成 `ErrInvalidToken` —— 删号即吊销。这也是 `getAdminFromRequest` 第三步（按 bearer token 找管理员）真正生效的原因。

## 与 server 的分工

身份注入在 `internal/server/auth_api.go resolveCaller`，owner 解析在 `s.ownerOf(r)`。令牌授权的是**具体的人**；`API_KEY` 授权的是**部署**、解析到 owner `""`，两者绝不能混用（`internal/server/auth_api_test.go` 有"API_KEY 不是人"的断言）。

## 测试

`auth_test.go`（纯函数：哈希/令牌形状）、`auth_db_test.go`（PBKDF2 升级、删号即吊销 —— 需 MariaDB）。
