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
    session_id: str
    status: str
    original_filename: str
    file_path: str
    file_ext: str
    mime_type: str
    sha256: str
    storage_backend: str
    index_version: int
    updated_at: datetime | None


class DocumentRepository:
    def __init__(self, mysql_dsn: str, mysql_session_time_zone: str = "+08:00"):
        self.engine: Engine = create_engine(
            mysql_dsn,
            pool_pre_ping=True,
            future=True,
            connect_args={"init_command": f"SET time_zone = '{mysql_session_time_zone}'"},
        )

    def get_document(self, document_id: str) -> DocumentRecord | None:
        with self.engine.begin() as conn:
            row = conn.execute(
                text(
                    """
                    SELECT
                        id,
                        user_name,
                        session_id,
                        status,
                        original_filename,
                        file_path,
                        file_ext,
                        mime_type,
                        sha256,
                        storage_backend,
                        index_version,
                        updated_at
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
            session_id=row["session_id"] or "",
            status=row["status"],
            original_filename=row["original_filename"],
            file_path=row["file_path"],
            file_ext=row["file_ext"],
            mime_type=row["mime_type"] or "",
            sha256=row["sha256"] or "",
            storage_backend=row["storage_backend"] or "local",
            index_version=row["index_version"] or 1,
            updated_at=row["updated_at"],
        )

    def ensure_job(self, event: dict[str, Any], worker_id: str, queue_name: str) -> str:
        job_id = event.get("job_id") or event["event_id"]
        with self.engine.begin() as conn:
            conn.execute(
                text(
                    """
                    INSERT INTO document_index_jobs
                        (id, document_id, event_id, trace_id, user_name, queue_name, status, attempt, worker_id, process_attempts, created_at, updated_at)
                    VALUES
                        (:id, :document_id, :event_id, :trace_id, :user_name, :queue_name, 'queued', 1, :worker_id, 1, NOW(), NOW())
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

    def mark_succeeded(
        self,
        document_id: str,
        job_id: str,
        mock: bool,
        chunk_count: int = 0,
        milvus_collection: str = "",
        embedding_model: str = "",
        embedding_dimension: int = 0,
        duration_ms: int = 0,
        process_attempts: int = 1,
    ) -> None:
        status = "indexed_mock" if mock else "indexed"
        with self.engine.begin() as conn:
            conn.execute(
                text(
                    """
                    UPDATE documents
                    SET status = :status,
                        chunk_count = :chunk_count,
                        error_message = '',
                        indexed_at = NOW(),
                        updated_at = NOW()
                    WHERE id = :document_id
                    """
                ),
                {"document_id": document_id, "status": status, "chunk_count": chunk_count},
            )
            conn.execute(
                text(
                    """
                    UPDATE document_index_jobs
                    SET status = 'succeeded',
                        chunk_count = :chunk_count,
                        milvus_collection = :milvus_collection,
                        embedding_model = :embedding_model,
                        embedding_dimension = :embedding_dimension,
                        duration_ms = :duration_ms,
                        process_attempts = :process_attempts,
                        finished_at = NOW(),
                        error_message = '',
                        updated_at = NOW()
                    WHERE id = :job_id
                    """
                ),
                {
                    "job_id": job_id,
                    "chunk_count": chunk_count,
                    "milvus_collection": milvus_collection,
                    "embedding_model": embedding_model,
                    "embedding_dimension": embedding_dimension,
                    "duration_ms": duration_ms,
                    "process_attempts": process_attempts,
                },
            )

    def mark_failed(
        self,
        document_id: str,
        job_id: str,
        error_message: str,
        duration_ms: int = 0,
        process_attempts: int = 1,
        milvus_collection: str = "",
        embedding_model: str = "",
        embedding_dimension: int = 0,
    ) -> None:
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
                    SET status = 'failed',
                        milvus_collection = :milvus_collection,
                        embedding_model = :embedding_model,
                        embedding_dimension = :embedding_dimension,
                        duration_ms = :duration_ms,
                        process_attempts = :process_attempts,
                        finished_at = NOW(),
                        error_message = :error_message,
                        updated_at = NOW()
                    WHERE id = :job_id
                    """
                ),
                {
                    "job_id": job_id,
                    "error_message": error_message,
                    "duration_ms": duration_ms,
                    "process_attempts": process_attempts,
                    "milvus_collection": milvus_collection,
                    "embedding_model": embedding_model,
                    "embedding_dimension": embedding_dimension,
                },
            )


def is_recent(updated_at: datetime | None, seconds: int = 300) -> bool:
    if updated_at is None:
        return False
    if updated_at.tzinfo is None:
        return (datetime.now() - updated_at).total_seconds() < seconds
    return (datetime.now(updated_at.tzinfo) - updated_at).total_seconds() < seconds
