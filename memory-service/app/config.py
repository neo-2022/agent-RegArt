import os
from pathlib import Path

class Settings:
    """Настройки сервиса памяти."""
    
    # Базовая директория проекта
    BASE_DIR = Path(__file__).parent.parent
    
    # Директория для хранения данных ChromaDB
    CHROMA_DIR = os.getenv("CHROMA_DIR", str(BASE_DIR / "data" / "chroma"))
    
    # Директория для временных файлов (при обработке)
    TEMP_DIR = os.getenv("TEMP_DIR", str(BASE_DIR / "data" / "temp"))
    
    # Модель для эмбеддингов
    EMBEDDING_MODEL = os.getenv("EMBEDDING_MODEL", "all-MiniLM-L6-v2")
    EMBEDDING_MODEL_VERSION = os.getenv("EMBEDDING_MODEL_VERSION", "1")
    
    # Размер чанков при разбиении текста
    CHUNK_SIZE = int(os.getenv("CHUNK_SIZE", "500"))
    
    # Перекрытие чанков
    CHUNK_OVERLAP = int(os.getenv("CHUNK_OVERLAP", "50"))
    
    # Количество результатов при поиске
    TOP_K = int(os.getenv("TOP_K", "5"))
    
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
