from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime
from typing import Any

from sqlalchemy import create_engine, text
from sqlalchemy.engine import Engine


@dataclass
class DocumentRecord:
    id: str
    user_name: str
    status: str
    updated_at: datetime | None


class DocumentRepository:
    def __init__(self, mysql_dsn: str):
        self.engine: Engine = create_engine(mysql_dsn, pool_pre_ping=True, future=True)

    def get_document(self, document_id: str) -> DocumentRecord | None:
        with self.engine.begin() as conn:
            row = conn.execute(
                text(
                    """
                    SELECT id, user_name, status, updated_at
                    FROM documents
                    WHERE id = :document_id AND deleted_at IS NULL
                    """
                ),
                {"document_id": document_id},
            ).mappings().first()
        if row is None:
            return None
        return DocumentRecord(
            id=row["id"],
            user_name=row["user_name"],
            status=row["status"],
            updated_at=row["updated_at"],
        )

    def ensure_job(self, event: dict[str, Any], worker_id: str, queue_name: str) -> str:
        job_id = event.get("job_id") or event["event_id"]
        with self.engine.begin() as conn:
            conn.execute(
                text(
                    """
                    INSERT INTO document_index_jobs
                        (id, document_id, event_id, trace_id, user_name, queue_name, status, attempt, worker_id, created_at, updated_at)
                    VALUES
                        (:id, :document_id, :event_id, :trace_id, :user_name, :queue_name, 'queued', 1, :worker_id, NOW(), NOW())
                    ON DUPLICATE KEY UPDATE
                        updated_at = updated_at
                    """
                ),
                {
                    "id": job_id,
                    "document_id": event["document_id"],
                    "event_id": event["event_id"],
                    "trace_id": event.get("trace_id", ""),
                    "user_name": event.get("user_name", ""),
                    "queue_name": queue_name,
                    "worker_id": worker_id,
                },
            )
        return job_id

    def mark_running(self, document_id: str, job_id: str, worker_id: str) -> None:
        with self.engine.begin() as conn:
            conn.execute(
                text(
                    """
                    UPDATE documents
                    SET status = 'indexing', error_message = '', updated_at = NOW()
                    WHERE id = :document_id
                    """
                ),
                {"document_id": document_id},
            )
            conn.execute(
                text(
                    """
                    UPDATE document_index_jobs
                    SET status = 'running', worker_id = :worker_id, started_at = NOW(), error_message = '', updated_at = NOW()
                    WHERE id = :job_id
                    """
                ),
                {"job_id": job_id, "worker_id": worker_id},
            )

    def mark_succeeded(self, document_id: str, job_id: str, mock: bool) -> None:
        status = "indexed_mock" if mock else "indexed"
        with self.engine.begin() as conn:
            conn.execute(
                text(
                    """
                    UPDATE documents
                    SET status = :status, error_message = '', indexed_at = NOW(), updated_at = NOW()
                    WHERE id = :document_id
                    """
                ),
                {"document_id": document_id, "status": status},
            )
            conn.execute(
                text(
                    """
                    UPDATE document_index_jobs
                    SET status = 'succeeded', finished_at = NOW(), error_message = '', updated_at = NOW()
                    WHERE id = :job_id
                    """
                ),
                {"job_id": job_id},
            )

    def mark_failed(self, document_id: str, job_id: str, error_message: str) -> None:
        with self.engine.begin() as conn:
            conn.execute(
                text(
                    """
                    UPDATE documents
                    SET status = 'index_failed', error_message = :error_message, updated_at = NOW()
                    WHERE id = :document_id
                    """
                ),
                {"document_id": document_id, "error_message": error_message},
            )
            conn.execute(
                text(
                    """
                    UPDATE document_index_jobs
                    SET status = 'failed', finished_at = NOW(), error_message = :error_message, updated_at = NOW()
                    WHERE id = :job_id
                    """
                ),
                {"job_id": job_id, "error_message": error_message},
            )


def is_recent(updated_at: datetime | None, seconds: int = 300) -> bool:
    if updated_at is None:
        return False
    if updated_at.tzinfo is None:
        return (datetime.now() - updated_at).total_seconds() < seconds
    return (datetime.now(updated_at.tzinfo) - updated_at).total_seconds() < seconds
