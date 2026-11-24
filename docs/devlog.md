# Go-RAG-Service 开发日志

---

## 2026-11-23 — 启动 / 关闭服务脚本

**概述**：补上"关闭"这一环。原来只能靠任务管理器杀进程，现在双击 bat 即可；启动脚本也一并修掉了两个会真实导致误判/崩溃的问题。

### 新增

| 文件 | 作用 |
|------|------|
| `manage.ps1` | `-Action status\|stop\|stopall`，状态面板 + 分层停止 |
| `服务状态.bat` | 查看三层组件与知识库状态 |
| `关闭服务.bat` | 只停 RAG 服务（释放 8080，保留依赖，重启约 1 秒） |
| `停止全部.bat` | 服务 + Ollama + postgres 容器一起停 |

### 修掉的两个真问题

**1. `localhost` 导致 Ollama 被误判为「已停止」**

Ollama 只监听 IPv4，`localhost` 在 Windows 上会先解析到 `::1`，连接失败要等整个超时才回退到 `127.0.0.1`。实测同一接口：

| 地址 | 结果 | 耗时 |
|------|------|------|
| `localhost:11434` | 超时失败 | 2063ms（TimeoutSec 2 直接放弃） |
| `127.0.0.1:11434` | 成功 | 52ms |

脚本里所有 `localhost` 已改为 `127.0.0.1`，`TimeoutSec` 提到 5。

**2. `Start-Process -NoNewWindow` 在代理环境下可能崩溃**

某些执行环境会同时注入大小写不同的代理变量（如 `HTTPS_PROXY` 与 `https_proxy`）。Windows 环境变量不区分大小写但保留原始键名，`Start-Process` 构造环境字典时可能撞键抛异常。去掉 `-NoNewWindow` 后正常。

现在 `start-server.ps1` 的启动段改为：`Start-Process` 失败 → 回退 WMI `Win32_Process.Create` → 都失败则打印手动启动命令并返回 1，不再静默失败。

### 中文输出的两个坑

1. **不要用 `{0,-26}` 定宽对齐**：中文占两列宽却只算一个字符，补位后实际宽度超出，终端折行覆盖，输出会错乱重复。
2. **整行一次 `Write-Host`**，不要用 `-NoNewline` 拼接：信息流被捕获/重定向时每个 `Write-Host` 会变成独立记录，`-NoNewline` 失效导致断行。

另外 `manage.ps1` 存为 UTF-8 **带 BOM**：PowerShell 5.1 读无 BOM 的 UTF-8 脚本会按 ANSI 解析，中文全乱码（`鏁版嵁搴撳鍣`）。

### 实测

| 操作 | 结果 |
|------|------|
| `-Action status` | 三层组件状态 + 知识库 3 文档 / 30 片段，全部正确 |
| `-Action stop` | 8080 立即释放，依赖不动 |
| `-Action stopall` | 服务 / Ollama / 容器三者全部停止 |
| 停止后再启动 | health 1 秒就绪 |
| 容器 stop→start | 3 个文档 / 30 个片段完好（数据在 volume） |

内存实测：RAG 服务 24MB、Ollama 空闲 41MB、postgres 容器 56MB、Docker Desktop 本体约 600MB —— 省内存的关键是退 Docker Desktop，停服务主要是释放端口。

### 遗留

- 启动脚本由用户本机双击实测通过。自动化执行环境可能因会话结束回收子进程，属环境差异，不影响人工使用。

---

## 2026-11-23 — 交互式测试终端 + 重启不再清库

**概述**：测试一次要手写两条长命令（`-F "file=@路径"`、`-d '{"question":...}'`），门槛高。新增 `cmd/ragcli` 交互终端：打字即提问、拖文件即上传。同时修掉「服务一重启知识库就清空」的根因。

### 新增文件

| 文件 | 作用 |
|------|------|
| `cmd/ragcli/main.go` | 交互终端：提问 / 上传 / docs / rm / topk |
| `cmd/ragcli/console_windows.go` | 把控制台代码页切到 UTF-8（65001），避免中文输入乱码 |
| `cmd/ragcli/console_other.go` | 非 Windows 空实现（build tag 隔离） |
| `start-server.ps1` | 依次拉起 postgres 容器 → Ollama → 服务 |
| `启动服务.bat` / `问答.bat` | 双击入口 |
| `ragcli.exe` | 编译产物 |

### 后端改动

| 文件 | 变更 |
|------|------|
| `repository/migration.go` | **不再每次启动 `DROP TABLE chunks`**。改为读 `pg_attribute` 判断向量维度，只有维度变化时才重建；`embeddingDim` 提为常量 |
| `repository/doc_repo.go` | 新增 `ListDocuments`（带片段数）、`DeleteDocument` |
| `repository/chunk_repo.go` | 检索 SQL join `documents` 带出 `filename`，`Chunk` 加 `Filename`/`Distance` |
| `service/rag.go` | `RAGSource` 加 `filename`/`distance`；来源截断从按字节改为按 rune（原来会把中文切成半个字） |
| `service/document.go` | 新增 `List` / `Delete` |
| `handler/document.go` | 新增 `List`（GET）、`Delete`（DELETE） |
| `cmd/server/main.go` | `/api/v1/documents` 按 method 分派到 Upload / List / Delete |

### 遇到的问题与处理

1. **PowerShell 5.1 读无 BOM 的 UTF-8 脚本按 ANSI 解析**：`start-server.ps1` 里的中文提示全部乱码（`鏁版嵁搴撳鍣`）。改法是把脚本内提示统一为纯 ASCII，并加注释说明原因。
2. **cmd 不识别 `\"` 转义**：`curl -d '{\"question\":...}'` 在 cmd 里反斜杠会原样传给服务端，Go 报 `invalid character '\'`。这也是做交互终端的直接动机——彻底绕开引号问题。
3. **Windows 拖放文件路径带引号**：`existingFile()` 会先 `Trim` 掉 `"` / `'` 再 `os.Stat`。

### 实测

| 操作 | 结果 |
|------|------|
| 重启服务后 | 3 个文档、30 个片段全部保留（改动前必然清零） |
| 拖路径上传 `开发手册.md` | 14.58s，13 块 |
| 带引号路径上传小文本文件 | 588ms，1 块 |
| 提问「有哪些接口」 | 879ms，三个接口全列出，来源带文件名 + 距离 |
| `GET /api/v1/documents` | 返回 3 条，带 `chunk_count` |
| `DELETE /api/v1/documents?id=...` | 200，chunks 级联删除 |
| `start-server.ps1` | 三步依赖全部识别为已运行，exit 0 |

### 遗留

- 同名文件重复入库不去重，靠 `rm` 手动清理
- `ragcli.exe` 需要重新编译才能带上新改动（`go build -o ragcli.exe ./cmd/ragcli`）
- 启动/关闭脚本已由用户本机双击实测通过

### 同日整理（工程化）

清理调试残留，把项目从「能跑」推到「可提交」：

| 动作 | 内容 |
|------|------|
| 删除 `tools/`（14 文件 / 172KB） | 全是调试产物：嵌入向量快照 `e_*.json`、手拼向量 SQL `q*.sql`、问答响应 `r*.json`、早期 `ask.bat`/`ask.json`（已被 ragcli 取代） |
| 删除 `server.exe`（13MB） | 2026-06-02 的旧编译产物，与当前代码不符，留着会被误双击 |
| 删除 `项目目录.txt` | 过时的目录说明，开发手册已有完整结构 |
| 删除空目录 `pkg/` | 无任何文件 |
| 新增 `.gitignore` | 忽略 `*.exe`、`.idea/`、`.workbuddy/`、`configs/config.yaml` |
| 新增 `configs/config.example.yaml` | 脱敏配置模板，新环境复制后填 key |
| `.gitignore` + 配置模板 | 真实配置只留本地，仓库仅提交脱敏模板 |
| 新增仓库根 `README.md` | 项目入口：技术栈、快速开始、接口表、设计要点、已知限制 |
| 修正手册过时表述 | 数据流里「调用 API 嵌入」改为「调用本地 Ollama 嵌入」，补上生文走远端 |

整理后体积 21MB → 8.5MB。`git check-ignore` 验证：`config.yaml` / `*.exe` / `.idea` / `.workbuddy` 均被忽略，`config.example.yaml` 与源码正常跟踪。

---

## 2026-11-23 — 切分策略重写（标题层级感知）+ 单元测试

**概述**：原 `chunkText` 按空行切分，Markdown 标题与正文被切散，检索只能拿回零信息标题碎片。重写为标题层级感知切分，并补 8 个单元测试。

### 问题表现

问「有哪些接口」时，检索返回的是 `## 数据流`、`## API 接口` 这类只有标题的块，模型答「上下文无法确定」。开发手册 4.9KB 被切成 53 块，平均 68 字。

### 根因（三层）

1. 按 `\n` + 空行切分，Markdown 每个块之间都有空行，导致标题单独成块。
2. 代码围栏内的 `#`（如 `# 1. 启动 PostgreSQL`、`# api_key: ...`）被误判为标题。
3. 父标题后紧跟子标题（`## 数据流` → `### 文档摄入`）时，父块无正文，成为零信息碎片。

### 改动（internal/service/document.go）

| 改动 | 说明 |
|------|------|
| 代码围栏感知 | ` ``` ` / `~~~` 内部的 `#` 不再当标题 |
| 标题层级切分 | 取「出现≥2次的最浅层级」作为切分点（本手册为 `##`），`###` 子节归入父块 |
| 零信息块合并 | 只有标题没有正文的块并入下一块 |
| 超长兜底 | 单行超上限硬切，单块不超 500 字 |

### 效果

| 文档 | 改前 | 改后 |
|------|------|------|
| 开发手册.md | 53 块 / 均 68 字 | 13 块 / 均 292 字 |
| README.md | 53 块 / 均 124 字 | 16 块 / 均 420 字 |

`## API 接口` 现在是一块 411 字的完整块，含健康检查 + 上传文档 + RAG 问答三个子节。提问「有哪些接口」可正确列出全部三个接口。

### 新增测试

`internal/service/chunk_test.go`，8 个用例：标题与正文同块、围栏内 `#` 忽略、纯文本按空行分段、块长上限、超长单行硬切、空文本、零信息块合并、子节归父块。`go test ./internal/service` 全绿。

### 仍存在的限制

- `SearchSimilarChunks` 无相似度阈值，库里只要有数据就一定返回 top_k 条，问无关问题会硬答。
- 0.6B 嵌入对短查询区分度有限（相关 0.47 / 无关 0.55，差距小）。
- 知识库混入无关文档时会互相干扰，模型会自行区分但答案变长。

---

## 2026-11-23 — 环境重建 + 全链路重新跑通（DeepSeek 生文 + 本地 0.6B 嵌入）

**概述**：项目停滞 5 个多月后重新启动。原远端供应商的 key 已失效，改为 DeepSeek 官方生文 + 本地 Ollama 嵌入的混合方案，端到端跑通。

### 环境

| 组件 | 版本/位置 | 说明 |
|------|-----------|------|
| Ollama | 0.23.3 | 本地安装；模型目录通过 `OLLAMA_MODELS` 环境变量指定 |
| 嵌入模型 | `qwen3embed`（Qwen3-Embedding-0.6B-Q8_0.gguf，610MB） | 1024 维，CPU ≈450ms/条 |
| 生文 | `deepseek-chat`（api.deepseek.com） | 实际路由到 `deepseek-flash` |
| 数据库 | Docker `rag-postgres`（pgvector 0.8.2） | 5432 端口 |

### 关键决策

- `registry.ollama.ai` 与 HuggingFace 均无法连接（TLS 失败），改从**魔搭 ModelScope** 下 GGUF（10-15MB/s），再用 `Modelfile` + `ollama create` 导入，与原教程一致。
- DeepSeek 无 embeddings 接口，因此 `strategy.embed=ollama`、`strategy.generate=api`，正好复用项目原有的策略路由设计。
- 选用 0.6B 嵌入而非 bge-m3：本机超过 1B 的模型会明显卡顿；且 1024 维可直接匹配 `migration.go` 写死的 `vector(1024)`，无需改表。

### 配置改动（configs/config.yaml）

`api.base_url` → `https://api.deepseek.com/v1`；`api.llm_model` → `deepseek-chat`；`api.embed_model` 留空；`ollama.embed_model` → `qwen3embed`；`strategy.embed` → `ollama`。

### 实测

| 操作 | 耗时 | 结果 |
|------|------|------|
| 上传小文本文件（288B，1 chunk） | 736ms | 200，`document_id` 正常返回 |
| 问答「有哪些接口」 | 1108ms | 回答准确，1 条来源 |
| 上传 `开发手册.md`（4.9KB，53 chunks） | 16.2s | 200 |
| 问答「什么嵌入模型/向量维度」 | 1325ms | 正确指出上下文未说明维度与模型的对应关系 |

### 发现的问题

- `repository/migration.go` 每次启动 `DROP TABLE chunks CASCADE`，导致 `documents` 表残留孤儿记录（`intro.txt`、`test_rag.txt` 均为 0 chunks）。做正式版前必须去掉。
- `configs/config.yaml` 含明文 API key，已加入 `.gitignore` 并用 `config.example.yaml` 作为脱敏模板；公开仓库只提交模板。

---

## 2026-06-02 — 端到端验证通过

**概述**：DeepSeek-V4-Pro + bge-large-zh-v1.5 全链路跑通，12 秒完成问答。

### 测试结果

```
Q: Go-RAG-Service 有哪些接口？
A: 提供文档上传和问答两个核心 API，上传的文档会自动分段、
   嵌入向量并存入 pgvector，查询时检索相关片段供大模型生成答案。

文档上传: 0.8s
RAG 问答: 12s (嵌入 + 检索 + 生文)
```

### 修复

| 问题 | 修复 |
|------|------|
| `nomic-embed-text`(768维) 与 `bge-large`(1024维) 不兼容 | migration 改为 `vector(1024)`，启动时 DROP CASCADE 重建 |
| Ollama 未启动导致嵌入失败 | `strategy.embed` 切为 `api` |

### 当前配置

- Embedding: `BAAI/bge-large-zh-v1.5` (硅基流动, 1024维)
- 生文: `deepseek-ai/DeepSeek-V4-Pro` (硅基流动)
- 策略: 嵌入和生文均走 API，不依赖本地 Ollama

---

## 2026-06-02 — 策略控制：Embed/Generate 分离路由

**概述**：本地 0.8B 模型生文超时，改为策略配置让 Embedding 走本地、Generate 走 API。新增 `strategy.embed` / `strategy.generate` 配置项。

### 问题

- 轻薄本上 `qwen3.5-0.8b` 即使简单 `你好` 也超时（180s+）
- `nomic-embed-text`（137M）嵌入速度正常（~1s），检索链路完好

### 修改

| 文件 | 变更 |
|------|------|
| `configs/config.yaml` | 新增 `strategy.embed` / `strategy.generate` |
| `internal/config/config.go` | 新增 `StrategyConfig` 结构体 |
| `internal/ai/fallback.go` | 改为按 strategy 分配 provider，不再 try-fallback |

---

## 2026-06-02 — 模型名修正 + 远端切换

**概述**：修复 Ollama 模型名 typo，远端从 OpenAI 切到硅基流动。

### 修改

| 文件 | 变更 |
|------|------|
| `configs/config.yaml` | `wen3.5-4b` → `qwen3.5-4b:latest`；base_url: `api.openai.com` → `api.siliconflow.cn` |
| `internal/ai/fallback.go` | 错误信息同时展示两端失败原因 |

---

## 2026-06-02 — 启动问题修复 + 开发环境完善

**概述**：修复 PostgreSQL 连接失败的启动体验，配 Docker Compose。

### 新增

| 文件 | 说明 |
|------|------|
| `docker-compose.yml` | PostgreSQL 16 + pgvector 容器 |
| `docs/devlog.md` | 本文档 |

### 修改

| 文件 | 变更 |
|------|------|
| `cmd/server/main.go` | DB 连接失败给出 Docker Compose 提示，优雅退出 |

---

## 2026-06-02 — 双通道 AI 架构搭建

**概述**：完成核心 RAG 架构，本地 Ollama 优先 + 远端 API 兜底的双通道设计。

### 新增文件

`internal/ai/provider.go`、`internal/ai/ollama.go`、`internal/ai/openai.go`、`internal/ai/fallback.go`、`internal/service/document.go`、`internal/service/rag.go`、`internal/handler/document.go`、`internal/handler/rag.go`、`internal/repository/doc_repo.go`

### 架构决策

- 双向 fallback 策略
- 段落 + 500 字符分段
- OpenAI 兼容 API 格式

---
