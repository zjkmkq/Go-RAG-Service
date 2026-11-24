# Go-RAG-Service 开发手册

---

## 项目概述

Go-RAG-Service 是一个基于 Go 语言构建的**检索增强生成（RAG）**服务。上传文档后自动分段、嵌入向量、存入 pgvector，查询时检索相关片段交给大模型生成精准回答。

## 当前状态（2026-11-23）

| 项目 | 状态 |
|------|------|
| 编译 | ✅ `go build ./...` 通过 |
| 服务启动 | ✅ 监听 `:8080` |
| 文档上传 | ✅ `POST /api/v1/documents`（小文件 0.6s，13 块手册 14.6s） |
| 向量嵌入 | ✅ qwen3-embed:0.6b（本地 Ollama，1024 维，约 450ms/条） |
| 向量检索 | ✅ pgvector 余弦相似度 |
| RAG 问答 | ✅ `POST /api/v1/rag`（0.9–1.8s） |
| LLM 模型 | deepseek-chat（`api.deepseek.com`） |
| 交互终端 | ✅ `ragcli.exe` + `启动服务.bat` |
| 重启保留数据 | ✅ 仅向量维度变化时才重建表 |

---

## 项目结构

```
go-rag-service/
├── cmd/
│   ├── server/main.go           # 服务端入口
│   └── ragcli/main.go           # 交互式测试终端（打字提问 / 拖文件上传）
├── configs/
│   ├── config.yaml              # 本地配置（复制模板填入，已被 .gitignore 忽略）
│   └── config.example.yaml      # 配置模板，新环境复制后填值
├── docker-compose.yml           # PostgreSQL + pgvector
├── ragcli.exe                   # 已编译的终端客户端
├── start-server.ps1             # 启动：拉起 postgres 容器 → Ollama → 服务
├── manage.ps1                   # 状态 / 停止（-Action status|stop|stopall）
├── 启动服务.bat                  # 双击：启动依赖 + 进问答
├── 问答.bat                      # 双击：只进问答（依赖已在跑）
├── 服务状态.bat                  # 双击：查看三层组件与知识库状态
├── 关闭服务.bat                  # 双击：只停 RAG 服务，保留依赖
├── 停止全部.bat                  # 双击：服务 + Ollama + 数据库容器一起停
├── internal/
│   ├── ai/
│   │   ├── provider.go          # Provider 接口
│   │   ├── ollama.go            # 本地 Ollama 客户端
│   │   ├── openai.go            # 远端 API 客户端
│   │   └── fallback.go          # 策略路由
│   ├── config/config.go         # 配置加载
│   ├── handler/
│   │   ├── document.go          # 文档上传/列表/删除接口
│   │   └── rag.go               # 问答接口
│   ├── logger/logger.go         # 日志
│   ├── repository/
│   │   ├── db.go                # 数据库连接
│   │   ├── migration.go         # 建表（维度不变则保留数据）
│   │   ├── chunk_repo.go        # 向量存储/检索
│   │   └── doc_repo.go          # 文档元数据
│   └── service/
│       ├── document.go          # 文档摄入流水线 + 切分
│       ├── chunk_test.go        # 切分单元测试（8 例）
│       └── rag.go               # RAG 问答流水线
└── go.mod / go.sum
```

---

## 环境依赖

| 组件 | 用途 | 获取方式 |
|------|------|----------|
| Go 1.25+ | 编译运行 | https://go.dev |
| PostgreSQL 16 + pgvector | 向量存储 | `docker compose up -d` |
| Ollama + qwen3-embed:0.6b | 向量嵌入（1024 维） | 官方安装包；模型用 `ollama create` 从 GGUF 导入 |
| DeepSeek API Key | 生文（DeepSeek 无 embeddings 接口） | https://platform.deepseek.com |

---

## 启动与关闭

日常全部靠双击，五个 bat 各管一件事：

| 操作 | 双击文件 | 等价命令 |
|------|----------|----------|
| 启动 + 进问答 | `启动服务.bat` | `powershell -File start-server.ps1` |
| 只进问答（依赖已在跑） | `问答.bat` | `ragcli.exe` |
| 看组件状态 | `服务状态.bat` | `powershell -File manage.ps1 -Action status` |
| 停 RAG 服务 | `关闭服务.bat` | `powershell -File manage.ps1 -Action stop` |
| 全停 | `停止全部.bat` | `powershell -File manage.ps1 -Action stopall` |

### 关闭分两层，别一上来就全停

| 层级 | 停什么 | 释放 | 恢复耗时 |
|------|--------|------|----------|
| `关闭服务.bat` | 只停 RAG 服务（杀 8080） | 端口 | 下一次启动约 1 秒 |
| `停止全部.bat` | 服务 + Ollama + postgres 容器 | 约 120MB | 需要等容器和 Ollama 起来 |
| 手动退 Docker Desktop | 托盘图标退出 | 约 600MB | 1-2 分钟 |

**实测内存占用**：RAG 服务 24MB、Ollama 空闲 41MB、postgres 容器 56MB、Docker Desktop 本体约 600MB。所以「省内存」的关键是退出 Docker Desktop，停服务主要意义是释放端口、避免旧进程干扰。

**停止不丢数据**：容器 stop/start 只停进程，数据在 volume 里。实测停全部再拉起，3 个文档 / 30 个片段完好。

### 启动时依次检查三层

`启动服务.bat` 会：

1. 检查 `rag-postgres` 容器，没起就 `docker start`
2. 检查 Ollama（`127.0.0.1:11434`），没起就从 `OLLAMA_BIN` / PATH 找 `ollama.exe` 拉起 `serve`
3. 检查 RAG 服务（`127.0.0.1:8080`），没起就新开窗口 `go run ./cmd/server`，然后轮询 health 直到就绪

> Ollama 不在 PATH 里时，设置环境变量 `OLLAMA_BIN` 指向 `ollama.exe` 的绝对路径即可（仅在本机用，不写入仓库）。

> 脚本里一律用 `127.0.0.1` 而不是 `localhost`。Ollama 只监听 IPv4，`localhost` 会先解析到 `::1` 并耗掉整个超时才回退，导致把运行中的服务误判为「已停止」（实测 2063ms 超时 vs 52ms 成功）。

想手动来：

```bash
# 0. 首次运行：从模板生成配置并填入 API key
cp configs/config.example.yaml configs/config.yaml
#    也可以设置环境变量 DEEPSEEK_API_KEY，配置里写 ${DEEPSEEK_API_KEY}

docker compose up -d          # 1. PostgreSQL + pgvector
ollama serve                  # 2. 嵌入服务（默认 11434）
go run ./cmd/server           # 3. 服务
./ragcli.exe                  # 4. 交互终端
```

---

## 交互终端（ragcli）

原来测试一次要拼两条长命令，`-F "file=@路径"` 里的 `@`、`-d '{"question":...}'` 这层 JSON 包装都得手写。`ragcli` 把这些都省掉了：

| 输入 | 效果 |
|------|------|
| 直接打字 | 当作问题提问 |
| 把文件拖进窗口 | 上传并入库（拖放产生的引号会自动去掉） |
| `docs` | 列出已入库文件和片段数 |
| `rm 2` | 删除列表里第 2 个文件（含其全部片段） |
| `topk 5` | 改每次召回的片段数，默认 3 |
| `help` / `exit` | 看帮助 / 退出 |

参数：`ragcli.exe --server http://localhost:8080 --topk 5`

输出里每条来源都带**文件名**和**余弦距离**（越小越相关），方便判断检索质量，而不是只看回答通不通顺。

> Windows 控制台默认 GBK，客户端启动时会自动把代码页切到 UTF-8（65001），中文输入不会乱码。

---

## 配置说明 (config.yaml)

```yaml
server:
  port: 8080                    # 服务端口

database:
  host: 127.0.0.1              # PostgreSQL 地址
  port: 5432
  user: postgres
  password: postgres
  dbname: rag_service

ollama:                         # 本地模型
  host: http://127.0.0.1:11434
  llm_model: qwen3.5-0.8b:latest
  embed_model: qwen3embed        # Qwen3-Embedding-0.6B (GGUF 导入，1024 维)
  timeout: 300s

api:                            # 远端 API（OpenAI 兼容）
  base_url: https://api.deepseek.com/v1
  api_key: "${DEEPSEEK_API_KEY}" # 从环境变量读取，勿写真实密钥
  llm_model: deepseek-chat       # 实际路由到 deepseek-flash；可选 deepseek-v4-pro
  embed_model: ""                # DeepSeek 无 embeddings 接口，留空
  timeout: 60s

strategy:                       # 策略路由
  embed: ollama                # 嵌入走本地（DeepSeek 不提供 embeddings）
  generate: api                # 生文走远端
```

**为什么嵌入必须在本地**：DeepSeek 的 `/v1/embeddings` 返回 404，只提供 `chat/completions`。项目本身把 `strategy.embed` 和 `strategy.generate` 拆成两条独立路由，所以「本地嵌入 + 远端生文」不用改代码，只改配置。

---

## API 接口

### 1. 健康检查

```
GET /health
→ 200 "ok"
```

### 2. 上传文档

```
POST /api/v1/documents
Content-Type: multipart/form-data

file: 文档文件（txt/md等）

→ 200 {"document_id":"uuid","filename":"intro.txt"}
```

### 3. 文档列表

```
GET /api/v1/documents

→ 200 {
    "documents": [
      {"id":"uuid","filename":"开发手册.md","file_type":"md",
       "file_size":4979,"chunk_count":13,"created_at":"2026-11-23T10:49:37Z"}
    ],
    "total": 3
  }
```

### 4. 删除文档

```
DELETE /api/v1/documents?id=<uuid>

→ 200 {"deleted":"uuid"}
→ 404 document not found
```

chunks 通过外键 `ON DELETE CASCADE` 一并删除。

### 5. RAG 问答

```
POST /api/v1/rag
Content-Type: application/json

{"question": "问题", "top_k": 3}

→ 200 {
    "answer": "基于文档的回答",
    "sources": [
      {"document_id":"uuid","filename":"开发手册.md",
       "content":"相关片段...","distance":0.547}
    ]
  }
```

`distance` 是余弦距离，越小越相关。检索无阈值，所以来源里出现不相关的文档时，看这个值就能判断是「真没检索到」还是「硬凑的」。

---

## 数据流

### 文档摄入

```
用户上传文件
  → 读取内容
  → Markdown 按标题层级切分（## 起新块，### 归入父块），单块上限 500 字
  → 调用本地 Ollama（qwen3-embed:0.6b）嵌入为 1024 维向量
  → 存入 PostgreSQL chunks 表 (pgvector)
```

### RAG 问答

```
用户提问
  → 本地 Ollama 嵌入问题为向量
  → pgvector 余弦相似度检索 Top-K 片段
  → 拼接上下文 + 问题 构建 Prompt
  → 调用远端 LLM（DeepSeek）生成回答
  → 返回答案 + 来源片段（含文件名与余弦距离）
```

---

## 数据库表结构

```sql
-- 文档元数据
documents (
    id UUID PRIMARY KEY,
    filename VARCHAR(255),
    file_type VARCHAR(50),
    file_size BIGINT,
    created_at TIMESTAMPTZ
)

-- 文本块 + 向量
chunks (
    id UUID PRIMARY KEY,
    document_id UUID REFERENCES documents ON DELETE CASCADE,
    content TEXT,
    embedding vector(1024),
    chunk_index INTEGER,
    created_at TIMESTAMPTZ
)
```

---

## 已知限制

- 仅支持纯文本文件（txt/md），不支持 PDF/Word
- 分段按 Markdown 标题层级切（代码围栏内的 # 不算标题），非 Markdown 文件退化为按空行分段
- 检索无相似度阈值：库里只要有数据就一定返回 top_k 条，问无关问题也会硬答
- 未实现会话历史/多轮对话
- 向量维度写在 `migration.go` 的 `embeddingDim` 常量里，换嵌入模型要同步改（维度不一致时启动会重建 chunks 表）
- 同名文件重复上传不会覆盖，会各自入库；需要手动 `rm`
- 0.6B 嵌入模型对短查询区分度偏弱：实测相关文本距离约 0.52，无关文本约 0.59，差距不大，会影响排序精度

---
