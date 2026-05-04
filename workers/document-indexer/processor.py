from __future__ import annotations

import threading
import time
from typing import Any

from indexer import RealDocumentIndexer
from repository import DocumentRepository, is_recent


TERMINAL_STATUSES = {"indexed", "indexed_mock"}


class DocumentProcessor:
    def __init__(self, repo: DocumentRepository, mq, config, logger, real_indexer: RealDocumentIndexer | None = None):
        self.repo = repo
        self.mq = mq
        self.config = config
        self.logger = logger
        self.real_indexer = real_indexer

    def handle(self, event: dict[str, Any], channel, method) -> None:
        document_id = event.get("document_id")
        event_id = event.get("event_id", "")
        trace_id = event.get("trace_id", "")

        if not document_id or event.get("schema_version") != 1:
            self.logger.warning(
                "invalid_message trace_id=%s event_id=%s document_id=%s",
                trace_id,
                event_id,
                document_id,
            )
            channel.basic_ack(delivery_tag=method.delivery_tag)
            return

        self.logger.info(
            "message_received trace_id=%s event_id=%s document_id=%s",
            trace_id,
            event_id,
            document_id,
        )

        started = time.perf_counter()
        process_attempts = 1
        try:
            doc = self.repo.get_document(document_id)
            if doc is None:
                self.logger.error(
                    "document_not_found trace_id=%s event_id=%s document_id=%s",
                    trace_id,
                    event_id,
                    document_id,
                )
                channel.basic_ack(delivery_tag=method.delivery_tag)
                return

            if doc.status in TERMINAL_STATUSES:
                self.logger.info(
                    "document_already_done trace_id=%s event_id=%s document_id=%s status=%s",
                    trace_id,
                    event_id,
                    document_id,
                    doc.status,
                )
                channel.basic_ack(delivery_tag=method.delivery_tag)
                return

            if doc.status == "indexing" and is_recent(doc.updated_at):
                self.logger.info(
                    "document_recently_indexing_delayed_retry trace_id=%s event_id=%s document_id=%s",
                    trace_id,
                    event_id,
                    document_id,
                )
                self._schedule_delayed_retry(event)
                channel.basic_ack(delivery_tag=method.delivery_tag)
                return

            job_id = self.repo.ensure_job(event, self.config.worker_id, self.config.queue)
            self.repo.mark_running(document_id, job_id, self.config.worker_id)
            if self.config.mock_index:
                if self.config.mock_sleep_seconds > 0:
                    time.sleep(self.config.mock_sleep_seconds)

                self.repo.mark_succeeded(
                    document_id,
                    job_id,
                    mock=True,
                    duration_ms=_duration_ms(started),
                    process_attempts=1,
                )
                self.logger.info(
                    "mock_index_succeeded trace_id=%s event_id=%s document_id=%s job_id=%s",
                    trace_id,
                    event_id,
                    document_id,
                    job_id,
                )
            else:
                if self.real_indexer is None:
                    raise RuntimeError("real indexer is not initialized while MOCK_INDEX=false")
                process_attempts = max(1, self.config.index_max_attempts)
                result, process_attempts = self._run_real_index_with_retry(doc)
                self.repo.mark_succeeded(
                    document_id,
                    job_id,
                    mock=False,
                    chunk_count=result.chunk_count,
                    milvus_collection=self.config.milvus_collection,
                    embedding_model=self.config.embedding_model,
                    embedding_dimension=self.config.embedding_dimension,
                    duration_ms=_duration_ms(started),
                    process_attempts=process_attempts,
                )
                self.logger.info(
                    "real_index_succeeded trace_id=%s event_id=%s document_id=%s job_id=%s chunk_count=%s attempts=%s",
                    trace_id,
                    event_id,
                    document_id,
                    job_id,
                    result.chunk_count,
                    process_attempts,
                )
            channel.basic_ack(delivery_tag=method.delivery_tag)
        except Exception as exc:
            self.logger.exception(
                "message_process_failed trace_id=%s event_id=%s document_id=%s error=%s",
                trace_id,
                event_id,
                document_id,
                exc,
            )
            try:
                job_id = event.get("job_id") or event.get("event_id", "")
                if document_id and job_id:
                    self.repo.mark_failed(
                        document_id,
                        job_id,
                        str(exc),
                        duration_ms=_duration_ms(started),
                        process_attempts=process_attempts,
                        milvus_collection=self.config.milvus_collection,
                        embedding_model=self.config.embedding_model,
                        embedding_dimension=self.config.embedding_dimension,
                    )
            finally:
                channel.basic_ack(delivery_tag=method.delivery_tag)

    def _run_real_index_with_retry(self, doc):
        last_exc: Exception | None = None
        max_attempts = max(1, self.config.index_max_attempts)
        for attempt in range(1, max_attempts + 1):
            try:
                return self.real_indexer.index_document(doc), attempt
            except Exception as exc:
                last_exc = exc
                self.logger.warning(
                    "real_index_attempt_failed document_id=%s attempt=%s max_attempts=%s error=%s",
                    doc.id,
                    attempt,
                    max_attempts,
                    exc,
                )
                if attempt < max_attempts:
                    delay_index = attempt - 1
                    delay = (
                        self.config.index_retry_delays[delay_index]
                        if delay_index < len(self.config.index_retry_delays)
                        else self.config.index_retry_delays[-1]
                        if self.config.index_retry_delays
                        else 0
                    )
                    if delay > 0:
                        time.sleep(delay)
        raise RuntimeError(f"real index failed after {max_attempts} attempts: {last_exc}")

    def _schedule_delayed_retry(self, event: dict[str, Any]) -> None:
        def retry_later() -> None:
            time.sleep(self.config.delayed_retry_seconds)
            document_id = event["document_id"]
            doc = self.repo.get_document(document_id)
            if doc is None or doc.status in TERMINAL_STATUSES:
                return
            from mq import DocumentMQ

            retry_mq = DocumentMQ(self.config)
            try:
                retry_mq.publish_event(event)
            finally:
                retry_mq.close()
            self.logger.info(
                "delayed_retry_republished trace_id=%s event_id=%s document_id=%s",
                event.get("trace_id", ""),
                event.get("event_id", ""),
                document_id,
            )

        thread = threading.Thread(target=retry_later, daemon=True)
        thread.start()


def _duration_ms(started: float) -> int:
    return int((time.perf_counter() - started) * 1000)
