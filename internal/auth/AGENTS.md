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
