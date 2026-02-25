import os
from pathlib import Path

from .vector_backend import resolve_vector_backend


class Settings:
    """Настройки сервиса памяти."""

    # Базовая директория проекта
    BASE_DIR = Path(__file__).parent.parent

    # Директория для временных файлов (при обработке)
    TEMP_DIR = os.getenv("TEMP_DIR", str(BASE_DIR / "data" / "temp"))

    # Конфигурация Qdrant backend
    QDRANT_URL = os.getenv("QDRANT_URL", "")
    QDRANT_PATH = os.getenv("QDRANT_PATH", str(BASE_DIR / "data" / "qdrant"))

    # Модель для эмбеддингов
    EMBEDDING_MODEL = os.getenv("EMBEDDING_MODEL", "all-MiniLM-L6-v2")
    EMBEDDING_MODEL_VERSION = os.getenv("EMBEDDING_MODEL_VERSION", "1")

    # Backend векторного хранилища: в текущей реализации поддерживается Qdrant.
    VECTOR_BACKEND = resolve_vector_backend(os.getenv("VECTOR_BACKEND", "qdrant"))

    # Размер чанков при разбиении текста
    CHUNK_SIZE = int(os.getenv("CHUNK_SIZE", "500"))

    # Перекрытие чанков
    CHUNK_OVERLAP = int(os.getenv("CHUNK_OVERLAP", "50"))

    # Количество результатов при поиске
    TOP_K = int(os.getenv("TOP_K", "5"))

    # Веса гибридной релевантности (semantic + keyword) для retrieval.
    SEARCH_SEMANTIC_WEIGHT = float(os.getenv("SEARCH_SEMANTIC_WEIGHT", "0.8"))
    SEARCH_KEYWORD_WEIGHT = float(os.getenv("SEARCH_KEYWORD_WEIGHT", "0.2"))

    # Коэффициент влияния приоритета памяти (critical/pinned/reinforced/normal/archived).
    RANK_WEIGHT_PRIORITY = float(os.getenv("RANK_WEIGHT_PRIORITY", "0.10"))

    # === Весовые коэффициенты ранжирования памяти ===
    RANK_WEIGHT_RELEVANCE = float(os.getenv("RANK_WEIGHT_RELEVANCE", "0.55"))
    RANK_WEIGHT_IMPORTANCE = float(os.getenv("RANK_WEIGHT_IMPORTANCE", "0.15"))
    RANK_WEIGHT_RELIABILITY = float(os.getenv("RANK_WEIGHT_RELIABILITY", "0.15"))
    RANK_WEIGHT_RECENCY = float(os.getenv("RANK_WEIGHT_RECENCY", "0.10"))
    RANK_WEIGHT_FREQUENCY = float(os.getenv("RANK_WEIGHT_FREQUENCY", "0.05"))

    # Горизонт «свежести» (в днях)
    RECENCY_WINDOW_DAYS = int(os.getenv("RECENCY_WINDOW_DAYS", "30"))

    # === Backup checks / readiness flags ===
    QDRANT_SNAPSHOT_ENABLED = os.getenv("QDRANT_SNAPSHOT_ENABLED", "false").lower() == "true"
    NEO4J_BACKUP_ENABLED = os.getenv("NEO4J_BACKUP_ENABLED", "false").lower() == "true"
    MINIO_VERSIONING_ENABLED = os.getenv("MINIO_VERSIONING_ENABLED", "false").lower() == "true"
    RESTORE_TEST_ENABLED = os.getenv("RESTORE_TEST_ENABLED", "false").lower() == "true"

    # Хост и порт для FastAPI
    HOST = os.getenv("HOST", "0.0.0.0")
    PORT = int(os.getenv("PORT", "8001"))

    # Режим отладки
    DEBUG = os.getenv("DEBUG", "False").lower() == "true"

    # TTL для документов (в днях, 0 = без ограничения)
    FACTS_TTL_DAYS = int(os.getenv("FACTS_TTL_DAYS", "90"))
    FILES_TTL_DAYS = int(os.getenv("FILES_TTL_DAYS", "30"))
    LEARNINGS_TTL_DAYS = int(os.getenv("LEARNINGS_TTL_DAYS", "0"))

    # Интервал проверки TTL/переиндексации (в секундах)
    REINDEX_CHECK_INTERVAL = int(os.getenv("REINDEX_CHECK_INTERVAL", "3600"))

    # === Детекция противоречий (Eternal RAG: раздел 8) ===
    # Порог косинусной близости для поиска потенциальных противоречий.
    # При добавлении нового знания ищутся семантически похожие записи;
    # если similarity >= порога, а текст отличается, фиксируется противоречие.
    CONTRADICTION_SIMILARITY_THRESHOLD = float(os.getenv("CONTRADICTION_SIMILARITY_THRESHOLD", "0.85"))
    # Максимум кандидатов для проверки на противоречие
    CONTRADICTION_TOP_K = int(os.getenv("CONTRADICTION_TOP_K", "3"))

    # === Skill Engine (Eternal RAG: раздел 5.3) ===
    # Confidence по умолчанию при создании нового навыка (0.0-1.0).
    SKILL_CONFIDENCE_DEFAULT = float(os.getenv("SKILL_CONFIDENCE_DEFAULT", "0.5"))
    # Минимальный порог confidence для автоматического применения навыка.
    SKILL_CONFIDENCE_MIN = float(os.getenv("SKILL_CONFIDENCE_MIN", "0.3"))
    # Максимум результатов при поиске навыков.
    SKILL_SEARCH_TOP_K = int(os.getenv("SKILL_SEARCH_TOP_K", "5"))
    # Имя Qdrant-коллекции для навыков.
    SKILL_COLLECTION_NAME = os.getenv("SKILL_COLLECTION_NAME", "agent_skills")

    # === Graph Engine (Eternal RAG: раздел 5.4) ===
    # Максимальная глубина обхода графа связей.
    GRAPH_MAX_DEPTH = int(os.getenv("GRAPH_MAX_DEPTH", "3"))
    # Максимум соседей, возвращаемых за один запрос.
    GRAPH_MAX_NEIGHBORS = int(os.getenv("GRAPH_MAX_NEIGHBORS", "20"))
    # Имя Qdrant-коллекции для связей графа знаний.
    GRAPH_COLLECTION_NAME = os.getenv("GRAPH_COLLECTION_NAME", "agent_relationships")
    # Допустимые типы связей между узлами графа знаний.
    GRAPH_RELATIONSHIP_TYPES = os.getenv(
        "GRAPH_RELATIONSHIP_TYPES",
        "relates_to,contradicts,depends_on,supersedes,derived_from"
    ).split(",")

    # === Neo4j (будущая интеграция, Eternal RAG: раздел 5.4) ===
    NEO4J_URL = os.getenv("NEO4J_URL", "bolt://localhost:7687")
    NEO4J_AUTH = os.getenv("NEO4J_AUTH", "neo4j/agentcore2024")


settings = Settings()
