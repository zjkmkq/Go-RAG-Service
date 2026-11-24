# Go-RAG-Service

基于 Go 的 RAG（检索增强生成）文档问答服务。上传 txt/md 文档后按 Markdown 标题层级切分、向量化入库；提问时检索相关片段交给大模型作答，并返回答案与来源。

## 技术栈

| 层 | 选型 |
|---|---|
| 语言 | Go 1.25（`net/http` 标准库，无 Web 框架） |
| 向量库 | PostgreSQL 16 + pgvector（余弦距离） |
| 嵌入 | 本地 Ollama，Qwen3-Embedding-0.6B，1024 维，CPU 约 450ms/条 |
| 生文 | DeepSeek（OpenAI 兼容接口） |
| 客户端 | 自带 `ragcli` 交互终端 |

## 快速开始

```bash
# 1. 配置（config.yaml 不入库，从模板复制）
cd go-rag-service
cp configs/config.example.yaml configs/config.yaml
# 填入 api_key，或设置环境变量 DEEPSEEK_API_KEY

# 2. 依赖
docker compose up -d          # PostgreSQL + pgvector
ollama serve                  # 嵌入服务（默认 11434）

# 3. 编译运行
go run ./cmd/server           # 服务，:8080
go build -o ragcli.exe ./cmd/ragcli && ./ragcli.exe   # 交互终端
```

Windows 下也可以直接双击 `go-rag-service/启动服务.bat`，它会依次检查并拉起三层依赖，然后进入问答。

终端里**打字即提问、把文件拖进窗口即上传**，无需拼 curl 参数。命令：`docs` / `rm N` / `topk N` / `help` / `exit`。

## 接口

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/health` | 健康检查 |
| POST | `/api/v1/documents` | 上传文档（multipart，字段 `file`） |
| GET | `/api/v1/documents` | 文档列表（含片段数） |
| DELETE | `/api/v1/documents?id=` | 删除文档及其片段 |
| POST | `/api/v1/rag` | 问答，返回 `answer` + `sources`（含文件名与余弦距离） |

## 目录

| 路径 | 说明 |
|---|---|
| `go-rag-service/cmd/server/` | 服务入口与路由 |
| `go-rag-service/cmd/ragcli/` | 交互式测试终端 |
| `go-rag-service/internal/` | ai / handler / service / repository / config / logger |
| `docs/manual.md` | 开发手册：配置、接口、数据流、表结构 |
| `docs/devlog.md` | 开发日志：按时间记录决策、踩坑与实测数据 |

## 设计要点

**嵌入放本地、生文放远端。** DeepSeek 只提供 `chat/completions`，没有 `embeddings` 接口（`/v1/embeddings` 返回 404）。项目把 `strategy.embed` 和 `strategy.generate` 拆成两条独立路由，因此「本地嵌入 + 远端生文」不需要改代码，只改配置。

**切分按标题层级，不按空行。** 按空行切会把 Markdown 标题和正文拆散，导致检索命中的全是零信息标题块。现在的规则：代码围栏内的 `#` 不算标题，取「出现 ≥2 次的最浅标题层级」作为切分点，子节归入父块，只有标题没正文的块并入下一块。见 `internal/service/document.go`，对应 8 个单元测试。

**重启不清库。** 早期 `migration.go` 每次启动都 `DROP TABLE chunks`，服务一重启知识库就空。现在读 `pg_attribute` 判断向量维度，只有换嵌入模型导致维度变化时才重建。

## 已知限制

- 仅支持 txt/md，不支持 PDF/Word
- 检索无相似度阈值，库里只要有数据就一定返回 top_k 条，问无关问题也会硬答
- 无会话历史，不支持多轮对话
- 0.6B 嵌入对短查询区分度偏弱：相关文本距离约 0.52，无关约 0.59，排序精度受影响
- 同名文件重复上传不覆盖，需手动删除
