# GopherAI

GopherAI 是一个以 Go 后端为主体、Python Worker 承担文档索引任务的学习型 AI 项目。当前阶段重点已经打通：

```text
文件上传 -> 本地落盘 -> MySQL 留档 -> RabbitMQ 异步消息 -> Python Worker -> LlamaIndex 切分 -> Embedding -> Milvus 写入
```

后续会在此基础上继续接入 Go 侧 RAG 检索和聊天链路。

## 技术栈

### Go 后端

- Go 1.24
- Gin：HTTP API
- GORM：MySQL ORM
- MySQL 8：用户、会话、文档、索引任务等结构化数据
- Redis：验证码、缓存等基础能力
- RabbitMQ：文件上传后的异步索引事件
- JWT：用户鉴权
- Eino / OpenAI-compatible SDK：LLM 调用与 Agent / Skill 能力的基础设施

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

## LLM 与 Embedding 选型

当前项目把聊天模型和文档向量化模型分开处理：

- Go 聊天模型：当前配置使用 DeepSeek 的 OpenAI-compatible 接口，配置项在 `config/config.toml`。
- Python 文档 embedding：当前使用智谱 OpenAI-compatible embedding 接口，模型为 `embedding-3`，向量维度为 `1024`。
- Milvus 向量检索配置：`COSINE` 距离，HNSW 索引。
- 当前切分参数：`CHUNK_SIZE=800`，`CHUNK_OVERLAP=120`。

API Key 不提交到仓库，Python Worker 的真实密钥放在：

```text
workers/document-indexer/.env
```

## 文档上传与索引流程

用户上传文档后，Go 后端会：

1. 将文件保存到 `uploads/<username>/<document_id>.<ext>`。
2. 写入 `documents` 表。
3. 写入 `document_index_jobs` 表。
4. 发送 RabbitMQ 消息到：

```text
exchange: gopherai.document
queue: gopherai.document.index
routing_key: document.uploaded
```

Python Worker 收到消息后会：

1. 从 MySQL 读取文档记录。
2. 将状态更新为 `indexing`。
3. 读取本地文件。
4. 使用 LlamaIndex 切分 chunk。
5. 调用 embedding API 生成向量。
6. 写入 Milvus collection：`gopherai_document_chunks_v1`。
7. 成功后将状态更新为 `indexed`，失败则更新为 `index_failed`。

Milvus chunk 会带上 `user_name` 和 `session_id`，后续 RAG 检索必须严格按用户和会话过滤。

## 可观测性

当前已经保留了几个开发期排障入口：

- Go 日志：`logs/go/gopherai.log`
- Python Worker 日志：`logs/python/document-indexer.log`
- RabbitMQ 管理台：`http://localhost:15672`
- Attu Milvus GUI：`http://localhost:8000`
- 文档查询接口：`GET /api/v1/documents`、`GET /api/v1/documents/:id`
- 上传链路使用 `trace_id` 串联 Go 日志、MQ 消息、MySQL 记录和 Worker 日志。

## 本地启动

启动基础设施：

```powershell
cd D:\work\Go\GopherAI\deploy
docker compose up -d
docker compose ps
```

启动 Go 后端：

```powershell
cd D:\work\Go\GopherAI
go run .
```

启动 Python Worker：

```powershell
conda activate gopherai
cd D:\work\Go\GopherAI\workers\document-indexer
python main.py
```

访问可视化工具：

```text
RabbitMQ: http://localhost:15672
Attu:     http://localhost:8000
```

## 快速验证

项目提供了一个最小链路测试脚本：

```powershell
cd D:\work\Go\GopherAI
powershell -ExecutionPolicy Bypass -File .\test\manual\upload-rag-smoke.ps1
```

成功时文档状态应从 `queued` 变为 `indexed`，并且 `chunk_count` 大于 0。随后可以在 Attu 中查看：

```text
collection: gopherai_document_chunks_v1
```

## 相关文档

- `doc/async-rag-local-runbook.md`：本地异步 RAG 链路启动与联调
- `doc/milvus-indexing-plan.md`：Milvus 文档索引设计
- `doc/attu-milvus-ui-runbook.md`：Attu 可视化工具使用说明
- `doc/gopherai-rag-context-handoff.md`：当前 RAG / Milvus 阶段上下文沉淀
- `test/rag-corpus-generation-guide.md`：后续召回评估测试语料生成指南
