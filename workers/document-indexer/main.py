from config import load_config
from indexer import RealDocumentIndexer
from logging_setup import setup_logging
from milvus_store import MilvusDocumentChunkStore
from mq import DocumentMQ
from processor import DocumentProcessor
from repository import DocumentRepository


def main() -> None:
    logger = setup_logging()
    config = load_config()

    logger.info(
        "worker_start worker_id=%s queue=%s exchange=%s routing_key=%s mock_index=%s",
        config.worker_id,
        config.queue,
        config.exchange,
        config.routing_key,
        config.mock_index,
    )

    repo = DocumentRepository(config.mysql_dsn, config.mysql_session_time_zone)
    mq = DocumentMQ(config)
    real_indexer = None
    if not config.mock_index:
        store = MilvusDocumentChunkStore(config, logger)
        real_indexer = RealDocumentIndexer(config, store, logger)

    processor = DocumentProcessor(repo, mq, config, logger, real_indexer=real_indexer)

    logger.info("worker_consuming worker_id=%s queue=%s", config.worker_id, config.queue)
    mq.consume(processor.handle)


if __name__ == "__main__":
    main()
