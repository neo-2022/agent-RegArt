import os
import uuid
import logging
from typing import List, Dict, Optional, Any

import chromadb
from chromadb.config import Settings as ChromaSettings
from chromadb.errors import NotFoundError
from sentence_transformers import SentenceTransformer

from .config import settings

# Настройка логирования
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)


class MemoryStore:
    """
    Класс для работы с долговременной памятью (RAG).
    Использует ChromaDB для векторного хранения и sentence-transformers для эмбеддингов.
    """
    
    def __init__(self):
        """Инициализация клиента ChromaDB и модели эмбеддингов."""
        # Создаём директорию для данных, если её нет
        os.makedirs(settings.CHROMA_DIR, exist_ok=True)
        os.makedirs(settings.TEMP_DIR, exist_ok=True)
        
        # Инициализация ChromaDB клиента
        self.client = chromadb.PersistentClient(
            path=settings.CHROMA_DIR,
            settings=ChromaSettings(anonymized_telemetry=False)
        )
        
        # Создаём или получаем коллекции
        self.facts_collection = self._get_or_create_collection("agent_memory_facts")
        self.files_collection = self._get_or_create_collection("agent_memory_files")
        # Коллекция для обучения агентов — хранит знания, извлечённые из диалогов.
        # Каждое знание привязано к конкретной модели LLM через метаданные (model_name).
        # Это позволяет каждой модели накапливать свою уникальную базу знаний.
        self.learnings_collection = self._get_or_create_collection("agent_learnings")
        
        # Загружаем модель эмбеддингов
        logger.info(f"Загрузка модели эмбеддингов: {settings.EMBEDDING_MODEL}")
        self.encoder = SentenceTransformer(settings.EMBEDDING_MODEL)
        logger.info("Модель эмбеддингов загружена")
    
    def _get_or_create_collection(self, name: str):
        """Вспомогательный метод для получения или создания коллекции."""
        collection_meta = {
            "hnsw:space": "cosine",
            "embedding_model": settings.EMBEDDING_MODEL,
            "embedding_model_version": settings.EMBEDDING_MODEL_VERSION,
        }
        try:
            col = self.client.get_collection(name)
            existing_model = (col.metadata or {}).get("embedding_model", "")
            if existing_model and existing_model != settings.EMBEDDING_MODEL:
                logger.warning(
                    f"Коллекция {name}: модель эмбеддингов изменилась "
                    f"({existing_model} -> {settings.EMBEDDING_MODEL}). "
                    f"Векторы могут быть несовместимы. Рекомендуется переиндексация."
                )
            return col
        except (ValueError, NotFoundError):
            logger.info(f"Создание новой коллекции: {name}")
            return self.client.create_collection(
                name=name,
                metadata=collection_meta
            )
    
    def add_fact(self, fact_text: str, metadata: Optional[Dict[str, Any]] = None) -> str:
        """
        Добавление факта в память.
        
        Args:
            fact_text: Текст факта
            metadata: Метаданные (например, {"agent": "admin", "source": "user"})
        
        Returns:
            ID добавленного факта
        """
        if not fact_text or not fact_text.strip():
            logger.warning("Попытка добавить пустой факт")
            return ""
        
        if len(fact_text) > 10 * 1024 * 1024:
            logger.warning(f"Факт превышает лимит 10 МБ: {len(fact_text)} байт")
            return ""
        
        fact_text = fact_text.replace("\x00", "")
        
        fact_id = str(uuid.uuid4())
        embedding = self.encoder.encode(fact_text).tolist()
        
        self.facts_collection.add(
            embeddings=[embedding],
            documents=[fact_text],
            metadatas=[metadata or {}],
            ids=[fact_id]
        )
        
        logger.info(f"Добавлен факт (ID: {fact_id}): {fact_text[:50]}...")
        return fact_id
    
    def search_facts(self, query: str, top_k: int = None, agent_name: Optional[str] = None, include_files: bool = False) -> List[Dict[str, Any]]:
        """
        Поиск релевантных фактов и/или фрагментов файлов.
        Возвращает структурированные результаты: text, score, source, metadata.
        """
        if top_k is None:
            top_k = settings.TOP_K
        
        if self.facts_collection.count() == 0 and (not include_files or self.files_collection.count() == 0):
            return []
        
        query_embedding = self.encoder.encode(query).tolist()
        results: List[Dict[str, Any]] = []
        
        if self.facts_collection.count() > 0:
            facts_res = self.facts_collection.query(
                query_embeddings=[query_embedding],
                n_results=top_k,
                include=["documents", "distances", "metadatas"],
                where={"agent": agent_name} if agent_name else None
            )
            if facts_res and 'documents' in facts_res and facts_res['documents']:
                docs = facts_res['documents'][0]
                dists = facts_res.get('distances', [[]])[0]
                metas = facts_res.get('metadatas', [[]])[0]
                for i, doc in enumerate(docs):
                    dist = dists[i] if i < len(dists) else 1.0
                    score = max(0.0, 1.0 - dist)
                    meta = metas[i] if i < len(metas) else {}
                    results.append({"text": doc, "score": round(score, 4), "source": "facts", "metadata": meta})
        
        if include_files and self.files_collection.count() > 0:
            files_res = self.files_collection.query(
                query_embeddings=[query_embedding],
                n_results=top_k,
                include=["documents", "distances", "metadatas"],
                where={"agent": agent_name} if agent_name else None
            )
            if files_res and 'documents' in files_res and files_res['documents']:
                docs = files_res['documents'][0]
                dists = files_res.get('distances', [[]])[0]
                metas = files_res.get('metadatas', [[]])[0]
                for i, doc in enumerate(docs):
                    dist = dists[i] if i < len(dists) else 1.0
                    score = max(0.0, 1.0 - dist)
                    meta = metas[i] if i < len(metas) else {}
                    results.append({"text": doc, "score": round(score, 4), "source": "files", "metadata": meta})
        
        seen = set()
        unique: List[Dict[str, Any]] = []
        for r in results:
            if r["text"] not in seen:
                seen.add(r["text"])
                unique.append(r)
        
        unique.sort(key=lambda x: x["score"], reverse=True)
        return unique
    
    def add_file_chunk(self, chunk_text: str, metadata: Dict[str, Any]) -> str:
        """
        Добавление фрагмента файла в память.
        
        Args:
            chunk_text: Текст фрагмента
            metadata: Метаданные (обязательно содержат agent, filename, file_id, chunk)
        
        Returns:
            ID добавленного фрагмента
        """
        if not chunk_text or not chunk_text.strip():
            return ""
        
        chunk_id = str(uuid.uuid4())
        embedding = self.encoder.encode(chunk_text).tolist()
        
        self.files_collection.add(
            embeddings=[embedding],
            documents=[chunk_text],
            metadatas=[metadata],
            ids=[chunk_id]
        )
        
        return chunk_id
    
    def list_files(self) -> List[Dict[str, Any]]:
        """
        Получение списка всех загруженных файлов с количеством чанков.
        
        Returns:
            Список словарей с информацией о файлах: file_name, chunks_count
        """
        if self.files_collection.count() == 0:
            return []
        
        try:
            all_data = self.files_collection.get(include=["metadatas"])
            if not all_data or 'metadatas' not in all_data:
                return []
            
            files_map: Dict[str, int] = {}
            for meta in all_data['metadatas']:
                fname = meta.get('file_name', meta.get('filename', 'unknown'))
                files_map[fname] = files_map.get(fname, 0) + 1
            
            return [{"file_name": name, "chunks_count": count} for name, count in files_map.items()]
        except Exception as e:
            logger.error(f"Ошибка получения списка файлов: {e}")
            return []

    def delete_file_by_name(self, file_name: str) -> int:
        """
        Удаление всех чанков файла по имени.
        
        Args:
            file_name: Имя файла
        
        Returns:
            Количество удалённых чанков
        """
        try:
            results = self.files_collection.get(
                where={"file_name": file_name}
            )
            if not results or 'ids' not in results or not results['ids']:
                results = self.files_collection.get(
                    where={"filename": file_name}
                )
            if not results or 'ids' not in results:
                return 0
            
            ids_to_delete = results['ids']
            if ids_to_delete:
                self.files_collection.delete(ids=ids_to_delete)
                logger.info(f"Удалено {len(ids_to_delete)} чанков файла {file_name}")
                return len(ids_to_delete)
            return 0
        except Exception as e:
            logger.error(f"Ошибка удаления файла {file_name}: {e}")
            return 0

    def delete_file_chunks(self, file_id: str) -> int:
        """
        Удаление всех фрагментов, принадлежащих указанному файлу.
        
        Args:
            file_id: Идентификатор файла
        
        Returns:
            Количество удалённых фрагментов
        """
        try:
            results = self.files_collection.get(where={"file_id": file_id})
            if not results or 'ids' not in results:
                return 0
            
            ids_to_delete = results['ids']
            if ids_to_delete:
                self.files_collection.delete(ids=ids_to_delete)
                logger.info(f"Удалено {len(ids_to_delete)} чанков для файла {file_id}")
                return len(ids_to_delete)
            return 0
        except Exception as e:
            logger.error(f"Ошибка при удалении чанков файла {file_id}: {e}")
            return 0
    
    def add_learning(self, text: str, model_name: str, agent_name: str,
                     category: str = "general", metadata: Optional[Dict[str, Any]] = None) -> str:
        """
        Добавление знания (обучающего факта) для конкретной модели LLM.
        
        Знания извлекаются автоматически из каждого успешного диалога:
        - предпочтения пользователя (язык, стиль, формат ответов)
        - факты о системе и окружении (ОС, ПО, конфигурации)
        - исправления и уточнения (если пользователь поправил агента)
        - навыки и паттерны (успешные подходы к решению задач)
        
        Args:
            text: Текст знания (факт, правило, предпочтение)
            model_name: Имя модели LLM (например, 'llama3.1:8b', 'gpt-4o')
            agent_name: Имя агента (admin)
            category: Категория знания (general, preference, fact, skill, correction)
            metadata: Дополнительные метаданные
        
        Returns:
            ID добавленного знания
        """
        if not text or not text.strip():
            logger.warning("Попытка добавить пустое знание")
            return ""
        
        if len(text) > 10 * 1024 * 1024:
            logger.warning(f"Знание превышает лимит 10 МБ: {len(text)} байт")
            return ""
        
        text = text.replace("\x00", "")
        
        allowed_categories = {"general", "preference", "fact", "skill", "correction"}
        if category not in allowed_categories:
            logger.warning(f"Недопустимая категория '{category}', используется 'general'")
            category = "general"
        
        learning_id = str(uuid.uuid4())
        embedding = self.encoder.encode(text).tolist()
        
        # Метаданные знания — привязка к модели, агенту и категории
        learning_metadata = {
            "model_name": model_name,
            "agent_name": agent_name,
            "category": category,
        }
        if metadata:
            learning_metadata.update(metadata)
        
        self.learnings_collection.add(
            embeddings=[embedding],
            documents=[text],
            metadatas=[learning_metadata],
            ids=[learning_id]
        )
        
        logger.info(f"Добавлено знание для модели {model_name} (категория: {category}): {text[:80]}...")
        return learning_id
    
    def search_learnings(self, query: str, model_name: str,
                         top_k: int = 5, category: Optional[str] = None) -> List[Dict[str, Any]]:
        """
        Поиск релевантных знаний для конкретной модели LLM.
        Возвращает структурированные результаты: text, score, source, metadata.
        """
        if self.learnings_collection.count() == 0:
            return []
        
        query_embedding = self.encoder.encode(query).tolist()
        
        where_filter = {"model_name": model_name}
        if category:
            where_filter = {
                "$and": [
                    {"model_name": model_name},
                    {"category": category}
                ]
            }
        
        try:
            results = self.learnings_collection.query(
                query_embeddings=[query_embedding],
                n_results=top_k,
                include=["documents", "distances", "metadatas"],
                where=where_filter
            )
            if results and 'documents' in results and results['documents']:
                docs = results['documents'][0]
                dists = results.get('distances', [[]])[0]
                metas = results.get('metadatas', [[]])[0]
                items: List[Dict[str, Any]] = []
                for i, doc in enumerate(docs):
                    dist = dists[i] if i < len(dists) else 1.0
                    score = max(0.0, 1.0 - dist)
                    meta = metas[i] if i < len(metas) else {}
                    items.append({"text": doc, "score": round(score, 4), "source": "learnings", "metadata": meta})
                items.sort(key=lambda x: x["score"], reverse=True)
                return items
        except Exception as e:
            logger.error(f"Ошибка поиска знаний для модели {model_name}: {e}")
        
        return []
    
    def get_learning_stats(self) -> Dict[str, Any]:
        """
        Получение статистики обучения по моделям и категориям.
        
        Возвращает общее количество знаний, разбивку по моделям
        и разбивку по категориям. Используется для мониторинга
        процесса обучения через UI или API.
        
        Returns:
            Словарь со статистикой: total_learnings, by_model, by_category
        """
        total = self.learnings_collection.count()
        by_model: Dict[str, int] = {}
        by_category: Dict[str, int] = {}
        
        if total > 0:
            try:
                # Получаем все метаданные для подсчёта статистики
                all_data = self.learnings_collection.get(include=["metadatas"])
                if all_data and 'metadatas' in all_data:
                    for meta in all_data['metadatas']:
                        model = meta.get('model_name', 'unknown')
                        cat = meta.get('category', 'general')
                        by_model[model] = by_model.get(model, 0) + 1
                        by_category[cat] = by_category.get(cat, 0) + 1
            except Exception as e:
                logger.error(f"Ошибка получения статистики обучения: {e}")
        
        return {
            "total_learnings": total,
            "by_model": by_model,
            "by_category": by_category
        }
    
    def delete_model_learnings(self, model_name: str, category: Optional[str] = None) -> int:
        """
        Удаление знаний конкретной модели (полностью или по категории).
        
        Используется для сброса обучения модели — например, при смене
        модели на агенте или при необходимости начать обучение заново.
        
        Args:
            model_name: Имя модели, знания которой удалить
            category: Если указана — удаляются только знания этой категории
        
        Returns:
            Количество удалённых знаний
        """
        try:
            where_filter = {"model_name": model_name}
            if category:
                where_filter = {
                    "$and": [
                        {"model_name": model_name},
                        {"category": category}
                    ]
                }
            
            results = self.learnings_collection.get(where=where_filter)
            if not results or 'ids' not in results:
                return 0
            
            ids_to_delete = results['ids']
            if ids_to_delete:
                self.learnings_collection.delete(ids=ids_to_delete)
                logger.info(f"Удалено {len(ids_to_delete)} знаний модели {model_name}")
                return len(ids_to_delete)
            return 0
        except Exception as e:
            logger.error(f"Ошибка удаления знаний модели {model_name}: {e}")
            return 0

    def get_stats(self) -> Dict[str, int]:
        """Получение статистики по коллекциям."""
        return {
            "facts_count": self.facts_collection.count(),
            "files_count": self.files_collection.count(),
            "learnings_count": self.learnings_collection.count()
        }


# Глобальный экземпляр (будет использоваться в main.py)
memory_store = MemoryStore()
