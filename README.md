# Doctor Agent — 循证医学AI助手

权威、专业、最高水平的医生智能体，专为**全中国人群**定制。

## 🎯 核心特性

- **纯循证医学**：所有回答基于已发表文献和临床指南，拒绝中医、偏方及未经科学验证的说法
- **Zero Hallucination**：三重反幻觉机制——提示词约束 + 知识库绑定 + 响应后引用验证
- **每条回答有据可查**：事实性陈述必须标注引用编号 `[N]`，附DOI/PMID和证据等级
- **全人群覆盖**：覆盖地贫、G6PD缺乏症、鼻咽癌、乙肝、乳糖不耐受、ALDH2酒精代谢缺陷等中国重点高发疾病
- **4层安全防护**：紧急检测(零延迟120响应) → 范围检查 → 引用验证 → 免责声明
- **超大知识库**：51个数据源，358+医学条目，177K+医患问答，35个专业工具

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
| 医学知识条目 | 358 | 核心疾病+常见病+WHO+卫健委指南 |
| 药物条目 | 13 | G6PD禁忌药物+中药 |
| ICD-10 疾病编码 | 35,862 | 国家临床版2.0 |
| NMPA 药品目录 | 167,615 | 国产+进口药品 |
| OpenCMKG 三元组 | 354,752 | 疾病-症状-药物-食物关系 |
| CPubMed-KG 三元组 | 105,328 | 86种疾病，PubMed文献挖掘 |
| CMeKG 疾病百科 | 8,807 | 症状/病因/治疗/药物/食物 |
| **华佗26M问答** | **177,703** | **16科室，2,701种疾病** |
| 默沙东诊疗手册 | 6,086 | 大众版+专业版 |
| MedlinePlus | 1,017 | 英文健康百科 |
| WHO 知识库 | 232 | 官方中文fact sheets |
| 工具总数 | **35** | 覆盖诊断/治疗/用药/遗传/人体部位分诊 |

## 🚀 快速开始（零配置）

**不用装 Go、不用配环境变量，下载就能用：**

1. **下载程序**：打开 [GitHub Releases](https://github.com/deep2code/doctor-agent/releases) 页面，下载对应你系统的文件（macOS / Windows / Linux）
2. **解压**，任选一种方式开始：
   - **网页版（推荐）**：运行 `doctor-agent` 可执行文件（Windows 双击 `doctor-agent.exe`，mac/Linux 终端执行 `./doctor-agent`），然后浏览器打开 **http://localhost:8080** —— 就是聊天界面，类似国内大模型网页版
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

浏览器打开 `http://localhost:8080`，首次运行引导配置 API Key（推荐智谱 glm-4-flash 免费）。

> 📁 MariaDB 数据库（`doctor_agent`）在首次启动时自动创建，无需手动建库。连接参数通过 `APP_DB_DSN` 环境变量配置。

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
        proxy_pass http://127.0.0.1:8080;
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
# 可选：RAG 数据镜像（默认使用公共仓库镜像）
# QDRANT_IMAGE=docker.io/你的用户名/doctor-agent-qdrant:latest
```

> **架构说明（2026-08-30 起，双镜像）**：`doctor-agent-qdrant` 是一个镜像 = **纯 gz
> 知识库（alpine 层，51 数据集）+ 标准 Qdrant + 构建期烘好的向量**，启动即用、零灌入
> 等待。MariaDB 只做业务库（用户/会话/消息/反馈），知识检索走 Qdrant。
> 本地重新烘焙知识镜像（先构建 app，再从 app 取 vector-bake 二进制）：
> ```bash
> make docker-build          # 先构建 app 镜像
> ./docker/qdrant-context/build.sh doctor-agent-qdrant:latest doctor-agent:latest
> ```

#### 2. 启动

```bash
docker compose up -d --build
```

#### 3. 访问

浏览器打开 `http://localhost:8080`

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
| `DEEPSEEK_API_KEY` | - | DeepSeek API 密钥 |
| `ANTHROPIC_API_KEY` | - | Anthropic API 密钥（使用 Claude 时） |
| `OPENAI_COMPAT_API_KEY` | - | OpenAI 兼容端点密钥（智谱/Qwen 等） |
| `OPENAI_COMPAT_BASE_URL` | - | OpenAI 兼容端点 URL |
| `OPENAI_COMPAT_MODEL` | - | OpenAI 兼容模型名 |
| `API_KEY` | 空 | 访问鉴权密钥，空=不鉴权 |
| `APP_DB_DSN` | `root:@tcp(localhost:3306)/doctor_agent` | MariaDB 应用库 DSN（也可设 `KNOWLEDGE_DB_DSN` 知识库） |
| `SESSION_DIR` | 空 | 会话持久化目录，空=仅内存 |
| `CORS_ORIGINS` | `*` | 允许的跨域来源 |
| `RATE_LIMIT` | `0` | 每 IP 每分钟请求数限制，0=不限 |
| `POST_VERIFY_SEMANTIC` | `false` | 语义二次校验（成本翻倍） |

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
go run . serve                # 启动后浏览器打开 http://localhost:8080 即网页版
go run . verify-knowledge     # 校验知识库
go run ./evals                # 离线防幻觉评测
```
</details>

### HTTP API

<details>
<summary>curl 示例（点击展开）</summary>

```bash
curl http://localhost:8080/health          # 健康检查

curl -X POST http://localhost:8080/chat \
  -H "Content-Type: application/json" \
  -d '{"message": "我吃了蚕豆后脸色发黄，是怎么回事？"}'

curl -N -X POST http://localhost:8080/chat/stream \   # SSE 流式
  -H "Content-Type: application/json" \
  -d '{"message": "我一喝牛奶就拉肚子，是乳糖不耐受吗？"}'
```
</details>

> 🔒 若设置了 `API_KEY`，所有 `/chat*` 请求需携带 `Authorization: Bearer <API_KEY>` 头（`/health` 免鉴权）。

### 想换模型 / 开高级功能？

默认 DeepSeek 开箱即用。换国内其他模型（智谱 / 豆包 / Qwen）、开鉴权限流、会话持久化等，见下方「🔧 配置」表和 `.env.example` 里的注释——**都是可选项，不配也能用**。

## 💻 系统要求

### 最低配置

| 资源 | 需求 | 说明 |
|------|------|------|
| 内存 | 2 GB | 知识库加载后约 500MB，运行时峰值约 1GB |
| 磁盘 | 500 MB | 二进制 + 嵌入知识库 + 会话文件 |
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
| `LLM_PROVIDER` | `deepseek` | `deepseek` / `openai-compat` |
| `POST_VERIFY_SEMANTIC` | `false` | 语义二次校验（LLM-as-judge，开启后 LLM 成本约翻倍） |
| `API_KEY` | 空 | `/chat` 端点 Bearer 鉴权；空 = 不鉴权 |
| `CORS_ORIGINS` | 空 | 逗号分隔的允许来源；空 = 允许全部 (`*`) |
| `RATE_LIMIT` | `0` | 每 IP 每分钟最大请求数；`0` = 不限 |
| `SESSION_DIR` | 空 | 会话 JSON 快照目录（重启后对话不丢）；空 = 仅内存 |

## 🏗️ 架构

```
用户输入
  ├─ [L1 紧急检测] 关键词匹配 → 120响应（零LLM延迟）
  ├─ [L2 范围检查] 排除兽医/法医/偏方/自残
  ├─ [知识检索] 关键词(BM25+CJK) + 向量/混合检索，命中 MariaDB 知识库 / Qdrant RAG
  ├─ [提示词组装] 5层系统提示词 + 检索知识注入
  ├─ [Agent循环] LLM Provider(流式) ← → 35个医疗工具
  ├─ [L3 引用验证] 引用真实性核查 + 诊断断言检查
  └─ [L4 免责声明] 返回 响应 + 引用列表 + 免责声明
```

HTTP 层另有可选安全中间件：Bearer 鉴权 → 每 IP 限流 → CORS 白名单。

## 🛠️ 35个医疗工具

1. **drug_safety_check** — G6PD药物禁忌查询（安全/不安全/谨慎/未知）
2. **genetic_risk_calculator** — 地贫遗传概率计算（Punnett方阵）
3. **food_risk_analyzer** — 中国常见食物风险分析（蚕豆/咸鱼/老火汤/海鲜/牛奶）
4. **symptom_triage** — 症状紧急分诊（EMERGENCY/URGENT/ROUTINE/SELF_CARE）
5. **reference_lookup** — 循证医学文献引用检索（内置知识库）
6. **literature_search** — Europe PMC 文献检索（4425篇摘要，真实DOI/PMID）
7. **msd_search** — 默沙东诊疗手册中文版全文检索（6086页）
8. **variant_lookup** — ClinVar 基因变异查询（HBB/HBA1/HBA2/G6PD，1399条）
9. **medline_search** — MedlinePlus 健康百科检索（1017页，英文）
10. **drug_lookup** — 国家医保药品目录查询（2024版）
11. **eml_lookup** — WHO基本药物清单查询（第24版，564种）
12. **drug_label_lookup** — FDA药品标签中文摘要查询（344条）
13. **nhc_search** — 国家卫健委诊疗方案/指南全文检索（39篇中文）
14. **fhs_search** — 香港卫生署家庭健康服务育儿知识检索（103页中文）
15. **aap_search** — 美国儿科学会 healthychildren 育儿百科检索（英文）
16. **lab_interpreter** — 实验室检查解读（含地贫筛查注意事项）
17. **icd10_lookup** — ICD-10疾病编码查询（35,862种疾病）
18. **nmpa_drug_lookup** — NMPA药品目录查询（167,615种药品）
19. **medical_kg_lookup** — 医学知识图谱查询（354,752条三元组）
20. **disease_encyclopedia_lookup** — 疾病百科查询（8,807种疾病）
21. **cpubmed_kg_lookup** — PubMed文献知识图谱查询（105,328条三元组）
22. **huatuo_qa_lookup** — 华佗26M医疗问答查询（177,703条，16科室）

## 📁 项目结构

```
doctor-agent/
├── main.go                         # CLI + HTTP 入口
├── internal/
│   ├── agent/agent.go                # Agent 核心循环（含流式 ProcessMessageStream）
│   ├── config/                       # 配置加载
│   ├── session/                      # 会话管理 + JSON 文件持久化
│   ├── prompt/                       # 5层系统提示词
│   ├── knowledge/                    # 知识库(MariaDB/Qdrant) + 检索器 + 引用系统
│   │   ├── data/                     # 51个源JSON（编辑后运行 make gz）
│   │   └── gz/                       # gzip 压缩的种子文件（seed/bake 输入）
│   ├── tools/                        # 35个医疗工具 + 注册表
│   ├── safety/                       # 4层安全防护
│   └── server/                       # HTTP API Server（鉴权/限流/CORS）
├── evals/                           # 防幻觉黄金评测集（中文36题 + 英文299题）
├── external/                        # 知识抓取/转换管线（WHO/MSD/EML/ClinVar…）
├── .github/workflows/ci.yml         # CI：build/vet/test/verify-knowledge/evals/lint
├── Makefile
├── .env.example
└── README.md
```

## ⚠️ 免责声明

本系统是**AI辅助工具**，仅供医学教育和信息参考，**不能替代执业医师的专业诊断、治疗建议或处方**。紧急情况请立即拨打120。
