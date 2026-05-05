# GopherAI

GopherAI 是一个以 Go 后端为主体、Vue 前端为交互入口、Python Worker 承担文档索引任务的学习型 AI 项目。当前已经跑通端到端 RAG 主链路：

```text
文件上传
-> 本地落盘
-> MySQL 留档
-> RabbitMQ 异步消息
-> Python Worker
-> LlamaIndex 切分
-> Embedding
-> Milvus 写入
-> Go 侧 Milvus 检索
-> 聊天 LLM 基于参考资料回答
```

当前阶段重点已经从“链路打通”进入“可观测、可测试、切分策略和召回效果优化”。

## 技术栈

### 前端

- Vue 3
- Vue Router
- Element Plus
- Axios
- Vue CLI dev server

前端目录：

```text
vue-frontend/
```

开发环境中，前端运行在 `http://localhost:8080`，并通过 `vue.config.js` 将 `/api` 代理到 Go 后端：

```text
/api -> http://localhost:9090/api/v1
```

### Go 后端

- Go 1.24
- Gin：HTTP API
- GORM：MySQL ORM
- MySQL 8：用户、会话、文档、索引任务等结构化数据
- Redis：验证码等基础能力
- RabbitMQ：文件上传后的异步索引事件
- Milvus Go SDK：聊天时直接检索向量库
- Eino / OpenAI-compatible SDK：LLM 调用与 Agent / Skill 能力基础设施
- JWT：用户鉴权

### Python 文档索引 Worker

- Python 3.11
- pika：消费 RabbitMQ 消息
- SQLAlchemy + PyMySQL：读写 MySQL
- LlamaIndex：文档切分与 embedding 调用
- PyMilvus：写入 Milvus collection
- python-dotenv：本地 `.env` 配置

### 本地基础设施

通过 `deploy/docker-compose.yml` 启动：

- MySQL：`4306 -> 3306`
- Redis：`6379`
- RabbitMQ Management：`5672` / `15672`
- Milvus standalone：`19530` / `9091`
- Attu：Milvus Web GUI，`8000 -> 3000`

## LLM 与 Embedding

当前项目把聊天模型和文档向量化模型分开处理：

- Go 聊天模型：当前配置使用 DeepSeek 的 OpenAI-compatible 接口，配置项在 `config/config.toml`。
- Python 文档 embedding：当前使用智谱 OpenAI-compatible embedding 接口，模型为 `embedding-3`，向量维度为 `1024`。
- Go RAG query embedding：复用同一套智谱 embedding 配置，保证查询向量和入库向量同源。
- Milvus 向量检索：`COSINE` 距离，HNSW 索引。
- 当前基础切分参数：`CHUNK_SIZE=800`，`CHUNK_OVERLAP=120`。

真实 API Key 不提交到仓库。开发期 embedding 相关配置放在：

```text
workers/document-indexer/.env
```

Go 启动时会加载 `config/.env` 和 `workers/document-indexer/.env`，因此 Go RAG 检索可以复用 Python Worker 的 embedding 配置。

## 文档上传与索引流程

用户上传文档后，Go 后端会：

1. 将文件保存到 `uploads/<username>/<document_id>.<ext>`。
2. 写入 `documents` 表。
3. 写入 `document_index_jobs` 表。
4. 发送 RabbitMQ 消息。

MQ 配置：

```text
exchange: gopherai.document
queue: gopherai.document.index
routing_key: document.uploaded
```

Python Worker 收到消息后会：

1. 从 MySQL 读取文档记录。
2. 将文档状态更新为 `indexing`。
3. 读取本地 `.md` / `.txt` 文件。
4. 使用 LlamaIndex 切分 chunk。
5. 调用 embedding API 生成向量。
6. 写入 Milvus collection：`gopherai_document_chunks_v1`。
7. 成功后更新为 `indexed`，失败则更新为 `index_failed`。

Milvus chunk 会带上 `user_name` 和 `session_id`，RAG 检索必须严格按当前用户和当前会话过滤。

聊天时选择 `modelType=2` 会进入 Go 侧 Milvus RAG：

```text
用户问题
-> Go 生成 query embedding
-> Milvus 按 user_name + session_id 检索
-> 拼接参考资料
-> 调用聊天 LLM
```

默认 RAG 参数：

```text
RAG_TOP_K=5
RAG_MAX_CONTEXT_CHARS=6000
RAG_RETRIEVAL_FAIL_OPEN=false
```

## 可观测性

当前已经保留了几个开发期排障入口：

- Go 日志：`logs/go/gopherai.log`
- Python Worker 日志：`logs/python/document-indexer.log`
- RabbitMQ 管理台：`http://localhost:15672`
- Attu Milvus GUI：`http://localhost:8000`
- 文档查询接口：`GET /api/v1/documents`、`GET /api/v1/documents/:id`
- 上传链路使用 `trace_id` 串联 Go 日志、MQ 消息、MySQL 记录和 Worker 日志。

后续会继续补齐更系统的可观测体系和测试计划。

## 本地启动

建议按这个顺序启动：

```text
Docker 基础设施 -> Go 后端 -> Python Worker -> Vue 前端
```

### 1. 启动基础设施

```powershell
cd D:\work\Go\GopherAI\deploy
docker compose up -d
docker compose ps
```

### 2. 启动 Go 后端

```powershell
cd D:\work\Go\GopherAI
go run .
```

默认地址：

```text
http://localhost:9090
```

### 3. 启动 Python Worker

```powershell
conda activate gopherai
cd D:\work\Go\GopherAI\workers\document-indexer
python main.py
```

### 4. 启动 Vue 前端

首次安装依赖：

```powershell
cd D:\work\Go\GopherAI\vue-frontend
npm install
```

启动开发服务器：

```powershell
cd D:\work\Go\GopherAI\vue-frontend
npm run serve
```

默认访问：

```text
http://localhost:8080
```

### 5. 访问可视化工具

```text
RabbitMQ: http://localhost:15672
Attu:     http://localhost:8000
```

## 快速验证

上传并等待索引完成：

```powershell
cd D:\work\Go\GopherAI
powershell -ExecutionPolicy Bypass -File .\test\manual\upload-rag-smoke.ps1
```

成功时文档状态应从 `queued` 变为 `indexed`，并且 `chunk_count` 大于 0。

随后使用返回的 `session_id` 测试 RAG 聊天：

```powershell
cd D:\work\Go\GopherAI
powershell -ExecutionPolicy Bypass -File .\test\manual\chat-rag-smoke.ps1 -SessionID "<upload script returned session_id>"
```

成功时回答中应包含：

```text
gopherai-milvus-smoke-anchor-20260504
```

也可以在 Attu 中查看：

```text
collection: gopherai_document_chunks_v1
```

## 测试语料

后续 RAG 召回评估使用的测试语料放在：

```text
test/rag-corpus/
```

生成规范见：

```text
test/rag-corpus-generation-guide.md
```

## 相关文档

- `doc/async-rag-local-runbook.md`：本地异步 RAG 链路启动与联调
- `doc/milvus-indexing-plan.md`：Milvus 文档索引设计
- `doc/go-milvus-rag-retrieval-plan.md`：Go 侧 Milvus RAG 检索链路设计
- `doc/attu-milvus-ui-runbook.md`：Attu 可视化工具使用说明
- `doc/gopherai-rag-context-handoff.md`：当前 RAG / Milvus 阶段上下文沉淀
- `test/rag-corpus-generation-guide.md`：后续召回评估测试语料生成指南
