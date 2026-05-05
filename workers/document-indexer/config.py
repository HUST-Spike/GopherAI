import os
from dataclasses import dataclass
from pathlib import Path
from typing import Tuple

from dotenv import load_dotenv


DEFAULT_PROJECT_ROOT = Path(__file__).resolve().parents[2]
WORKER_ENV_PATH = Path(__file__).resolve().with_name(".env")

load_dotenv(WORKER_ENV_PATH)


@dataclass(frozen=True)
class WorkerConfig:
    project_root: str
    mysql_dsn: str
    mysql_session_time_zone: str
    rabbitmq_url: str
    exchange: str
    queue: str
    routing_key: str
    worker_id: str
    mock_index: bool
    mock_sleep_seconds: float
    delayed_retry_seconds: int
    embedding_api_key: str
    embedding_base_url: str
    embedding_model: str
    embedding_dimension: int
    embedding_batch_size: int
    milvus_uri: str
    milvus_collection: str
    milvus_insert_batch_size: int
    chunk_size: int
    chunk_overlap: int
    index_max_attempts: int
    index_retry_delays: Tuple[int, ...]


def load_config() -> WorkerConfig:
    return WorkerConfig(
        project_root=os.getenv("PROJECT_ROOT", str(DEFAULT_PROJECT_ROOT)),
        mysql_dsn=os.getenv(
            "MYSQL_DSN",
            "mysql+pymysql://root:123456@127.0.0.1:4306/GopherAI?charset=utf8mb4",
        ),
        mysql_session_time_zone=os.getenv("MYSQL_SESSION_TIME_ZONE", "+08:00"),
        rabbitmq_url=os.getenv(
            "RABBITMQ_URL",
            "amqp://root:123456@127.0.0.1:5672/%2F",
        ),
        exchange=os.getenv("DOCUMENT_INDEX_EXCHANGE", "gopherai.document"),
        queue=os.getenv("DOCUMENT_INDEX_QUEUE", "gopherai.document.index"),
        routing_key=os.getenv("DOCUMENT_INDEX_ROUTING_KEY", "document.uploaded"),
        worker_id=os.getenv("WORKER_ID", "document-indexer-local-1"),
        mock_index=os.getenv("MOCK_INDEX", "true").lower() == "true",
        mock_sleep_seconds=float(os.getenv("MOCK_SLEEP_SECONDS", "0")),
        delayed_retry_seconds=int(os.getenv("DELAYED_RETRY_SECONDS", "30")),
        embedding_api_key=os.getenv("EMBEDDING_API_KEY", ""),
        embedding_base_url=os.getenv(
            "EMBEDDING_BASE_URL",
            "https://open.bigmodel.cn/api/paas/v4",
        ),
        embedding_model=os.getenv("EMBEDDING_MODEL", "embedding-3"),
        embedding_dimension=int(os.getenv("EMBEDDING_DIMENSION", "1024")),
        embedding_batch_size=int(os.getenv("EMBEDDING_BATCH_SIZE", "16")),
        milvus_uri=os.getenv("MILVUS_URI", "http://127.0.0.1:19530"),
        milvus_collection=os.getenv("MILVUS_COLLECTION", "gopherai_document_chunks_v1"),
        milvus_insert_batch_size=int(os.getenv("MILVUS_INSERT_BATCH_SIZE", "64")),
        chunk_size=int(os.getenv("CHUNK_SIZE", "1000")),
        chunk_overlap=int(os.getenv("CHUNK_OVERLAP", "150")),
        index_max_attempts=int(os.getenv("INDEX_MAX_ATTEMPTS", "3")),
        index_retry_delays=_parse_retry_delays(os.getenv("INDEX_RETRY_DELAYS", "2,5")),
    )


def _parse_retry_delays(value: str) -> Tuple[int, ...]:
    if not value.strip():
        return ()
    return tuple(int(item.strip()) for item in value.split(",") if item.strip())
