# internal/database — 业务库（MariaDB，纯 Go）

`go-sql-driver/mysql`，无 CGO。库 = `doctor_agent`（users / sessions / messages / feedback / family_members 等 10 张表）。**知识库里绝不该出现这些表** —— 知识层在 `internal/knowledge`，另一个容器。

| 文件 | 作用 |
|---|---|
| `db.go` | `DB` / `Config` / `User`；`New()` 一连上就跑 `migrate()` 建表；`Close` / `Health` / `Stats`；用户 CRUD（`CreateUser` / `GetUser` / `GetUserByUsername` / `UpdateUserLastLogin` / `UpdateUserPasswordHash`）；会话行：`SetSessionOwner:424`（**`WHERE id=? AND user_id IS NULL`**，只绑未认领的行）、owner 过滤一律 `COALESCE(user_id,'') = ?`（:389、:441、:478） |
| `family.go` | 家庭成员档案 CRUD。`ListFamilyMembers:81` / `GetFamilyMember` / `UpdateFamilyMember` / `DeleteFamilyMember:163,170` 全部把 owner 当**入参**，**没有超级用户逃逸**（空 owner = 匿名桶，不是"看所有"） |
| `share.go` | `ShareSnapshot` / `CreateShare` / `GetShare` —— 分享链接快照 |

## 两条会静默出错事的规则

1. **`COALESCE(user_id,'') = ?`，不是 `= ''`**。匿名行的列是 `NULL`（外键指向 `users(id)` 禁止写 `""`），直接比 `''` 匹配不到任何东西，等于把匿名桶交给错误的调用方 —— 不报错，只是返回空。
2. **本层测试永远不要连知识库**。2026-09-21 的事故：`family_test.go` 把测试库指向 `MARIA_DB_KNOWLEDGE_DB`（默认 `doctor_knowledge`），`New()` 的 `migrate()` 就把 10 张业务表建进了知识库，而 `update-kb.sh` 的 `mariadb-dump --databases doctor_knowledge` 会把它们（可能含真人数据）打进发布镜像。现在的测试用独立库 `doctor_agent_test_db`，并在 `MARIA_DB_APP_DB` 与知识库同名时直接 skip。发布门在 `update-kb.sh` 第 2 步：`doctor_knowledge` 只允许有 `kb_items`。

## 测试

`family_test.go`（含 `TestFamilyMemberUserIsolation`）。跑测试要带 `MARIA_DB_PORT=3307`（dev 容器 `doctor-kb-test`）。
