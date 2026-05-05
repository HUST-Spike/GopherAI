from __future__ import annotations

from typing import Iterable

from pymilvus import DataType, Function, FunctionType, MilvusClient

from config import WorkerConfig


class MilvusDocumentChunkStore:
    def __init__(self, config: WorkerConfig, logger):
        self.config = config
        self.logger = logger
        self.client = MilvusClient(uri=config.milvus_uri)
        self.ensure_collection()

    def ensure_collection(self) -> None:
        if self.client.has_collection(self.config.milvus_collection):
            self._validate_existing_collection()
            self.client.load_collection(self.config.milvus_collection)
            return

        schema = MilvusClient.create_schema(auto_id=False, enable_dynamic_field=False)
        schema.add_field("chunk_id", DataType.VARCHAR, is_primary=True, max_length=256)
        schema.add_field("document_id", DataType.VARCHAR, max_length=36)
        schema.add_field("user_name", DataType.VARCHAR, max_length=50)
        schema.add_field("session_id", DataType.VARCHAR, max_length=36)
        schema.add_field("original_filename", DataType.VARCHAR, max_length=512)
        schema.add_field("file_path", DataType.VARCHAR, max_length=1024)
        schema.add_field("storage_backend", DataType.VARCHAR, max_length=30)
        schema.add_field("source_type", DataType.VARCHAR, max_length=30)
        schema.add_field("index_version", DataType.INT64)
        schema.add_field("chunk_index", DataType.INT64)
        schema.add_field(
            "content",
            DataType.VARCHAR,
            max_length=8192,
            enable_analyzer=self.config.milvus_bm25_enabled,
            analyzer_params={"type": "chinese"} if self.config.milvus_bm25_enabled else None,
        )
        schema.add_field("content_sha256", DataType.VARCHAR, max_length=64)
        schema.add_field("embedding_model", DataType.VARCHAR, max_length=100)
        schema.add_field("embedding_dimension", DataType.INT64)
        schema.add_field("metadata_json", DataType.VARCHAR, max_length=4096)
        schema.add_field("created_at_unix", DataType.INT64)
        schema.add_field("embedding", DataType.FLOAT_VECTOR, dim=self.config.embedding_dimension)
        if self.config.milvus_bm25_enabled:
            schema.add_field(self.config.milvus_bm25_sparse_field, DataType.SPARSE_FLOAT_VECTOR)
            schema.add_function(
                Function(
                    name=self.config.milvus_bm25_function_name,
                    function_type=FunctionType.BM25,
                    input_field_names=["content"],
                    output_field_names=[self.config.milvus_bm25_sparse_field],
                )
            )

        index_params = MilvusClient.prepare_index_params()
        index_params.add_index(
            field_name="embedding",
            index_type="HNSW",
            metric_type="COSINE",
            params={"M": 16, "efConstruction": 200},
        )
        if self.config.milvus_bm25_enabled:
            index_params.add_index(
                field_name=self.config.milvus_bm25_sparse_field,
                index_type="SPARSE_INVERTED_INDEX",
                metric_type="BM25",
                params={"drop_ratio_build": self.config.milvus_bm25_drop_ratio_build},
            )

        self.client.create_collection(
            collection_name=self.config.milvus_collection,
            schema=schema,
            index_params=index_params,
        )
        self.client.load_collection(self.config.milvus_collection)
        self.logger.info(
            "milvus_collection_created collection=%s dimension=%s",
            self.config.milvus_collection,
            self.config.embedding_dimension,
        )

    def _validate_existing_collection(self) -> None:
        desc = self.client.describe_collection(self.config.milvus_collection)
        fields = desc.get("fields", [])
        embedding_field = next((field for field in fields if field.get("name") == "embedding"), None)
        if embedding_field is None:
            raise RuntimeError(f"Milvus collection {self.config.milvus_collection} missing embedding field")
        if self.config.milvus_bm25_enabled:
            sparse_field = next((field for field in fields if field.get("name") == self.config.milvus_bm25_sparse_field), None)
            if sparse_field is None:
                raise RuntimeError(
                    f"Milvus collection {self.config.milvus_collection} missing BM25 sparse field "
                    f"{self.config.milvus_bm25_sparse_field}. Recreate/reindex the collection or set MILVUS_BM25_ENABLED=false."
                )

        params = embedding_field.get("params") or {}
        dim = int(params.get("dim") or params.get("dimension") or 0)
        if dim and dim != self.config.embedding_dimension:
            raise RuntimeError(
                f"Milvus collection dimension mismatch: collection={dim}, config={self.config.embedding_dimension}"
            )

    def delete_document_chunks(self, document_id: str, index_version: int) -> None:
        expr = f'document_id == "{document_id}" and index_version == {index_version}'
        self.client.delete(collection_name=self.config.milvus_collection, filter=expr)

    def insert_chunks(self, rows: list[dict]) -> None:
        for batch in _batched(rows, self.config.milvus_insert_batch_size):
            self.client.insert(collection_name=self.config.milvus_collection, data=batch)


def _batched(items: list[dict], batch_size: int) -> Iterable[list[dict]]:
    if batch_size <= 0:
        batch_size = 64
    for start in range(0, len(items), batch_size):
        yield items[start : start + batch_size]
