# Doctor Agent — 循证医学AI助手

权威、专业、最高水平的医生智能体，专为**全中国人群**定制。

## 🎯 核心特性

- **纯循证医学**：所有回答基于已发表文献和临床指南，拒绝中医、偏方及未经科学验证的说法
- **Zero Hallucination**：三重反幻觉机制——提示词约束 + 知识库绑定 + 响应后引用验证
- **每条回答有据可查**：事实性陈述必须标注引用编号 `[N]`，附DOI/PMID和证据等级
- **全人群覆盖**：覆盖地贫、G6PD缺乏症、鼻咽癌、乙肝、乳糖不耐受、ALDH2酒精代谢缺陷等中国重点高发疾病
- **4层安全防护**：紧急检测(零延迟120响应) → 范围检查 → 引用验证 → 免责声明
- **超大知识库**：78 个数据源（知识库版本 v1.39.0）——WHO/国家临床 ICD-10+ICD-11/HPO/Orphanet/ICD-O-3 编码库、60 万+医患问答、10 个统一语料全文源（StatPearls/MedlinePlus Genetics/LactMed/精神障碍诊疗规范/急救手册/旅行医学…）、13 个活跃工具

## 📋 中国重点疾病覆盖

| 疾病/特征 | 中国流行情况 / 高发地区 | 知识库覆盖 |
|-----------|-----------|-----------|
| α-地中海贫血 | 广西14.95%, 广东8.53% | ✅ 诊断+治疗+遗传咨询 |
| β-地中海贫血 | 广西6.78%, 广东4.53% | ✅ 诊断+治疗+遗传咨询 |
| G6PD缺乏症 | 南宁17.45%, 广东4% | ✅ 药物禁忌+诱因+管理 |
| 鼻咽癌(NPC) | 广东/广西/香港 ASR 20-30/10万 | ✅ EBV筛查+风险因素 |
| 慢性乙肝 | 全国约5-6%（南方偏高） | ✅ 诊断+抗病毒治疗 |
| 乳糖不耐受 | 中国成人>80% | ✅ 诊断+饮食管理 |
| ALDH2缺乏 | 东亚~36% | ✅ 酒精-癌症风险 |
| 登革热 | 南方省份为主（广东/广西/海南/云南） | ✅ 诊断+分诊 |
| 湿疹/真菌感染 | 全国常见（南方湿热地区更易发） | ✅ 诊断+治疗 |

## 📚 知识库规模

| 数据源 | 数据量 | 说明 |
|--------|--------|------|
| 医学知识条目 | 423 | 核心疾病 + 常见病 60 + 老年/妇产/儿保 62 + WHO 官方中文 fact sheets 232 + 疫苗立场文件 12 + 中国疾控科普 26 + 免疫规划/喂养指南 14 |
| 卫健委诊疗指南全文 | 47 | 传染病/常见病诊疗方案中文全文（含 OCR 版） |
| ICD-10 疾病编码 | 35,862 | 国家临床版2.0 |
| ICD-11 MMS 编码 | 35,339 | WHO 2025-01 中文版，含 ICD-10 映射 |
| HPO 人类表型本体 | 19,836 | 中英表型名+同义词 |
| Orphanet 罕见病 | 11,647 | 中英名+ICD-10/11 映射 |
| ICD-O-3 肿瘤形态学 | 1,077 | WHO 第3版 8000/0–9992/3 |
| 医保药品目录 | 3,618 | 2025 版（西药+中成药+谈判药品） |
| NMPA 药品目录 | 167,615 | 国产+进口药品 |
| OpenCMKG 三元组 | 354,752 | 疾病-症状-药物-食物关系 |
| CPubMed-KG 三元组 | 105,328 | PubMed 中文文献挖掘 |
| CMeKG 疾病百科 | 8,807 | 症状/病因/治疗/药物/食物 |
| **华佗26M问答** | **177,703** | **16科室，2,701种疾病** |
| **综合医患问答** | **426,978** | 含源芯医患对话（儿科等） |
| 默沙东诊疗手册 | 6,127 | 大众版+专业版全文 |
| MedlinePlus | 1,017 + 201 | 英文健康百科 + 中文多语言材料 |
| 统一语料全文 (corpus×10) | 14,976 | StatPearls 9,644 / LactMed 哺乳期用药 1,948 / MedlinePlus Genetics 2,830 / 精神障碍诊疗规范 96 节 / WHO 旅行医学 193 国 / 红十字急救 21 / 中国疾控科普 11 / OTC 用药安全 8 + 说明书 24 / 中文 MedlinePlus 201（与上行同计，不重复累加） |
| 活跃工具数 | **13** | 统一检索+知识图谱+专用工具 |

## 🚀 快速开始（零配置）

**不用装 Go、不用配环境变量，下载就能用：**

1. **下载程序**：打开 [GitHub Releases](https://github.com/deep2code/doctor-agent/releases) 页面，下载对应你系统的文件（macOS / Windows / Linux）
2. **解压**，任选一种方式开始：
   - **网页版（推荐）**：运行 `doctor-agent` 可执行文件（Windows 双击 `doctor-agent.exe`，mac/Linux 终端执行 `./doctor-agent`），然后浏览器打开 **http://localhost:7071** （产品落地页），点「进入咨询台」或直接访问 **http://localhost:7071/app** 开始聊天
   - **命令行版**：双击 `start-chat`（`.bat` / `.sh`）
3. **首次运行**会问你一个 API Key——推荐 **智谱 glm-4-flash（免费）**，去 [open.bigmodel.cn](https://open.bigmodel.cn) 注册免费获取，粘贴一次自动保存，之后永不再问
4. **直接输入问题**，回车即答

> 💡 想用别的模型？设置环境变量即可覆盖：`DEEPSEEK_API_KEY=sk-...`、豆包/智谱走 `LLM_PROVIDER=openai-compat`（详见 `.env.example` 与下方「🔧 配置」）。首次运行引导也已内置 **智谱 / DeepSeek / 豆包** 三选一。

## 📦 部署指南

### 快速体验（本地运行）

```bash
# 下载对应平台二进制（从 GitHub Releases）
chmod +x doctor-agent          # macOS / Linux
./doctor-agent                 # Windows: doctor-agent.exe
```

浏览器打开 `http://localhost:7071`（聊天界面在 `/app`），首次运行引导配置 API Key（推荐智谱 glm-4-flash 免费）。

> 📁 MariaDB 业务库（`doctor_agent`：用户/会话/消息/反馈）在首次启动时自动创建，无需手动建库。连接参数通过 `MARIA_DB_*` 或 `APP_DB_DSN` 配置。
> ⚠️ **知识库是独立的一个 MariaDB 库（`doctor_knowledge`）**，二进制内不含数据。裸机/开发环境部署后需执行一次 `./doctor-agent seed-knowledge` 从 `gz/*.zst` 灌入。**Docker 生产环境不需要这一步**：compose 的 `mariadb` 服务用的是预灌好知识的 `doctor-agent-kb` 镜像（首次挂空卷自动导入），向量检索读预烘焙的 `doctor-agent-qdrant` 镜像，查询端 embedding 由 `doctor-agent-embed` 提供。

---

### 完整生产部署（Ubuntu/Debian 服务器）

以下以 Ubuntu 22.04 为例，从零搭建完整生产环境。

#### 第一步：准备服务器

```bash
# 更新系统
sudo apt update && sudo apt upgrade -y

# 创建专用用户和目录
sudo useradd -r -s /bin/false doctor-agent
sudo mkdir -p /opt/doctor-agent/{data,sessions,logs}
sudo chown -R doctor-agent:doctor-agent /opt/doctor-agent
```

#### 第二步：部署程序

```bash
# 下载（替换为最新版本和实际架构）
cd /tmp
wget https://github.com/deep2code/doctor-agent/releases/latest/download/doctor-agent-linux-amd64
chmod +x doctor-agent-linux-amd64

# 部署到目标目录
sudo cp doctor-agent-linux-amd64 /opt/doctor-agent/doctor-agent
sudo chown doctor-agent:doctor-agent /opt/doctor-agent/doctor-agent
```

#### 第三步：配置环境变量

```bash
sudo tee /opt/doctor-agent/.env << 'EOF'
# LLM 配置（必填）
LLM_PROVIDER=deepseek
DEEPSEEK_API_KEY=sk-你的密钥

# 安全配置（生产环境必填）
API_KEY=自定义一个复杂的访问密钥

# 数据库
APP_DB_DSN=root:your_password@tcp(localhost:3306)/doctor_agent

# 会话持久化
SESSION_DIR=/opt/doctor-agent/sessions

# 可选：语义校验（开启后 LLM 成本翻倍）
POST_VERIFY_SEMANTIC=false
EOF

sudo chown doctor-agent:doctor-agent /opt/doctor-agent/.env
sudo chmod 600 /opt/doctor-agent/.env
```

#### 第四步：创建 systemd 服务

```bash
sudo tee /etc/systemd/system/doctor-agent.service << 'EOF'
[Unit]
Description=Doctor Agent - 循证医学AI助手
After=network.target
Wants=network-online.target

[Service]
Type=simple
User=doctor-agent
Group=doctor-agent
WorkingDirectory=/opt/doctor-agent
EnvironmentFile=/opt/doctor-agent/.env
ExecStart=/opt/doctor-agent/doctor-agent serve
Restart=always
RestartSec=5
StandardOutput=append:/opt/doctor-agent/logs/access.log
StandardError=append:/opt/doctor-agent/logs/error.log

# 安全加固
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/opt/doctor-agent/data /opt/doctor-agent/sessions /opt/doctor-agent/logs
PrivateTmp=true

[Install]
WantedBy=multi-user.target
EOF
```

```bash
# 启动服务
sudo systemctl daemon-reload
sudo systemctl enable doctor-agent
sudo systemctl start doctor-agent

# 验证状态
sudo systemctl status doctor-agent
sudo tail -f /opt/doctor-agent/logs/access.log
```

#### 第五步：配置 Nginx 反向代理 + HTTPS

```bash
# 安装 Nginx 和 Certbot
sudo apt install -y nginx certbot python3-certbot-nginx

# 创建 Nginx 配置
sudo tee /etc/nginx/sites-available/doctor-agent << 'EOF'
server {
    listen 80;
    server_name medical.example.com;
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl http2;
    server_name medical.example.com;

    # SSL 证书（Certbot 自动配置）
    ssl_certificate /etc/letsencrypt/live/medical.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/medical.example.com/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;

    # 上传大小限制（支持图片分析）
    client_max_body_size 12M;

    # 日志
    access_log /var/log/nginx/doctor-agent-access.log;
    error_log /var/log/nginx/doctor-agent-error.log;

    location / {
        proxy_pass http://127.0.0.1:7071;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # SSE 流式响应（必须）
        proxy_read_timeout 300s;
        proxy_send_timeout 300s;
        proxy_buffering off;
    }
}
EOF

# 启用站点
sudo ln -s /etc/nginx/sites-available/doctor-agent /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx

# 申请 SSL 证书
sudo certbot --nginx -d medical.example.com
```

#### 第六步：配置防火墙

```bash
# UFW 防火墙
sudo ufw allow OpenSSH
sudo ufw allow 'Nginx Full'
sudo ufw enable
sudo ufw status

# 或者 iptables（仅开放必要端口）
# sudo iptables -A INPUT -p tcp --dport 22 -j ACCEPT
# sudo iptables -A INPUT -p tcp --dport 80 -j ACCEPT
# sudo iptables -A INPUT -p tcp --dport 443 -j ACCEPT
# sudo iptables -A INPUT -i lo -j ACCEPT
# sudo iptables -A INPUT -m state --state ESTABLISHED,RELATED -j ACCEPT
# sudo iptables -P INPUT DROP
```

#### 第七步：设置日志轮转

```bash
sudo tee /etc/logrotate.d/doctor-agent << 'EOF'
/opt/doctor-agent/logs/*.log {
    daily
    rotate 30
    compress
    delaycompress
    missingok
    notifempty
    create 0640 doctor-agent doctor-agent
}
EOF
```

#### 第八步：备份策略

```bash
# 创建备份脚本
sudo tee /opt/doctor-agent/backup.sh << 'SCRIPT'
#!/bin/bash
BACKUP_DIR="/opt/doctor-agent/backups"
DATE=$(date +%Y%m%d_%H%M%S)
mkdir -p $BACKUP_DIR

# 备份数据库
cp /opt/doctor-agent/data/doctor-agent.db $BACKUP_DIR/db_$DATE.db

# 备份配置
cp /opt/doctor-agent/.env $BACKUP_DIR/env_$DATE

# 备份会话
tar czf $BACKUP_DIR/sessions_$DATE.tar.gz /opt/doctor-agent/sessions/

# 保留最近30天
find $BACKUP_DIR -mtime +30 -delete
SCRIPT

chmod +x /opt/doctor-agent/backup.sh

# 添加定时任务（每天凌晨3点备份）
(crontab -l 2>/dev/null; echo "0 3 * * * /opt/doctor-agent/backup.sh") | crontab -
```

---

### Docker Compose 部署

#### 1. 创建 .env 文件

```bash
cp .env.example .env
```

编辑 `.env`，填入你的 API Key：

```bash
LLM_PROVIDER=deepseek
DEEPSEEK_API_KEY=sk-你的密钥
API_KEY=自定义访问密钥
# 可选：镜像名覆盖（默认已指向公共仓库，四个服务各一个变量）
# QDRANT_IMAGE=docker.io/你的用户名/doctor-agent-qdrant:latest
# EMBED_IMAGE=docker.io/你的用户名/doctor-agent-embed:latest
# KB_IMAGE=docker.io/你的用户名/doctor-agent-kb:latest
```

> **架构说明（2026-09-20 校准；compose 共 4 个服务 / 4 个镜像）**：
> - `doctor-agent` —— Go 源码 + 前端，**改代码才重建**。
> - `doctor-agent-qdrant` —— 标准 Qdrant（pin v1.19.0）+ **构建期烘好的向量**（gz 知识源
>   仅作构建期输入，不在最终镜像内；bake WAL 已清理），启动即用、零灌入等待。瘦身手段：
>   ① 烘焙按 `vectorSkipDatasets`（`internal/knowledge/bake.go`）跳过已有专用 lookup 工具
>   或关键词全文层够用的数据集（medkg/nmpa/cpubmed/icd10/icd11/hpo/orphanet/icdo3/corpus/
>   public_resources），② collection 用 `datatype=float16`（向量磁盘减半），③ payload 去掉
>   无消费方的 text/timestamp 字段 —— 镜像从 ~10.8GB 降到 ~2-2.5GB。
> - `doctor-agent-embed` —— 查询端 embedding：bge-m3 **INT8**（ONNX，CPU），
>   OpenAI 兼容 `/v1/embeddings` 监听 :18080，模型文件已打进镜像（运行机无需准备模型）。
>   必须与烘焙向量**同模型**，否则语义召回静默失效。
> - `doctor-agent-kb` —— 预灌全部医学知识的 MariaDB 11.4 数据镜像：`docker/Dockerfile.kb`
>   把 `doctor_knowledge` 全量 dump 放进 `/docker-entrypoint-initdb.d/`，**空卷首启自动导入、
>   运行时零 seed**；镜像 tag 取 `internal/knowledge/data/version.json` 的版本号。
>
> **一次部署 = 4 个镜像都拉到**（`docker compose up -d`），app 依赖 mariadb + qdrant + embed。
> **只读部署**：compose 不给 qdrant 挂卷，向量直接读镜像层（容器重建即恢复），磁盘约 5GB
> 可用即可；如需写入 Qdrant 请自行挂卷。
> ⚠️ **MariaDB 不是"只做业务库"**：同一实例里既有业务库 `doctor_agent`（用户/会话/消息/反馈），
> 又有知识库 `doctor_knowledge`（关键词/精确查找层，由 `doctor-agent-kb` 预灌）。Qdrant 只负责
> 向量这一腿，二者是混合检索的两条腿，不是替代关系。
> 本地重新构建并推送（烘焙工具已独立，不依赖 app 镜像；打包前需先在本机烘出 `qdrant-storage/`
> —— 有产物 `./build.sh qdrant` 直接 COPY 打包，无产物才会调烘焙脚本。⚠️ `build.sh` 的 macOS
> 分支引用的 `bake-local.sh` 目前不在仓库内，此路径已损坏，可用 GPU 烘焙管线 `bake-gpu.sh` 代替）：
> ```bash
> python3 external/make_gz.py  # data/*.json 变更后先重新压缩（自动合并 .partNNN 分片）
> ./build.sh               # 改代码时：只构建+推送 app 镜像
> ./build.sh qdrant        # 改知识库时：只构建+推送 RAG 镜像
> ./build.sh embed         # 只在 embedding 服务/模型变更时
> ./build.sh kb            # 只在知识数据变更且要更新 MariaDB 关键词层时
> ./build.sh full          # 全量：app + qdrant + embed + kb
> ```

#### 2. 启动

```bash
docker compose up -d --build
```

#### 3. 访问

浏览器打开 `http://localhost:7071`（聊天界面 `/app`；compose 已把服务端口映射为 `7071:7071`）

#### 4. 常用命令

```bash
docker compose logs -f          # 查看日志
docker compose restart          # 重启
docker compose down             # 停止
docker compose up -d --build    # 重新构建并启动
```

---

### 环境变量完整说明

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `LLM_PROVIDER` | `deepseek` | LLM 提供商：`deepseek` / `anthropic` / `openai-compat` |
| `DEEPSEEK_API_KEY` / `DEEPSEEK_MODEL` / `DEEPSEEK_VISION_MODEL` | - / `deepseek-flash` / `deepseek-flash` | DeepSeek 密钥与模型名（视觉模型供影像分析） |
| `ANTHROPIC_API_KEY` / `ANTHROPIC_MODEL` | - / `claude-sonnet-4-20250514` | 使用 Claude 时 |
| `OPENAI_COMPAT_BASE_URL` / `_API_KEY` / `_MODEL` / `_VISION_MODEL` | 空 | 任意 OpenAI 协议端点（智谱/Qwen/豆包等） |
| `MAX_TOKENS` / `TEMPERATURE` | `8192` / `0.3` | 生成参数 |
| `MAX_HISTORY_TURNS` | `20` | 送入模型的历史轮数上限 |
| `MAX_TOOL_ITERATIONS` / `MAX_TOOL_CALLS` | `5` / `5` | Agent 循环次数与单轮并发工具数 |
| `KNOWLEDGE_RETRIEVAL_ENABLED` / `KNOWLEDGE_TOP_K` | `true` / `8` | 知识检索开关与返回条数 |
| `QUERY_UNDERSTANDING_ENABLED` / `QUERY_UNDERSTANDING_BRANCHES` / `UNDERSTAND_MODEL` | `true` / `5` / 空 | 查询改写/别名扩展分支 |
| `ALIAS_MAP_PATH` | `data/alias_map.json` | 可选外部同义词表，**按 key 合并**进 `//go:embed` 内置 `alias_map.json`（同名 key 外部优先，其余保留）；文件缺失/解析失败只提示，不影响启动 |
| `VECTOR_STORE_ENABLED` / `VECTOR_STORE_HOST` / `VECTOR_STORE_PORT` / `VECTOR_COLLECTION` | `true` / `localhost` / `6334` / `medical_knowledge` | Qdrant 向量检索（不可达时自动降级为关键词检索） |
| `EMBEDDING_ENABLED` / `EMBEDDING_BASE_URL` / `EMBEDDING_API_KEY` / `EMBEDDING_MODEL` | `true` / 空 / 空 / `bge-m3` | 无本地模型回退：`EMBEDDING_BASE_URL` 未配则打 warn 并降级为 keyword-only（`seed-knowledge`/`vector-bake` 会直接报错退出）；查询端模型必须与烘焙向量同源（bge-m3） |
| `EMERGENCY_DETECTION_ENABLED` / `SCOPE_GUARD_ENABLED` / `POST_VERIFY_ENABLED` | `true` | L1/L2/L3 安全层开关 |
| `POST_VERIFY_SEMANTIC` / `POST_VERIFY_JUDGE_MODEL` | `false` / 空 | 语义二次校验（LLM-as-judge，成本约翻倍） |
| `SERVER_HOST` / `SERVER_PORT` | `0.0.0.0` / **`7071`** | 监听地址与端口（网页版/反代 upstream 都用它） |
| `API_KEY` | 空 | Bearer 鉴权密钥，空=不鉴权；公开页与 `/health` 始终免鉴权 |
| `CORS_ORIGINS` | 空 | 逗号分隔的允许来源；空 = 允许全部 (`*`) |
| `RATE_LIMIT` | `0` | 每 IP 每分钟请求数限制，`0`=不限（公开页不计） |
| `PUBLIC_BASE_URL` | 空 | 生产域名，用于落地页 canonical/og:url 与 sitemap |
| `SESSION_DIR` | 空 | 会话 JSON 快照目录，空=仅内存 |
| `MARIA_DB_HOST` / `_PORT` / `_USER` / `_PASSWORD` | `localhost` / `3306` / `root` / 空 | 两个库共用的连接参数 |
| `MARIA_DB_KNOWLEDGE_DB` / `MARIA_DB_APP_DB` | `doctor_knowledge` / `doctor_agent` | 知识库 / 业务库库名 |
| `KNOWLEDGE_DB_DSN` / `APP_DB_DSN` | 空（由 `MARIA_DB_*` 组合） | 显式覆盖两个库的完整 DSN |
| `ADMIN_PASSWORD` | 空 | 首次启动自动创建 `admin` 账号的密码；**为空时回退 `admin123` 并打 Warn——生产必须显式设置** |
| `MEDIA_DIR` | `data/media` | `/media/*` 静态资源目录 |
| `LOG_LEVEL` | `info` | slog 级别：`debug` / `info` / `warn` / `error` |

---

### 常用运维命令

```bash
# 服务管理
sudo systemctl start doctor-agent      # 启动
sudo systemctl stop doctor-agent       # 停止
sudo systemctl restart doctor-agent    # 重启
sudo systemctl status doctor-agent     # 状态

# 查看日志
sudo journalctl -u doctor-agent -f     # 实时日志
sudo tail -f /opt/doctor-agent/logs/access.log

# 数据库操作（MariaDB）
mysql -h localhost -u root -p doctor_agent -e "SHOW TABLES;"              # 查看表
mysql -h localhost -u root -p doctor_agent -e "SELECT * FROM users;"     # 查看用户
mysql -h localhost -u root -p doctor_agent -e "SELECT * FROM sessions;"   # 查看会话

# 备份恢复
sudo systemctl stop doctor-agent
mysqldump -h localhost -u root -p doctor_agent > /opt/doctor-agent/backups/db_$(date +%Y%m%d_%H%M%S).sql
mysql -h localhost -u root -p doctor_agent < /opt/doctor-agent/backups/db_backup.sql
sudo systemctl start doctor-agent

# 更新版本
sudo systemctl stop doctor-agent
sudo cp /tmp/doctor-agent-linux-amd64 /opt/doctor-agent/doctor-agent
sudo systemctl start doctor-agent
```

### 开发者用（源码运行）

<details>
<summary>源码构建 / 校验 / 评测（点击展开）</summary>

```bash
go run . chat                 # 命令行聊天（首次同样会引导填 key）
go run . serve                # 启动后浏览器打开 http://localhost:7071 (落地页) / http://localhost:7071/app (咨询台)
go run . verify-knowledge     # 校验知识库
go run ./evals                # 离线防幻觉评测
```
</details>

### HTTP API

<details>
<summary>curl 示例（点击展开）</summary>

```bash
curl http://localhost:7071/health          # 健康检查

curl -X POST http://localhost:7071/chat \
  -H "Content-Type: application/json" \
  -d '{"message": "我吃了蚕豆后脸色发黄，是怎么回事？"}'

curl -N -X POST http://localhost:7071/chat/stream \   # SSE 流式
  -H "Content-Type: application/json" \
  -d '{"message": "我一喝牛奶就拉肚子，是乳糖不耐受吗？", "conversation_id": "可选，续同一会话"}'
```
</details>

> 🔒 请求体字段：`message`、`conversation_id`（可选，多轮）、`images`（可选，base64 图片走影像分析）、`member_id`（可选，家庭档案成员）。
> 🔒 若设置了 `API_KEY`，除落地页与探针（`/`、`/map`、`/stats`、`/health`、`/robots.txt`、`/sitemap.xml`、`/llms.txt`）外全部端点都要携带 `Authorization: Bearer <API_KEY>`——包括网页聊天界面 `/app`，因此**启用 API_KEY 后浏览器直连网页版会被 401 拦截**（面向 API 调用方或在反代层放行）。

**路由一览**（`internal/server/server.go`）：

| 分组 | 路径 |
|---|---|
| 公开页面 | `GET /`（落地页）、`/map`、`/stats`、`/robots.txt`、`/sitemap.xml`、`/llms.txt` |
| Web UI | `GET /app`（咨询台单页应用）+ 懒加载静态资源 `/mermaid.min.js` `/three.min.js` `/anatomy*.js` `/qrcode.min.js` `/favicon.ico` `/media/*` |
| 对话 | `POST /chat`、`POST /chat/stream`（真 SSE）、`POST /feedback`（评分） |
| 会话/家庭（需 MariaDB 业务库） | `/sessions`、`/sessions/{id}`、`/family`、`/family/{id}`、`/share`、`/share/{token}` |
| 管理 | `/admin`（Basic-auth 控制台页）、`/admin/users*`、`/admin/sessions*`、`/admin/knowledge*`（含 stats/versions/export）、`/admin/sync`(+`/status`)、`/admin/feedback*`、`/admin/audit-logs`、`/admin/config*`、`/admin/api-stats*`、`/admin/analytics`、`/admin/batch/{users,knowledge}`、`/admin/export` |
| 探针 | `GET /health`（免鉴权、免限流） |

### 想换模型 / 开高级功能？

默认 DeepSeek 开箱即用。换国内其他模型（智谱 / 豆包 / Qwen）、开鉴权限流、会话持久化等，见下方「🔧 配置」表和 `.env.example` 里的注释——**都是可选项，不配也能用**。

## 💻 系统要求

### 最低配置

| 资源 | 需求 | 说明 |
|------|------|------|
| 内存 | 2 GB | 知识库加载后约 500MB，运行时峰值约 1GB |
| 磁盘 | 500 MB | 二进制 + 会话/日志文件（知识库在 MariaDB，向量在 Qdrant，均不占应用磁盘） |
| CPU | 1 核 | 纯 CPU 运行，无 GPU 要求 |
| 网络 | 需要 | 调用 LLM API (Claude/DeepSeek/智谱) |

### 推荐配置

| 资源 | 需求 | 说明 |
|------|------|------|
| 内存 | 4 GB | 更流畅的对话体验，支持更大对话历史 |
| 磁盘 | 1 GB | 含日志和多个会话存储 |
| CPU | 2 核 | 并发处理多个请求 |

### 资源消耗说明

- **CPU/内存**：本地只运行 HTTP 服务器 + 知识检索，不运行 LLM 推理
- **网络**：主要开销是调用外部 LLM API 生成回答
- **存储**：会话文件 (JSON) + 日志，可选持久化到磁盘
- **GPU**：不需要，所有 AI 推理通过 API 完成

### 可选组件

| 组件 | 额外需求 | 说明 |
|------|----------|------|
| Qdrant 向量检索 | 1-2 GB 内存 | 提升语义检索效果 |
| Session 持久化 | 额外磁盘 | 重启后恢复对话历史 |
| 日志存储 | 额外磁盘 | 可选，便于审计 |

### LLM API 费用参考

| 服务商 | 模型 | 费用 |
|--------|------|------|
| 智谱 | glm-4-flash | **免费** |
| DeepSeek | deepseek-chat | 约 ¥0.001/千tokens |
| Anthropic | Claude 3.5 Sonnet | 约 $0.003/千tokens |

> 💡 推荐使用 **智谱 glm-4-flash（免费）** 作为默认模型，适合日常使用。

## 🔧 配置（环境变量，详见 .env.example）

| 变量 | 默认 | 说明 |
|---|---|---|
| `LLM_PROVIDER` | `deepseek` | `deepseek` / `anthropic` / `openai-compat` |
| `POST_VERIFY_SEMANTIC` | `false` | 语义二次校验（LLM-as-judge，开启后 LLM 成本约翻倍） |
| `API_KEY` | 空 | 除公开页/`/health` 外所有端点的 Bearer 鉴权；空 = 不鉴权 |
| `CORS_ORIGINS` | 空 | 逗号分隔的允许来源；空 = 允许全部 (`*`) |
| `RATE_LIMIT` | `0` | 每 IP 每分钟最大请求数；`0` = 不限 |
| `SESSION_DIR` | 空 | 会话 JSON 快照目录（重启后对话不丢）；空 = 仅内存 |
| `SERVER_PORT` | `7071` | HTTP 监听端口（网页版与反代 upstream 同端口） |

> 完整变量清单见上文「🚀 部署指南 → 环境变量完整说明」，或 `.env.example`。

## 🏗️ 架构

```
用户输入
  ├─ [L1 紧急检测] 关键词匹配 → 120响应（零LLM延迟）
  ├─ [L2 范围检查] 排除兽医/法医/偏方/自残
  ├─ [知识检索] 关键词(BM25+CJK) + 向量/混合检索，命中 MariaDB 知识库 / Qdrant RAG
  ├─ [提示词组装] 分层系统提示词（Layer 0–4 共 9 段）+ 检索知识注入
  ├─ [Agent循环] LLM Provider(流式) ← → 13个医疗工具
  ├─ [L3 引用验证] 引用真实性核查 + 诊断断言检查
  └─ [L4 免责声明] 返回 响应 + 引用列表 + 免责声明
```

HTTP 层另有可选安全中间件：Bearer 鉴权 → 每 IP 限流 → CORS 白名单。

## 🛠️ 13个活跃医疗工具

1. **drug_safety_check** — G6PD药物禁忌查询（安全/不安全/谨慎/未知）
2. **genetic_risk_calculator** — 地贫遗传概率计算（Punnett方阵）
3. **food_risk_analyzer** — 中国常见食物风险分析（蚕豆/咸鱼/老火汤/海鲜/牛奶）
4. **symptom_triage** — 症状紧急分诊（EMERGENCY/URGENT/ROUTINE/SELF_CARE）
5. **drug_interaction_check** — 药物相互作用检查
6. **drug_label_lookup** — FDA药品标签中文摘要查询（344条）
7. **knowledge_search** — 统一医学知识检索（跨多个数据集）
8. **exact_lookup** — 精确查找（12 类：ICD-10/ICD-11/HPO表型/Orphanet罕见病/ICD-O-3形态学/NMPA/医保目录2025/FDA标签/TTD/SIDER/ClinVar变异/EML）
9. **medical_kg_lookup** — 医学知识图谱查询（354,752条三元组）
10. **cpubmed_kg_lookup** — PubMed文献知识图谱查询（105,328条三元组）
11. **lab_report_analyze** — 实验室检查报告分析
12. **visit_prep** — 就诊准备工具
13. **medical_image_analyze** — 医学影像分析（解剖图）

> 📝 注：其他工具（reference_lookup, literature_search, msd_search, medline_search, drug_lookup, eml_lookup, nhc_search, fhs_search, aap_search, lab_interpreter, icd10_lookup, nmpa_drug_lookup, disease_encyclopedia_lookup, huatuo_qa_lookup, body_part_lookup, growth_assessment, milestone_lookup, newborn_care_lookup, symptom_checker 等）已整合到 knowledge_search / exact_lookup 统一检索架构中（variant 查询并入 exact_lookup type=variant）。

## 📁 项目结构

```
doctor-agent/
├── main.go                         # CLI + HTTP 入口（chat / serve / verify-knowledge / seed-knowledge / sync-knowledge / version）
├── cmd/
│   ├── vector-bake/                # 离线 gz → Qdrant 向量烘焙（独立无业务依赖）
│   └── kbseed/                     # 知识库种子工具
├── internal/
│   ├── agent/agent.go                # Agent 核心循环（含流式 ProcessMessageStream）、13 个工具注册处
│   ├── config/                       # 环境变量配置（Load/Validate、两个库的 DSN 组合）
│   ├── database/                     # MariaDB 业务库（users/sessions/messages/feedback/family）
│   ├── auth/                         # 用户鉴权（管理员建号、登录、token、SHA256+salt）
│   ├── embedding/                    # OpenAI 协议 embedding 客户端（bge-m3）
│   ├── logging/                      # slog 初始化
│   ├── session/                      # 会话状态 + PatientContext + SESSION_DIR JSON 快照
│   ├── prompt/                       # 分层系统提示词（Layer 0–4 共 9 段）
│   ├── knowledge/                    # 知识库(MariaDB 懒加载 + Qdrant 向量) + 各检索器 + 引用系统 + verify.go
│   │   ├── data/                     # 78个源JSON（编辑后运行 python3 external/make_gz.py → gz/*.zst）
│   │   │                             #   超过 GitHub 单文件 100MiB 的两份（huatuo_qa / medical_qa_pairs）以
│   │   │                             #   X.json.partNNN + X.json.parts 分片入库，make_gz 会自动合并
│   │   └── gz/                       # zstd 压缩的种子文件（seed/bake 输入，二进制不内嵌）
│   ├── tools/                        # 13个活跃工具 + Registry + router(知识图谱意图路由)
│   ├── safety/                       # 4层安全防护（紧急检测/范围守卫/引用后验证/免责声明）
│   └── server/                       # HTTP Server + 中间件 + web/(落地页·咨询台·地图·统计·分享·管理台，go:embed)
├── evals/                           # 防幻觉黄金评测集（中文77 + 英文299 + CMB/CMExam各200选择题）
├── external/                        # 知识抓取/转换管线（medkb 统一语料 + WHO/MSD/EML/ClinVar…）
├── Dockerfile / Dockerfile.qdrant(.slim) / Dockerfile.embed / docker/Dockerfile.kb
│                                  # 四镜像：app / 预烘焙向量 Qdrant / bge-m3 INT8 / 预灌知识 MariaDB
├── docker-compose.yml               # mariadb + qdrant + embed + app（:7071）
├── build.sh / remote-deploy.sh      # 唯一打包入口 / 构建机部署脚本
├── .github/workflows/ci.yml         # CI：Lint / Test / Build(+verify-knowledge) / Docker Build / Verify gz
├── .env.example
└── README.md
```

> 🧪 运行 `go test ./...` 需要本地 MariaDB（`MARIA_DB_*` 或 `KNOWLEDGE_DB_DSN`/`APP_DB_DSN`）；离线可跑的子集是纯逻辑用例（如 `TestWireNewDatasetsSmoke`、`TestVectorSkipDatasetsAreDisjoint`）。

## ⚠️ 免责声明

本系统是**AI辅助工具**，仅供医学教育和信息参考，**不能替代执业医师的专业诊断、治疗建议或处方**。紧急情况请立即拨打120。
