from __future__ import annotations

import hashlib
import json
import os
import time
from dataclasses import dataclass

from llama_index.core import Document
from llama_index.core.node_parser import SentenceSplitter
from llama_index.embeddings.openai import OpenAIEmbedding

from config import WorkerConfig
from milvus_store import MilvusDocumentChunkStore
from repository import DocumentRecord


MAX_CONTENT_LENGTH = 8192
MAX_METADATA_JSON_LENGTH = 4096


@dataclass(frozen=True)
class IndexResult:
    chunk_count: int


class RealDocumentIndexer:
    def __init__(self, config: WorkerConfig, store: MilvusDocumentChunkStore, logger):
        self.config = config
        self.store = store
        self.logger = logger
        self.splitter = SentenceSplitter(
            chunk_size=config.chunk_size,
            chunk_overlap=config.chunk_overlap,
        )
        self.embed_model = OpenAIEmbedding(
            model_name=config.embedding_model,
            api_key=config.embedding_api_key,
            api_base=config.embedding_base_url,
            dimensions=config.embedding_dimension,
            embed_batch_size=config.embedding_batch_size,
        )

    def index_document(self, doc: DocumentRecord) -> IndexResult:
        if not doc.session_id:
            raise ValueError("document session_id is required for Milvus indexing")
        if not self.config.embedding_api_key:
            raise ValueError("EMBEDDING_API_KEY is required when MOCK_INDEX=false")

        content = self._read_text_file(doc.file_path)
        nodes = self.splitter.get_nodes_from_documents(
            [
                Document(
                    text=content,
                    metadata={
                        "document_id": doc.id,
                        "user_name": doc.user_name,
                        "session_id": doc.session_id,
                        "source": doc.file_path,
                    },
                )
            ]
        )
        chunks = [node.get_content(metadata_mode="none") for node in nodes]
        chunks = [chunk for chunk in chunks if chunk.strip()]
        if not chunks:
            raise ValueError("no chunks generated")

        for idx, chunk in enumerate(chunks):
            if len(chunk) > MAX_CONTENT_LENGTH:
                raise ValueError(f"chunk content too long: chunk_index={idx}, length={len(chunk)}")

        embeddings = self._embed_chunks(chunks)
        rows = [
            self._build_row(doc=doc, chunk=chunk, embedding=embedding, chunk_index=idx)
            for idx, (chunk, embedding) in enumerate(zip(chunks, embeddings))
        ]

        self.store.delete_document_chunks(doc.id, doc.index_version)
        self.store.insert_chunks(rows)
        self.logger.info(
            "milvus_chunks_inserted document_id=%s session_id=%s chunk_count=%s collection=%s",
            doc.id,
            doc.session_id,
            len(rows),
            self.config.milvus_collection,
        )
        return IndexResult(chunk_count=len(rows))

    def _read_text_file(self, file_path: str) -> str:
        resolved_path = file_path
        if not os.path.isabs(resolved_path):
            resolved_path = os.path.join(self.config.project_root, file_path)
        with open(resolved_path, "r", encoding="utf-8") as file:
            return file.read()

    def _embed_chunks(self, chunks: list[str]) -> list[list[float]]:
        embeddings: list[list[float]] = []
        batch_size = max(1, self.config.embedding_batch_size)
        for start in range(0, len(chunks), batch_size):
            batch = chunks[start : start + batch_size]
            embeddings.extend(self.embed_model.get_text_embedding_batch(batch))
        if len(embeddings) != len(chunks):
            raise RuntimeError(f"embedding count mismatch: embeddings={len(embeddings)}, chunks={len(chunks)}")
        for idx, embedding in enumerate(embeddings):
            if len(embedding) != self.config.embedding_dimension:
                raise RuntimeError(
                    f"embedding dimension mismatch: chunk_index={idx}, actual={len(embedding)}, expected={self.config.embedding_dimension}"
                )
        return embeddings

    def _build_row(self, doc: DocumentRecord, chunk: str, embedding: list[float], chunk_index: int) -> dict:
        chunk_sha256 = _sha256_text(chunk)
        metadata = {
            "source_type": "local_file",
            "storage_backend": doc.storage_backend,
            "file_ext": doc.file_ext,
            "mime_type": doc.mime_type,
            "document_sha256": doc.sha256,
            "chunk_sha256": chunk_sha256,
            "chunk_size": self.config.chunk_size,
            "chunk_overlap": self.config.chunk_overlap,
            "original_filename": doc.original_filename,
            "file_path": doc.file_path,
        }
        metadata_json = json.dumps(metadata, ensure_ascii=False)
        if len(metadata_json) > MAX_METADATA_JSON_LENGTH:
            raise ValueError(f"metadata_json too long: chunk_index={chunk_index}, length={len(metadata_json)}")

        return {
            "chunk_id": f"{doc.id}:v{doc.index_version}:chunk_{chunk_index:06d}",
            "document_id": doc.id,
            "user_name": doc.user_name,
            "session_id": doc.session_id,
            "original_filename": doc.original_filename,
            "file_path": doc.file_path,
            "storage_backend": doc.storage_backend,
            "source_type": "local_file",
            "index_version": int(doc.index_version),
            "chunk_index": int(chunk_index),
            "content": chunk,
            "content_sha256": chunk_sha256,
            "embedding_model": self.config.embedding_model,
            "embedding_dimension": int(self.config.embedding_dimension),
            "metadata_json": metadata_json,
            "created_at_unix": int(time.time()),
            "embedding": embedding,
        }


def _sha256_text(value: str) -> str:
    return hashlib.sha256(value.encode("utf-8")).hexdigest()
