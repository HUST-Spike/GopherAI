from __future__ import annotations

from dataclasses import dataclass
from typing import Any


SUPPORTED_CHUNK_STRATEGIES = {
    "sentence_splitter",
    "markdown_sentence_splitter",
    "semantic_splitter",
}


@dataclass(frozen=True)
class ChunkingConfig:
    strategy: str = "sentence_splitter"
    chunk_size: int = 1000
    chunk_overlap: int = 150
    markdown_max_section_chars: int = 1600
    semantic_breakpoint_percentile_threshold: int = 95
    semantic_buffer_size: int = 1
    semantic_fallback_chunk_size: int = 1000
    semantic_fallback_chunk_overlap: int = 150


def split_text_to_nodes(
    *,
    content: str,
    metadata: dict[str, Any],
    config: ChunkingConfig,
    embed_model: Any | None = None,
) -> list[Any]:
    strategy = config.strategy
    if strategy not in SUPPORTED_CHUNK_STRATEGIES:
        raise ValueError(f"Unsupported chunk strategy: {strategy}")

    if strategy == "sentence_splitter":
        return _sentence_nodes(content=content, metadata=metadata, chunk_size=config.chunk_size, chunk_overlap=config.chunk_overlap)
    if strategy == "markdown_sentence_splitter":
        return _markdown_sentence_nodes(content=content, metadata=metadata, config=config)
    if strategy == "semantic_splitter":
        if embed_model is None:
            raise ValueError("semantic_splitter requires an embedding model")
        nodes = _semantic_nodes(content=content, metadata=metadata, config=config, embed_model=embed_model)
        return _split_oversized_nodes(
            nodes=nodes,
            chunk_size=config.semantic_fallback_chunk_size,
            chunk_overlap=config.semantic_fallback_chunk_overlap,
        )
    raise ValueError(f"Unsupported chunk strategy: {strategy}")


def node_content(node: Any) -> str:
    return str(node.get_content(metadata_mode="none")).strip()


def nodes_to_chunks(nodes: list[Any]) -> list[str]:
    return [chunk for node in nodes if (chunk := node_content(node))]


def _sentence_nodes(*, content: str, metadata: dict[str, Any], chunk_size: int, chunk_overlap: int) -> list[Any]:
    from llama_index.core import Document
    from llama_index.core.node_parser import SentenceSplitter

    splitter = SentenceSplitter(chunk_size=chunk_size, chunk_overlap=chunk_overlap)
    return splitter.get_nodes_from_documents([Document(text=content, metadata=metadata)])


def _markdown_sentence_nodes(*, content: str, metadata: dict[str, Any], config: ChunkingConfig) -> list[Any]:
    from llama_index.core import Document
    from llama_index.core.node_parser import SentenceSplitter
    from llama_index.core.node_parser.file.markdown import MarkdownNodeParser

    markdown_parser = MarkdownNodeParser()
    section_nodes = markdown_parser.get_nodes_from_documents([Document(text=content, metadata=metadata)])
    sentence_splitter = SentenceSplitter(chunk_size=config.chunk_size, chunk_overlap=config.chunk_overlap)

    final_nodes: list[Any] = []
    for node in section_nodes:
        chunk = node_content(node)
        if not chunk:
            continue
        if len(chunk) <= config.markdown_max_section_chars:
            final_nodes.append(node)
            continue
        final_nodes.extend(sentence_splitter.get_nodes_from_documents([Document(text=chunk, metadata=dict(node.metadata or {}))]))
    return final_nodes


def _semantic_nodes(*, content: str, metadata: dict[str, Any], config: ChunkingConfig, embed_model: Any) -> list[Any]:
    from llama_index.core import Document
    from llama_index.core.node_parser import SemanticSplitterNodeParser

    splitter = SemanticSplitterNodeParser.from_defaults(
        embed_model=embed_model,
        breakpoint_percentile_threshold=config.semantic_breakpoint_percentile_threshold,
        buffer_size=config.semantic_buffer_size,
        include_metadata=True,
        include_prev_next_rel=True,
    )
    return splitter.get_nodes_from_documents([Document(text=content, metadata=metadata)])


def _split_oversized_nodes(*, nodes: list[Any], chunk_size: int, chunk_overlap: int) -> list[Any]:
    from llama_index.core import Document
    from llama_index.core.node_parser import SentenceSplitter

    splitter = SentenceSplitter(chunk_size=chunk_size, chunk_overlap=chunk_overlap)
    final_nodes: list[Any] = []
    for node in nodes:
        chunk = node_content(node)
        if not chunk:
            continue
        if len(chunk) <= chunk_size:
            final_nodes.append(node)
            continue
        final_nodes.extend(splitter.get_nodes_from_documents([Document(text=chunk, metadata=dict(node.metadata or {}))]))
    return final_nodes
