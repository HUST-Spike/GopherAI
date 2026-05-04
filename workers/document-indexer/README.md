# GopherAI Document Indexer

第一阶段 Worker 只负责消费 RabbitMQ 文档上传事件，并 mock 更新 MySQL 状态，不执行真实 LlamaIndex、Embedding 或 Milvus 写入。

本地运行方式由操作者手动执行：

```powershell
conda activate gopherai
cd workers/document-indexer
python main.py
```

需要的环境变量可放在当前目录 `.env` 中，也可以直接设置到 shell。
