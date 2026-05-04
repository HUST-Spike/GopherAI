import logging
import os
import sys


def setup_logging() -> logging.Logger:
    base_dir = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
    log_dir = os.path.join(base_dir, "logs", "python")
    os.makedirs(log_dir, exist_ok=True)
    log_path = os.path.join(log_dir, "document-indexer.log")

    logger = logging.getLogger("document-indexer")
    logger.setLevel(logging.INFO)
    logger.handlers.clear()

    formatter = logging.Formatter(
        "%(asctime)s level=%(levelname)s service=document-indexer %(message)s"
    )

    stream_handler = logging.StreamHandler(sys.stdout)
    stream_handler.setFormatter(formatter)
    logger.addHandler(stream_handler)

    file_handler = logging.FileHandler(log_path, encoding="utf-8")
    file_handler.setFormatter(formatter)
    logger.addHandler(file_handler)

    return logger
