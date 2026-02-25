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


settings = Settings()
