"""
RAG TTL (Time-To-Live) и политика переиндексации.

Модуль отвечает за:
- Отслеживание возраста документов в коллекциях ChromaDB
- Автоматическое удаление устаревших документов (по TTL)
- Переиндексацию документов при смене модели эмбеддингов
- Фоновый планировщик для периодических проверок
"""

import logging
import time
import threading
from typing import Optional

from .config import settings

logger = logging.getLogger(__name__)

# TTL настройки (в днях)
DEFAULT_FACTS_TTL = int(settings.FACTS_TTL_DAYS) if hasattr(settings, 'FACTS_TTL_DAYS') else 90
DEFAULT_FILES_TTL = int(settings.FILES_TTL_DAYS) if hasattr(settings, 'FILES_TTL_DAYS') else 30
DEFAULT_LEARNINGS_TTL = int(settings.LEARNINGS_TTL_DAYS) if hasattr(settings, 'LEARNINGS_TTL_DAYS') else 0

# Интервал проверки (в секундах)
REINDEX_CHECK_INTERVAL = int(
    settings.REINDEX_CHECK_INTERVAL
    if hasattr(settings, 'REINDEX_CHECK_INTERVAL') else 3600
)


class TTLManager:
    """Управление TTL документов и переиндексацией."""

    def __init__(self, memory_store):
        self.store = memory_store
        self._scheduler_running = False
        self._scheduler_thread: Optional[threading.Thread] = None

    def get_expired_ids(self, collection_name: str, ttl_days: int):
        """Получить ID документов с истёкшим TTL."""
        if ttl_days <= 0:
            return []

        collection_map = {
            "facts": self.store.facts_collection,
            "files": self.store.files_collection,
            "learnings": self.store.learnings_collection,
        }
        collection = collection_map.get(collection_name)
        if not collection or collection.count() == 0:
            return []

        cutoff_ts = time.time() - (ttl_days * 86400)
        expired_ids = []

        try:
            all_data = collection.get(include=["metadatas"])
            if not all_data or "ids" not in all_data:
                return []

            for i, meta in enumerate(all_data.get("metadatas", [])):
                created_at = meta.get("created_at", 0)
                if isinstance(created_at, (int, float)) and created_at > 0:
                    if created_at < cutoff_ts:
                        expired_ids.append(all_data["ids"][i])
        except Exception as e:
            logger.error(f"Ошибка при проверке TTL для {collection_name}: {e}")

        return expired_ids

    def cleanup_expired(self, collection_name: str = "all") -> dict:
        """Удалить документы с истёкшим TTL."""
        ttl_map = {
            "facts": DEFAULT_FACTS_TTL,
            "files": DEFAULT_FILES_TTL,
            "learnings": DEFAULT_LEARNINGS_TTL,
        }

        collections = [collection_name] if collection_name != "all" else list(ttl_map.keys())
        result = {"total_deleted": 0, "by_collection": {}}

        for col_name in collections:
            ttl = ttl_map.get(col_name, 0)
            if ttl <= 0:
                continue

            expired = self.get_expired_ids(col_name, ttl)
            if not expired:
                result["by_collection"][col_name] = 0
                continue

            collection_map = {
                "facts": self.store.facts_collection,
                "files": self.store.files_collection,
                "learnings": self.store.learnings_collection,
            }
            try:
                collection_map[col_name].delete(ids=expired)
                result["by_collection"][col_name] = len(expired)
                result["total_deleted"] += len(expired)
                logger.info(f"TTL cleanup: удалено {len(expired)} из {col_name}")
            except Exception as e:
                logger.error(f"Ошибка удаления TTL для {col_name}: {e}")
                result["by_collection"][col_name] = 0

        return result

    def check_reindex_needed(self) -> dict:
        """Проверить, нужна ли переиндексация (смена модели эмбеддингов)."""
        status = {"needs_reindex": False, "collections": {}}

        for col_name, collection in [
            ("facts", self.store.facts_collection),
            ("files", self.store.files_collection),
            ("learnings", self.store.learnings_collection),
        ]:
            meta = collection.metadata or {}
            stored_model = meta.get("embedding_model", "")
            stored_version = meta.get("embedding_model_version", "")

            needs = (
                stored_model != "" and stored_model != settings.EMBEDDING_MODEL
            ) or (
                stored_version != "" and stored_version != settings.EMBEDDING_MODEL_VERSION
            )

            status["collections"][col_name] = {
                "stored_model": stored_model,
                "stored_version": stored_version,
                "current_model": settings.EMBEDDING_MODEL,
                "current_version": settings.EMBEDDING_MODEL_VERSION,
                "needs_reindex": needs,
            }
            if needs:
                status["needs_reindex"] = True

        return status

    def reindex_collection(self, collection_name: str, force: bool = False) -> int:
        """Переиндексировать коллекцию (пересчитать эмбеддинги)."""
        collection_map = {
            "facts": self.store.facts_collection,
            "files": self.store.files_collection,
            "learnings": self.store.learnings_collection,
        }
        collection = collection_map.get(collection_name)
        if not collection:
            return 0

        if not force:
            status = self.check_reindex_needed()
            col_status = status["collections"].get(collection_name, {})
            if not col_status.get("needs_reindex", False):
                logger.info(f"Переиндексация {collection_name} не требуется")
                return 0

        count = collection.count()
        if count == 0:
            return 0

        logger.info(f"Начинаем переиндексацию {collection_name} ({count} документов)")

        try:
            all_data = collection.get(include=["documents", "metadatas"])
            if not all_data or "documents" not in all_data:
                return 0

            docs = all_data["documents"]
            ids = all_data["ids"]
            metas = all_data.get("metadatas", [{}] * len(docs))

            new_embeddings = self.store.encoder.encode(docs).tolist()

            collection.update(
                ids=ids,
                embeddings=new_embeddings,
            )

            logger.info(f"Переиндексировано {len(docs)} документов в {collection_name}")
            return len(docs)
        except Exception as e:
            logger.error(f"Ошибка переиндексации {collection_name}: {e}")
            return 0

    def start_scheduler(self):
        """Запустить фоновый планировщик проверок TTL."""
        if self._scheduler_running:
            return

        self._scheduler_running = True
        self._scheduler_thread = threading.Thread(
            target=self._scheduler_loop, daemon=True
        )
        self._scheduler_thread.start()
        logger.info(f"TTL scheduler запущен (интервал: {REINDEX_CHECK_INTERVAL}с)")

    def stop_scheduler(self):
        """Остановить планировщик."""
        self._scheduler_running = False

    def _scheduler_loop(self):
        """Цикл планировщика."""
        while self._scheduler_running:
            try:
                result = self.cleanup_expired()
                if result["total_deleted"] > 0:
                    logger.info(f"TTL scheduler: удалено {result['total_deleted']} документов")

                reindex_status = self.check_reindex_needed()
                if reindex_status["needs_reindex"]:
                    logger.warning("Обнаружена необходимость переиндексации!")
            except Exception as e:
                logger.error(f"Ошибка TTL scheduler: {e}")

            time.sleep(REINDEX_CHECK_INTERVAL)
