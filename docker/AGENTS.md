# docker — 知识库容器（第 4 个镜像）

本目录只服务一件事：把灌好的 `doctor_knowledge` 做成**镜像即数据**的 MariaDB 容器。业务库不在这里（那是 compose 里的 `mariadb` 服务，用官方 `mariadb:11.4` + 唯一命名卷 `mariadb_data`）。

## 文件

| 文件 | 作用 |
|---|---|
| `Dockerfile.kb` | `FROM mariadb:11.4`（:26-29）：COPY `init-doctor_knowledge.sql.gz` 到 `/docker-entrypoint-initdb.d/01-*`、COPY `kb/99-kb-marker.sh`、COPY `kb/kb-entrypoint.sh` 作为入口 |
| `kb/kb-entrypoint.sh` | 启动时先清掉**自己打过标记**的数据目录再 `exec` 官方入口（:16-26）。这是"不挂持久卷也不复用旧知识"的真正保证 —— 因为 compose 重建容器时**会复用上个容器的匿名卷**，只改 compose 会静默继续跑旧知识 |
| `kb/99-kb-marker.sh` | dump 导完后 `touch /var/lib/mysql/.doctor-agent-kb-imported`（:6）。只有带这个标记的目录才会被清；别人初始化过的卷只告警不动（实测：外部建过表的卷挂进来，表数据完好） |
| `kb/init-doctor_knowledge.sql.gz` | 202M（2026-10-07 那份；删掉两份问答语料前是 639M），**gitignored**。由 `./update-kb.sh` 生成，或 `build.sh kb` 现场从 `doctor-kb-test` 容器导出。⚠️ `build_kb` 只在它**存在**时才跳过现场导出（`build.sh:209` 的 `-s` 测试），所以删除数据集/重播之后不手动换掉它，就会把上一版内容原样重发；那句现场导出还带 `2>/dev/null`，dump 挂了照样留下一个坏 gz 打进镜像 —— 换 dump 时先把它移走、自己导一份并**打包前**跑 `gzip -t` + 表清单 + 行数 + 死数据集名出现次数这四道检查 |
| `Dockerfile.kb.dockerignore`（在仓库根） | 白名单，只放行上面 3 个文件。**`docker/` 下新增文件必须在这里 `!` 加一行**，否则构建报 "failed to compute cache key ... not found" |

## 为什么这样设计

旧拓扑把知识库和业务库放同一实例同一持久卷，官方 entrypoint 只在数据目录为空时导入 —— 于是机器跑一段时间后，磁盘上躺的还是**首次启动那天**灌进去的知识，之后发布一概不生效。2026-10-02 拆分：`kb` 服务**不挂卷**，容器重建 = 重新导入。

实测结论（本机）：空数据目录 → 1.1GB / 1,385,585 行 → **45 秒**健康；命名卷被第二个容器复用时照样重灌（60 秒，日志有"清空上次导入"）。健康检查天然可靠，不需要改镜像 —— mariadb 官方 `healthcheck.sh --connect` 读 `@@skip_networking`，导入期返回 1，所以 `--connect --innodb_initialized` 在整份 dump 导完之前一直失败，app 有 `depends_on: {kb: {condition: service_healthy}}` 就不会读到半空库。

## 代价（如实记录）

`/admin` 的知识上传与 `sync-knowledge` 写的是 kb 容器的**临时存储**，容器重建即回落到镜像内容。永久改动必须做成新镜像：`make_gz.py` → `go run ./cmd/kbseed` → `./update-kb.sh`（`make_gz` 必须在最前，因为播种链路自己从 `gz/` 读）。这与"基础数据跟着镜像走"是同一个决定的两面，不是新 bug。

## 发布口径

`./build.sh kb` **只发 `:latest`**（version.json 的那一行只用于溯源打印，不做 tag）。发布前后核对：`doctor_knowledge` 只有 `kb_items` 一张表、`SELECT COUNT(*)` 行数、dump 文本里死数据集名出现 0 次。2026-10-04 已发布 `:latest = sha256:cff616eb85450784…`（knowledge 1.56.0，1,385,514 行 / 39 数据集）；**2026-10-07 重发 `:latest = sha256:ed66f9b0711187cc…`**（knowledge 1.57.0，780,833 行 / 37 数据集，`'medicalqa'`/`'huatuo'` 在 dump 里各 0 次）。发布后用推出去的镜像起一次性容器实测过导入: **26 秒**可查，`dataset IN ('medical','medicalqa','huatuo')` 只回 `medical` 7,325 行。两个现场踩到的坑: ① `docker run` 这个镜像**必须**给 `MARIADB_ROOT_PASSWORD`，否则官方入口直接报错退出（compose 里那句 env 不是可选的）；② 镜像本身**不带** healthcheck（检查写在 compose 的 `healthcheck:` 里），所以手工验证只能轮询 `SELECT 1`，`docker inspect` 的 `.State.Health` 是空的。
