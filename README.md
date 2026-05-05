# GopherAI

GopherAI 是一个面向学习和原型验证的智能问答系统。项目以 Go 后端为主，Vue 提供聊天与文件上传界面，Python Worker 负责异步文档索引，Milvus 承担向量 / BM25 检索。当前已经跑通“上传文档 -> 建索引 -> 会话内检索 -> 工具增强聊天”的完整链路。

## 当前能力

- 用户注册、登录、会话管理和聊天历史。
- 会话级文档上传，文档只在当前用户和当前会话内可检索。
- 异步 RAG 索引链路：Go 上传文件，RabbitMQ 派发任务，Python Worker 切分、向量化并写入 Milvus。
- 统一聊天入口：SmartModel 根据问题自动决定是否调用工具、检索文档或执行技能。
- 前端支持流式回答、Markdown 渲染、工具调用卡片、Skills 下拉选择。
- 已建立 RAG 检索评估脚本和实验记录，可重复对比切分、RRF、rerank 等策略。

## 架构设计

核心链路如下：

```text
Vue 前端
-> Go API
-> MySQL / Redis / RabbitMQ
-> Python document-indexer
-> LlamaIndex 切分
-> Embedding
-> Milvus 向量库
-> Go SmartModel / RAG Retriever
-> LLM 回答
```

文档处理采用异步设计：

```text
上传文件
-> 本地 uploads 落盘
-> MySQL documents / document_index_jobs 留痕
-> RabbitMQ document.uploaded
-> Python Worker 消费
-> LlamaIndex 切分
-> 智谱 embedding-3 向量化
-> Milvus 写入 dense vector + BM25 sparse vector
```

检索时严格使用：

```text
user_name + session_id
```

作为过滤条件，避免跨用户、跨会话召回。

## 技术栈

**前端**

- Vue 3
- Vue Router
- Element Plus
- Axios

**Go 后端**

- Gin
- GORM
- MySQL
- Redis
- RabbitMQ
- Milvus Go SDK
- Eino / OpenAI-compatible LLM 调用
- JWT 鉴权

**Python Worker**

- Python 3.11
- LlamaIndex
- PyMilvus
- SQLAlchemy + PyMySQL
- pika

**本地基础设施**

- MySQL
- Redis
- RabbitMQ
- Milvus standalone
- Attu

基础设施通过 `deploy/docker-compose.yml` 启动。

## LLM 应用策略

当前项目不是简单聊天壳，而是把 RAG、工具调用和技能能力统一到一个 SmartModel 聊天入口中。

### RAG 切分策略

经过评测后，当前采用 Markdown-aware 切分：

```env
CHUNK_STRATEGY=markdown_sentence_splitter
CHUNK_SIZE=1000
CHUNK_OVERLAP=150
MARKDOWN_MAX_SECTION_CHARS=1600
```

选择原因：它在当前测试语料中比纯 sentence splitter 和 semantic splitter 更稳定，能更好保留 Markdown 文档的章节边界。

### 检索增强策略

当前生产式检索链路可以开启：

```text
dense top50 + BM25 top50
-> RRF 融合
-> 智谱 rerank
-> final top5 注入上下文
```

关键配置包括：

```env
RAG_FUSION_ENABLED=true
RAG_FUSION_STRATEGY=rrf
RAG_RERANK_ENABLED=true
RAG_RERANK_PROVIDER=zhipu
RAG_RERANK_SCORE_MODE=rerank_only
```

评测结论简述：

- Markdown 切分后，`doc@5` 达到 `1.0`，`anchor@5` 达到 `0.9917`。
- 引入 rerank 后，`anchor@5` 提升到 `1.0`。
- RRF + rerank 保留了 dense / BM25 / rerank 分数，便于后续观测和调参。

详细实验记录见：

```text
test/rag-eval/chunking-strategy-experiment-summary.md
test/rag-eval/retrieval-fusion-rerank-summary.md
```

### Tools 与 Skills

SmartModel 支持自动工具调用，前端会把调用过程展示为可折叠工具卡片，包含参数、结果预览、状态、耗时和重试次数。

当前能力包括：

- 时间、计算、网页抓取、搜索、文档列表、会话文档检索等全局工具。
- Skills 下拉选择，当前支持编程、数据分析、翻译、写作等技能。
- `run_python` 仅在编程 / 数据分析技能启用后可见。
- 工具调用记录会写入 `tool_invocations`，并带有 `trace_id`，方便排查。

## 配置说明

主要配置文件：

```text
config/config.toml                 # Go 服务基础配置
config/.env                        # Go 侧 LLM / embedding / Milvus / RAG 配置
workers/document-indexer/.env      # Python Worker 索引配置
vue-frontend/vue.config.js         # 前端代理配置
```

示例文件：

```text
config/.env.example
workers/document-indexer/.env.example
```

不要提交真实 API Key。

## 本地运行

建议启动顺序：

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

### 4. 启动前端

```powershell
cd D:\work\Go\GopherAI\vue-frontend
npm install
npm run serve
```

默认地址：

```text
http://localhost:8080
```

前端代理：

```text
/api -> http://localhost:9090/api/v1
```

可视化工具：

```text
RabbitMQ: http://localhost:15672
Attu:     http://localhost:8000
```

## 快速验证

### 后端和前端检查

```powershell
cd D:\work\Go\GopherAI
go test ./...
go build -buildvcs=false ./...
```

```powershell
cd D:\work\Go\GopherAI\vue-frontend
npm run lint
npm run build
```

### RAG 手动验证

启动 Go、Python Worker、Milvus、RabbitMQ 后：

1. 登录前端。
2. 新建或选择一个会话。
3. 上传 `.md` 或 `.txt` 文档。
4. 等待文档索引完成。
5. 在同一个会话中提问文档相关问题。

也可以使用手动 smoke 脚本：

```powershell
cd D:\work\Go\GopherAI
powershell -ExecutionPolicy Bypass -File .\test\manual\upload-rag-smoke.ps1
powershell -ExecutionPolicy Bypass -File .\test\manual\chat-rag-smoke.ps1 -SessionID "<upload script returned session_id>"
```

## 项目目录

```text
common/                      Go 公共能力：LLM、RAG、MCP、Skill、RabbitMQ 等
controller/                  HTTP Controller
service/                     业务逻辑
dao/                         数据访问
model/                       数据模型
config/                      Go 配置
workers/document-indexer/    Python 文档索引 Worker
vue-frontend/                Vue 前端
deploy/                      Docker Compose 基础设施
test/rag-eval/               RAG 评测配置与实验结果
test/manual/                 手动 smoke test
doc/                         设计文档和阶段记录
```

## 当前阶段总结

GopherAI 当前已经具备一个可演示的 LLM 应用闭环：

- 有用户和会话体系。
- 有会话级文档上传和异步索引。
- 有 Milvus 检索、BM25 融合和 rerank。
- 有 SmartModel 工具调用与 Skills。
- 有前端流式交互和工具调用可视化。
- 有可复现实验记录支撑 RAG 策略选择。

后续可以继续完善：更系统的观测面板、更多文件格式解析、查询改写、更完整的测试覆盖，以及面向真实使用场景的产品化体验。
