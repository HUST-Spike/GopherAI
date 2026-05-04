import os
from dataclasses import dataclass

from dotenv import load_dotenv


load_dotenv()


@dataclass(frozen=True)
class WorkerConfig:
    mysql_dsn: str
    rabbitmq_url: str
    exchange: str
    queue: str
    routing_key: str
    worker_id: str
    mock_index: bool
    mock_sleep_seconds: float
    delayed_retry_seconds: int


def load_config() -> WorkerConfig:
    return WorkerConfig(
        mysql_dsn=os.getenv(
            "MYSQL_DSN",
            "mysql+pymysql://root:123456@127.0.0.1:4306/GopherAI?charset=utf8mb4",
        ),
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
    )
