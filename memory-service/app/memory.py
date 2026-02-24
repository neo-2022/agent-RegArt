import os
import uuid
import logging
import json
from datetime import datetime, timezone
from typing import List, Dict, Optional, Any

import chromadb
from chromadb.config import Settings as ChromaSettings
from chromadb.errors import NotFoundError
from sentence_transformers import SentenceTransformer

from .config import settings
from .ranking import build_rank_score

# Настройка логирования
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

LEARNING_STATUS_ACTIVE = "active"
LEARNING_STATUS_SUPERSEDED = "superseded"
LEARNING_STATUS_DELETED = "deleted"


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
        self.audit_collection = self._get_or_create_collection("agent_memory_audit")
        
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

    @staticmethod
    def _utc_now_iso() -> str:
        """Возвращает текущее UTC-время в ISO-формате для версионирования метаданных."""
        return datetime.now(timezone.utc).isoformat()

    @staticmethod
    def _as_int(value: Any, default: int = 0) -> int:
        """Безопасное преобразование к int для устойчивости к старым/грязным метаданным."""
        try:
            return int(value)
        except (TypeError, ValueError):
            return default

    @staticmethod
    def _is_active_learning(meta: Dict[str, Any]) -> bool:
        """Проверяет, является ли запись активной (не superseded и не deleted)."""
        return meta.get("status", LEARNING_STATUS_ACTIVE) == LEARNING_STATUS_ACTIVE

    def _build_learning_key(self, model_name: str, category: str, text: str) -> str:
        """
        Формирует стабильный ключ знания.

        Ключ используется для логического versioning: новые версии создаются поверх
        одной и той же сущности знания (learning_key).
        """
        # Логическая сущность знания группируется по модели+категории.
        # Текст может меняться между версиями (это и есть сигнал конфликта/эволюции знания).
        _ = text
        return f"{model_name.strip().lower()}::{category.strip().lower()}"

    def _find_latest_learning_version(self, learning_key: str) -> Optional[Dict[str, Any]]:
        """Возвращает последнюю активную версию знания по learning_key."""
        try:
            data = self.learnings_collection.get(
                where={"learning_key": learning_key},
                include=["metadatas", "documents"]
            )
        except Exception as e:
            logger.error(f"Ошибка чтения версии знания {learning_key}: {e}")
            return None

        ids = data.get("ids", []) if data else []
        metas = data.get("metadatas", []) if data else []
        docs = data.get("documents", []) if data else []
        if not ids:
            return None

        candidates = []
        for idx, doc_id in enumerate(ids):
            meta = metas[idx] if idx < len(metas) and isinstance(metas[idx], dict) else {}
            if not self._is_active_learning(meta):
                continue
            candidates.append({
                "id": doc_id,
                "metadata": meta,
                "document": docs[idx] if idx < len(docs) else "",
            })

        if not candidates:
            return None

        return max(candidates, key=lambda item: self._as_int(item["metadata"].get("version"), 1))

    def _add_audit_log(
        self,
        event_type: str,
        model_name: Optional[str] = None,
        workspace_id: Optional[str] = None,
        learning_id: Optional[str] = None,
        details: Optional[Dict[str, Any]] = None,
    ) -> None:
        """Пишет событие в коллекцию аудита (без чувствительных данных)."""
        audit_id = str(uuid.uuid4())
        created_at = self._utc_now_iso()
        log_metadata = {
            "event_type": event_type,
            "model_name": model_name or "",
            "workspace_id": workspace_id or "",
            "learning_id": learning_id or "",
            "created_at": created_at,
            "details_json": json.dumps(details or {}, ensure_ascii=False),
        }
        payload = f"event={event_type};model={model_name or ''};workspace={workspace_id or ''};learning={learning_id or ''}"
        embedding = self.encoder.encode(payload).tolist()
        self.audit_collection.add(
            embeddings=[embedding],
            documents=[payload],
            metadatas=[log_metadata],
            ids=[audit_id],
        )
    
    def _build_workspace_where(self, workspace_id: Optional[str]) -> Optional[Dict[str, Any]]:
        """Формирует where-фильтр для изоляции по workspace."""
        if not workspace_id:
            return None
        return {"workspace_id": workspace_id}

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
        
        fact_id = str(uuid.uuid4())
        embedding = self.encoder.encode(fact_text).tolist()
        
        fact_metadata = dict(metadata or {})
        # Явно фиксируем workspace в метаданных, даже если он не задан.
        # Это упрощает последующие миграции политики изоляции.
        fact_metadata.setdefault("workspace_id", "default")

        self.facts_collection.add(
            embeddings=[embedding],
            documents=[fact_text],
            metadatas=[fact_metadata],
            ids=[fact_id]
        )

        self._add_audit_log(
            event_type="fact_added",
            workspace_id=fact_metadata.get("workspace_id"),
            details={"fact_id": fact_id},
        )
        
        logger.info(f"Добавлен факт (ID: {fact_id}): {fact_text[:50]}...")
        return fact_id
    
    def search_facts(
        self,
        query: str,
        top_k: int = None,
        agent_name: Optional[str] = None,
        include_files: bool = False,
        workspace_id: Optional[str] = None,
    ) -> List[Dict[str, Any]]:
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
            facts_where = self._build_workspace_where(workspace_id)
            if agent_name and facts_where:
                facts_where = {"$and": [{"agent": agent_name}, facts_where]}
            elif agent_name:
                facts_where = {"agent": agent_name}

            facts_res = self.facts_collection.query(
                query_embeddings=[query_embedding],
                n_results=top_k,
                include=["documents", "distances", "metadatas"],
                where=facts_where
            )
            if facts_res and 'documents' in facts_res and facts_res['documents']:
                docs = facts_res['documents'][0]
                dists = facts_res.get('distances', [[]])[0]
                metas = facts_res.get('metadatas', [[]])[0]
                for i, doc in enumerate(docs):
                    dist = dists[i] if i < len(dists) else 1.0
                    relevance = max(0.0, 1.0 - dist)
                    meta = metas[i] if i < len(metas) else {}
                    score = build_rank_score(relevance, meta)
                    results.append({"text": doc, "score": score, "source": "facts", "metadata": meta})
        
        if include_files and self.files_collection.count() > 0:
            files_where = self._build_workspace_where(workspace_id)
            if agent_name and files_where:
                files_where = {"$and": [{"agent": agent_name}, files_where]}
            elif agent_name:
                files_where = {"agent": agent_name}

            files_res = self.files_collection.query(
                query_embeddings=[query_embedding],
                n_results=top_k,
                include=["documents", "distances", "metadatas"],
                where=files_where
            )
            if files_res and 'documents' in files_res and files_res['documents']:
                docs = files_res['documents'][0]
                dists = files_res.get('distances', [[]])[0]
                metas = files_res.get('metadatas', [[]])[0]
                for i, doc in enumerate(docs):
                    dist = dists[i] if i < len(dists) else 1.0
                    relevance = max(0.0, 1.0 - dist)
                    meta = metas[i] if i < len(metas) else {}
                    score = build_rank_score(relevance, meta)
                    results.append({"text": doc, "score": score, "source": "files", "metadata": meta})
        
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
        
        file_metadata = dict(metadata)
        file_metadata.setdefault("workspace_id", "default")

        self.files_collection.add(
            embeddings=[embedding],
            documents=[chunk_text],
            metadatas=[file_metadata],
            ids=[chunk_id]
        )

        self._add_audit_log(
            event_type="file_chunk_added",
            workspace_id=file_metadata.get("workspace_id"),
            details={"chunk_id": chunk_id, "file_name": file_metadata.get("file_name", file_metadata.get("filename", ""))},
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
                     category: str = "general", metadata: Optional[Dict[str, Any]] = None,
                     workspace_id: Optional[str] = None) -> str:
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
        
        learning_id = str(uuid.uuid4())
        embedding = self.encoder.encode(text).tolist()

        normalized_workspace = workspace_id or (metadata or {}).get("workspace_id") or "default"
        learning_key = self._build_learning_key(
            model_name=f"{normalized_workspace}:{model_name}",
            category=category,
            text=text,
        )
        latest = self._find_latest_learning_version(learning_key)
        next_version = 1
        previous_version_id: Optional[str] = None
        conflict_detected = False

        if latest:
            previous_version_id = latest["id"]
            next_version = self._as_int(latest["metadata"].get("version"), 1) + 1
            # Конфликт: та же логическая сущность знания, но текст изменился.
            # Обе версии сохраняются, предыдущая помечается как superseded.
            conflict_detected = latest.get("document", "").strip() != text.strip()

            latest_meta = dict(latest["metadata"])
            latest_meta["status"] = LEARNING_STATUS_SUPERSEDED
            latest_meta["superseded_at"] = self._utc_now_iso()
            latest_meta["superseded_by"] = learning_id
            self.learnings_collection.update(
                ids=[previous_version_id],
                metadatas=[latest_meta],
            )

        # Метаданные знания — привязка к модели, агенту и категории + versioning.
        learning_metadata = {
            "model_name": model_name,
            "agent_name": agent_name,
            "category": category,
            "workspace_id": normalized_workspace,
            "learning_key": learning_key,
            "version": next_version,
            "status": LEARNING_STATUS_ACTIVE,
            "created_at": self._utc_now_iso(),
            "conflict_detected": conflict_detected,
            "previous_version_id": previous_version_id or "",
        }
        if metadata:
            learning_metadata.update(metadata)

        self.learnings_collection.add(
            embeddings=[embedding],
            documents=[text],
            metadatas=[learning_metadata],
            ids=[learning_id]
        )

        self._add_audit_log(
            event_type="learning_added",
            model_name=model_name,
            workspace_id=normalized_workspace,
            learning_id=learning_id,
            details={
                "version": next_version,
                "conflict_detected": conflict_detected,
                "previous_version_id": previous_version_id or "",
            },
        )

        logger.info(
            f"Добавлено знание для модели {model_name} (категория: {category}, версия: {next_version}, конфликт: {conflict_detected})"
        )
        return learning_id
    
    def search_learnings(self, query: str, model_name: str,
                         top_k: int = 5, category: Optional[str] = None,
                         workspace_id: Optional[str] = None) -> List[Dict[str, Any]]:
        """
        Поиск релевантных знаний для конкретной модели LLM.
        Возвращает структурированные результаты: text, score, source, metadata.
        """
        if self.learnings_collection.count() == 0:
            return []
        
        query_embedding = self.encoder.encode(query).tolist()
        
        base_filter: Dict[str, Any] = {"model_name": model_name}
        if workspace_id:
            base_filter = {"$and": [base_filter, {"workspace_id": workspace_id}]}

        where_filter = base_filter
        if category:
            where_filter = {"$and": [base_filter, {"category": category}]}
        
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
                    relevance = max(0.0, 1.0 - dist)
                    meta = metas[i] if i < len(metas) else {}
                    if self._is_active_learning(meta):
                        score = build_rank_score(relevance, meta)
                        items.append({"text": doc, "score": score, "source": "learnings", "metadata": meta})
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

    def get_learning_metadata(self, learning_id: str) -> Dict[str, Any]:
        """Возвращает метаданные знания по ID (используется API-слоем для ответа клиенту)."""
        try:
            data = self.learnings_collection.get(ids=[learning_id], include=["metadatas"])
            metas = data.get("metadatas", []) if data else []
            if metas and isinstance(metas[0], dict):
                return metas[0]
        except Exception as e:
            logger.error(f"Ошибка получения метаданных знания {learning_id}: {e}")
        return {}

    def list_learning_versions(
        self,
        model_name: str,
        category: Optional[str] = None,
        workspace_id: Optional[str] = None,
    ) -> List[Dict[str, Any]]:
        """Возвращает историю версий знаний с фильтрами модели/категории/workspace."""
        where_filter: Dict[str, Any] = {"model_name": model_name}
        if workspace_id:
            where_filter = {"$and": [where_filter, {"workspace_id": workspace_id}]}
        if category:
            where_filter = {"$and": [where_filter, {"category": category}]}

        try:
            data = self.learnings_collection.get(where=where_filter, include=["metadatas", "documents"])
            ids = data.get("ids", []) if data else []
            metas = data.get("metadatas", []) if data else []
            docs = data.get("documents", []) if data else []
            versions: List[Dict[str, Any]] = []

            for idx, learning_id in enumerate(ids):
                meta = metas[idx] if idx < len(metas) and isinstance(metas[idx], dict) else {}
                versions.append({
                    "id": learning_id,
                    "version": self._as_int(meta.get("version"), 1),
                    "status": str(meta.get("status", LEARNING_STATUS_ACTIVE)),
                    "text": docs[idx] if idx < len(docs) else "",
                    "metadata": meta,
                })

            versions.sort(key=lambda item: item["version"], reverse=True)
            return versions
        except Exception as e:
            logger.error(f"Ошибка получения истории версий для модели {model_name}: {e}")
            return []
    
    def delete_model_learnings(self, model_name: str, category: Optional[str] = None,
                               workspace_id: Optional[str] = None) -> int:
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
            where_filter: Dict[str, Any] = {"model_name": model_name}
            if workspace_id:
                where_filter = {"$and": [where_filter, {"workspace_id": workspace_id}]}
            if category:
                where_filter = {"$and": [where_filter, {"category": category}]}
            
            results = self.learnings_collection.get(where=where_filter)
            if not results or 'ids' not in results:
                return 0
            
            ids_to_delete = results['ids']
            if ids_to_delete:
                metas = results.get("metadatas", [])
                active_pairs = []
                for idx, learning_id in enumerate(ids_to_delete):
                    meta = metas[idx] if idx < len(metas) and isinstance(metas[idx], dict) else {}
                    if not self._is_active_learning(meta):
                        continue
                    updated_meta = dict(meta)
                    updated_meta["status"] = LEARNING_STATUS_DELETED
                    updated_meta["deleted_at"] = self._utc_now_iso()
                    active_pairs.append((learning_id, updated_meta))

                if active_pairs:
                    self.learnings_collection.update(
                        ids=[item[0] for item in active_pairs],
                        metadatas=[item[1] for item in active_pairs],
                    )
                    logger.info(f"Soft-delete: помечено удалёнными {len(active_pairs)} знаний модели {model_name}")
                    self._add_audit_log(
                        event_type="learnings_soft_deleted",
                        model_name=model_name,
                        workspace_id=workspace_id,
                        details={"deleted_count": len(active_pairs), "category": category or ""},
                    )
                    return len(active_pairs)
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

    def list_audit_logs(
        self,
        top_k: int = 100,
        workspace_id: Optional[str] = None,
        model_name: Optional[str] = None,
    ) -> List[Dict[str, Any]]:
        """Возвращает список аудита с опциональными фильтрами."""
        where_filter: Optional[Dict[str, Any]] = None
        filters = []
        if workspace_id:
            filters.append({"workspace_id": workspace_id})
        if model_name:
            filters.append({"model_name": model_name})
        if len(filters) == 1:
            where_filter = filters[0]
        elif len(filters) > 1:
            where_filter = {"$and": filters}

        try:
            data = self.audit_collection.get(where=where_filter, include=["metadatas"])
            ids = data.get("ids", []) if data else []
            metas = data.get("metadatas", []) if data else []
            logs: List[Dict[str, Any]] = []
            for idx, item_id in enumerate(ids):
                meta = metas[idx] if idx < len(metas) and isinstance(metas[idx], dict) else {}
                logs.append({
                    "id": item_id,
                    "event_type": str(meta.get("event_type", "")),
                    "model_name": str(meta.get("model_name", "")) or None,
                    "workspace_id": str(meta.get("workspace_id", "")) or None,
                    "learning_id": str(meta.get("learning_id", "")) or None,
                    "created_at": str(meta.get("created_at", "")),
                    "details": json.loads(meta.get("details_json", "{}")) if isinstance(meta.get("details_json", "{}"), str) else {},
                })

            logs.sort(key=lambda item: item.get("created_at", ""), reverse=True)
            return logs[:max(top_k, 1)]
        except Exception as e:
            logger.error(f"Ошибка получения аудита: {e}")
            return []


# Глобальный экземпляр (будет использоваться в main.py)
memory_store = MemoryStore()
