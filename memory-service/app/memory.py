import os
import uuid
import logging
import json
import time
from datetime import datetime, timezone
from threading import Lock
from typing import List, Dict, Optional, Any

from qdrant_client import QdrantClient
from sentence_transformers import SentenceTransformer

from .config import settings
from .qdrant_store import QdrantCollectionCompat
from .ranking import build_rank_score, blend_relevance_scores, resolve_priority_score
from .vector_backend import VECTOR_BACKEND_QDRANT

# Настройка логирования
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

LEARNING_STATUS_ACTIVE = "active"
LEARNING_STATUS_SUPERSEDED = "superseded"
LEARNING_STATUS_DELETED = "deleted"


class MemoryStore:
    """
    Класс для работы с долговременной памятью (RAG).
    Использует Qdrant как единственный backend векторного хранилища (этап миграции Eternal RAG).
    """
    
    def __init__(self):
        """Инициализация клиента Qdrant и модели эмбеддингов."""
        # Создаём директории данных, если их нет
        os.makedirs(settings.QDRANT_PATH, exist_ok=True)
        os.makedirs(settings.TEMP_DIR, exist_ok=True)

        # На этом этапе memory-service работает только через Qdrant backend.
        # Ранняя проверка защищает от случайного старта в legacy-режиме.
        if settings.VECTOR_BACKEND != VECTOR_BACKEND_QDRANT:
            raise RuntimeError(
                f"memory-service поддерживает только VECTOR_BACKEND=qdrant, получено: {settings.VECTOR_BACKEND}"
            )

        # Инициализация клиента Qdrant: локальный persistent-режим или внешний URL.
        if settings.QDRANT_URL:
            self.client = QdrantClient(url=settings.QDRANT_URL)
        else:
            self.client = QdrantClient(path=settings.QDRANT_PATH)

        # Загружаем модель эмбеддингов до инициализации коллекций,
        # чтобы создавать коллекции с корректной размерностью вектора.
        logger.info(f"Загрузка модели эмбеддингов: {settings.EMBEDDING_MODEL}")
        self.encoder = SentenceTransformer(settings.EMBEDDING_MODEL)
        logger.info("Модель эмбеддингов загружена")
        self._vector_size = int(self.encoder.get_sentence_embedding_dimension())

        # Создаём или получаем коллекции
        self.facts_collection = self._get_or_create_collection("agent_memory_facts")
        self.files_collection = self._get_or_create_collection("agent_memory_files")
        # Коллекция для обучения агентов — хранит знания, извлечённые из диалогов.
        # Каждое знание привязано к конкретной модели LLM через метаданные (model_name).
        # Это позволяет каждой модели накапливать свою уникальную базу знаний.
        self.learnings_collection = self._get_or_create_collection("agent_learnings")
        self.audit_collection = self._get_or_create_collection("agent_memory_audit")

        # === Skill Engine & Graph Engine (Eternal RAG: разделы 5.3, 5.4) ===
        # Коллекции для навыков и связей графа знаний.
        # Инициализируются лениво через свойства skill_engine / graph_engine,
        # чтобы не замедлять старт сервиса, если движки не используются.
        self._skills_collection = self._get_or_create_collection(settings.SKILL_COLLECTION_NAME)
        self._graph_collection = self._get_or_create_collection(settings.GRAPH_COLLECTION_NAME)
        self._skill_engine = None
        self._graph_engine = None

        self._metrics_lock = Lock()
        self._retrieval_metrics: Dict[str, float] = {
            "search_requests_total": 0,
            "search_errors_total": 0,
            "search_latency_ms_total": 0.0,
            "search_results_total": 0,
        }

    
    def _get_or_create_collection(self, name: str):
        """Вспомогательный метод для получения/создания коллекции Qdrant."""
        return QdrantCollectionCompat(client=self.client, name=name, vector_size=self._vector_size)

    @property
    def skill_engine(self):
        """Ленивая инициализация Skill Engine (Eternal RAG: раздел 5.3)."""
        if self._skill_engine is None:
            from .skill_engine import SkillEngine
            self._skill_engine = SkillEngine(
                collection=self._skills_collection,
                encoder=self.encoder,
            )
        return self._skill_engine

    @property
    def graph_engine(self):
        """Ленивая инициализация Graph Engine (Eternal RAG: раздел 5.4)."""
        if self._graph_engine is None:
            from .graph_engine import GraphEngine
            self._graph_engine = GraphEngine(
                collection=self._graph_collection,
                encoder=self.encoder,
            )
        return self._graph_engine

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

    def _encode_to_list(self, text: str) -> list:
        """
        Кодирует текст в вектор и возвращает как список (list).

        Обрабатывает случай, когда encoder.encode() возвращает как numpy-массив
        (production: SentenceTransformer), так и обычный список (тесты: mock).
        """
        result = self.encoder.encode(text)
        if isinstance(result, list):
            return result
        return result.tolist()

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
        embedding = self._encode_to_list(payload)
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

    def _detect_contradictions(
        self,
        text: str,
        embedding: list,
        model_name: str,
        workspace_id: str,
        exclude_id: Optional[str] = None,
    ) -> List[Dict[str, Any]]:
        """
        Обнаружение противоречий (Eternal RAG: раздел 8).

        При добавлении нового знания сравниваем его embedding с существующими знаниями
        той же модели. Если семантическая близость высокая (превышает порог
        CONTRADICTION_SIMILARITY_THRESHOLD), но тексты различаются — фиксируем
        потенциальное противоречие.

        Edge-cases:
        - пустая коллекция — возвращаем пустой список;
        - знание с exclude_id (текущая запись) пропускается;
        - неактивные знания (superseded/deleted) игнорируются;
        - ошибки поиска не блокируют добавление знания.

        Returns:
            Список противоречий: [{"id", "text", "similarity", "learning_key"}]
        """
        if self.learnings_collection.count() == 0:
            return []

        threshold = settings.CONTRADICTION_SIMILARITY_THRESHOLD
        top_k = settings.CONTRADICTION_TOP_K

        try:
            where_filter: Dict[str, Any] = {"model_name": model_name}
            if workspace_id:
                where_filter = {"$and": [{"model_name": model_name}, {"workspace_id": workspace_id}]}

            results = self.learnings_collection.query(
                query_embeddings=[embedding],
                n_results=top_k,
                include=["documents", "distances", "metadatas"],
                where=where_filter,
            )
            if not results or "documents" not in results or not results["documents"]:
                return []

            docs = results["documents"][0]
            dists = results.get("distances", [[]])[0]
            metas = results.get("metadatas", [[]])[0]
            ids = results.get("ids", [[]])[0]

            contradictions: List[Dict[str, Any]] = []
            normalized_text = text.strip().lower()

            for i, doc in enumerate(docs):
                doc_id = ids[i] if i < len(ids) else ""
                # Пропускаем текущую запись
                if exclude_id and doc_id == exclude_id:
                    continue

                meta = metas[i] if i < len(metas) else {}
                # Игнорируем неактивные знания
                if not self._is_active_learning(meta):
                    continue

                dist = dists[i] if i < len(dists) else 1.0
                similarity = max(0.0, 1.0 - dist)

                # Высокая семантическая близость, но текст отличается — противоречие
                if similarity >= threshold and doc.strip().lower() != normalized_text:
                    contradictions.append({
                        "id": doc_id,
                        "text": doc,
                        "similarity": round(similarity, 4),
                        "learning_key": str(meta.get("learning_key", "")),
                    })

            return contradictions
        except Exception as e:
            logger.warning(f"Ошибка при детекции противоречий (не блокирует добавление): {e}")
            return []

    @staticmethod
    def _keyword_relevance(query: str, text: str) -> float:
        """
        Лёгкий keyword-сигнал релевантности [0..1] по доле совпавших токенов запроса.

        Edge-cases:
        - пустой query/text -> 0.0;
        - повторяющиеся токены запроса схлопываются множеством, чтобы не завышать score.
        """
        if not query or not text:
            return 0.0
        query_tokens = {token.strip().lower() for token in query.split() if token.strip()}
        if not query_tokens:
            return 0.0
        text_lc = text.lower()
        matched = sum(1 for token in query_tokens if token in text_lc)
        return matched / len(query_tokens)

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
        embedding = self._encode_to_list(fact_text)
        
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
        min_priority: Optional[str] = None,
    ) -> List[Dict[str, Any]]:
        """
        Поиск релевантных фактов и/или фрагментов файлов.
        Возвращает структурированные результаты: text, score, source, metadata.
        """
        start_ts = time.perf_counter()
        if top_k is None:
            top_k = settings.TOP_K
        
        if self.facts_collection.count() == 0 and (not include_files or self.files_collection.count() == 0):
            self._record_search_metrics(start_ts=start_ts, results_count=0, is_error=False)
            return []
        
        query_embedding = self._encode_to_list(query)
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
                fact_ids = facts_res.get('ids', [[]])[0]
                for i, doc in enumerate(docs):
                    dist = dists[i] if i < len(dists) else 1.0
                    semantic_relevance = max(0.0, 1.0 - dist)
                    keyword_relevance = self._keyword_relevance(query=query, text=doc)
                    relevance = blend_relevance_scores(semantic_relevance, keyword_relevance)
                    meta = metas[i] if i < len(metas) else {}
                    doc_id = fact_ids[i] if i < len(fact_ids) else ""
                    score = build_rank_score(relevance, meta)
                    results.append({"id": doc_id, "text": doc, "score": score, "source": "facts", "metadata": meta})
        
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
                file_ids = files_res.get('ids', [[]])[0]
                for i, doc in enumerate(docs):
                    dist = dists[i] if i < len(dists) else 1.0
                    semantic_relevance = max(0.0, 1.0 - dist)
                    keyword_relevance = self._keyword_relevance(query=query, text=doc)
                    relevance = blend_relevance_scores(semantic_relevance, keyword_relevance)
                    meta = metas[i] if i < len(metas) else {}
                    doc_id = file_ids[i] if i < len(file_ids) else ""
                    score = build_rank_score(relevance, meta)
                    results.append({"id": doc_id, "text": doc, "score": score, "source": "files", "metadata": meta})
        
        seen = set()
        unique: List[Dict[str, Any]] = []
        for r in results:
            if r["text"] not in seen:
                seen.add(r["text"])
                unique.append(r)
        
        # Фильтрация по минимальному приоритету памяти применяется после объединения
        # candidates из разных источников, чтобы одинаково обрабатывать facts и files.
        if min_priority:
            threshold = resolve_priority_score(min_priority)
            unique = [
                item for item in unique
                if resolve_priority_score((item.get("metadata") or {}).get("priority", "normal")) >= threshold
            ]

        unique.sort(key=lambda x: x["score"], reverse=True)
        self._record_search_metrics(start_ts=start_ts, results_count=len(unique), is_error=False)
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
        embedding = self._encode_to_list(chunk_text)
        
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

    def rename_file(self, old_name: str, new_name: str) -> int:
        """
        Переименование файла в RAG-базе знаний.

        Обновляет метаданные file_name у всех чанков файла.
        Не затрагивает содержимое и embeddings — только имя.

        Args:
            old_name: Текущее имя файла
            new_name: Новое имя файла

        Returns:
            Количество обновлённых чанков
        """
        if not old_name or not new_name or not new_name.strip():
            logger.warning("Попытка переименования с пустым именем")
            return 0

        try:
            results = self.files_collection.get(
                where={"file_name": old_name},
                include=["metadatas"],
            )
            if not results or "ids" not in results or not results["ids"]:
                # Пробуем альтернативный ключ metadata (filename)
                results = self.files_collection.get(
                    where={"filename": old_name},
                    include=["metadatas"],
                )
            if not results or "ids" not in results or not results["ids"]:
                return 0

            ids = results["ids"]
            metas = results.get("metadatas", [])
            updated_metas = []
            for idx, doc_id in enumerate(ids):
                meta = dict(metas[idx]) if idx < len(metas) and isinstance(metas[idx], dict) else {}
                meta["file_name"] = new_name.strip()
                # Обновляем и альтернативный ключ, если он был
                if "filename" in meta:
                    meta["filename"] = new_name.strip()
                updated_metas.append(meta)

            self.files_collection.update(ids=ids, metadatas=updated_metas)

            self._add_audit_log(
                event_type="file_renamed",
                details={"old_name": old_name, "new_name": new_name.strip(), "chunks_updated": len(ids)},
            )

            logger.info(f"Переименован файл «{old_name}» → «{new_name.strip()}» ({len(ids)} чанков)")
            return len(ids)
        except Exception as e:
            logger.error(f"Ошибка переименования файла {old_name}: {e}")
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
    
    def move_file(self, file_name: str, target_folder: str) -> tuple:
        """
        Перемещение файла между папками в RAG-базе знаний.

        Обновляет метаданные file_name у всех чанков файла, добавляя/заменяя
        префикс папки. Содержимое и embeddings не затрагиваются.

        Args:
            file_name: Текущее имя файла (может содержать путь папки)
            target_folder: Целевая папка

        Returns:
            Кортеж (old_path, new_path, chunks_updated)
        """
        if not file_name or not target_folder:
            return (file_name, file_name, 0)

        try:
            # Ищем файл по текущему имени
            results = self.files_collection.get(
                where={"file_name": file_name},
                include=["metadatas"],
            )
            if not results or "ids" not in results or not results["ids"]:
                results = self.files_collection.get(
                    where={"filename": file_name},
                    include=["metadatas"],
                )
            if not results or "ids" not in results or not results["ids"]:
                return (file_name, file_name, 0)

            # Извлекаем базовое имя файла (без текущей папки)
            parts = file_name.rsplit("/", 1)
            base_name = parts[-1] if len(parts) > 1 else file_name

            # Формируем новый путь
            target_folder = target_folder.strip().rstrip("/")
            new_path = f"{target_folder}/{base_name}"

            ids = results["ids"]
            metas = results.get("metadatas", [])
            updated_metas = []
            for idx in range(len(ids)):
                meta = dict(metas[idx]) if idx < len(metas) and isinstance(metas[idx], dict) else {}
                meta["file_name"] = new_path
                meta["folder"] = target_folder
                if "filename" in meta:
                    meta["filename"] = new_path
                updated_metas.append(meta)

            self.files_collection.update(ids=ids, metadatas=updated_metas)

            self._add_audit_log(
                event_type="file_moved",
                details={"old_path": file_name, "new_path": new_path, "chunks_updated": len(ids)},
            )

            logger.info(f"Перемещён файл «{file_name}» → «{new_path}» ({len(ids)} чанков)")
            return (file_name, new_path, len(ids))
        except Exception as e:
            logger.error(f"Ошибка перемещения файла {file_name}: {e}")
            return (file_name, file_name, 0)

    def soft_delete_file(self, file_name: str) -> int:
        """
        Мягкое удаление файла — пометка deleted_at вместо физического удаления.

        Файл остаётся в коллекции, но исключается из поиска и списков.
        Можно восстановить через restore_file().

        Args:
            file_name: Имя файла для мягкого удаления

        Returns:
            Количество помеченных чанков
        """
        try:
            results = self.files_collection.get(
                where={"file_name": file_name},
                include=["metadatas"],
            )
            if not results or "ids" not in results or not results["ids"]:
                results = self.files_collection.get(
                    where={"filename": file_name},
                    include=["metadatas"],
                )
            if not results or "ids" not in results or not results["ids"]:
                return 0

            ids = results["ids"]
            metas = results.get("metadatas", [])
            deleted_at = self._utc_now_iso()
            updated_metas = []
            for idx in range(len(ids)):
                meta = dict(metas[idx]) if idx < len(metas) and isinstance(metas[idx], dict) else {}
                meta["deleted_at"] = deleted_at
                meta["status"] = "deleted"
                updated_metas.append(meta)

            self.files_collection.update(ids=ids, metadatas=updated_metas)

            self._add_audit_log(
                event_type="file_soft_deleted",
                details={"file_name": file_name, "chunks_marked": len(ids)},
            )

            logger.info(f"Мягкое удаление файла «{file_name}» ({len(ids)} чанков)")
            return len(ids)
        except Exception as e:
            logger.error(f"Ошибка мягкого удаления файла {file_name}: {e}")
            return 0

    def restore_file(self, file_name: str) -> int:
        """
        Восстановление мягко удалённого файла.

        Снимает пометку deleted_at и возвращает файл в активное состояние.

        Args:
            file_name: Имя файла для восстановления

        Returns:
            Количество восстановленных чанков
        """
        try:
            results = self.files_collection.get(
                where={"file_name": file_name},
                include=["metadatas"],
            )
            if not results or "ids" not in results or not results["ids"]:
                return 0

            ids = results["ids"]
            metas = results.get("metadatas", [])
            restored = 0
            updated_ids = []
            updated_metas = []
            for idx in range(len(ids)):
                meta = dict(metas[idx]) if idx < len(metas) and isinstance(metas[idx], dict) else {}
                if meta.get("deleted_at") or meta.get("status") == "deleted":
                    meta.pop("deleted_at", None)
                    meta["status"] = "active"
                    updated_ids.append(ids[idx])
                    updated_metas.append(meta)
                    restored += 1

            if updated_ids:
                self.files_collection.update(ids=updated_ids, metadatas=updated_metas)

            self._add_audit_log(
                event_type="file_restored",
                details={"file_name": file_name, "chunks_restored": restored},
            )

            logger.info(f"Восстановлен файл «{file_name}» ({restored} чанков)")
            return restored
        except Exception as e:
            logger.error(f"Ошибка восстановления файла {file_name}: {e}")
            return 0

    def pin_file(self, file_name: str, pinned: bool = True) -> int:
        """
        Закрепление/открепление файла.

        Закреплённые файлы показываются первыми в списке и не удаляются по TTL.

        Args:
            file_name: Имя файла
            pinned: True — закрепить, False — открепить

        Returns:
            Количество обновлённых чанков
        """
        try:
            results = self.files_collection.get(
                where={"file_name": file_name},
                include=["metadatas"],
            )
            if not results or "ids" not in results or not results["ids"]:
                results = self.files_collection.get(
                    where={"filename": file_name},
                    include=["metadatas"],
                )
            if not results or "ids" not in results or not results["ids"]:
                return 0

            ids = results["ids"]
            metas = results.get("metadatas", [])
            updated_metas = []
            for idx in range(len(ids)):
                meta = dict(metas[idx]) if idx < len(metas) and isinstance(metas[idx], dict) else {}
                meta["pinned"] = "true" if pinned else "false"
                # Закреплённые файлы получают приоритет "pinned" для ранжирования
                if pinned:
                    meta["priority"] = "pinned"
                else:
                    meta.setdefault("priority", "normal")
                    if meta["priority"] == "pinned":
                        meta["priority"] = "normal"
                updated_metas.append(meta)

            self.files_collection.update(ids=ids, metadatas=updated_metas)

            action = "закреплён" if pinned else "откреплён"
            self._add_audit_log(
                event_type="file_pinned" if pinned else "file_unpinned",
                details={"file_name": file_name, "chunks_updated": len(ids)},
            )

            logger.info(f"Файл «{file_name}» {action} ({len(ids)} чанков)")
            return len(ids)
        except Exception as e:
            logger.error(f"Ошибка закрепления файла {file_name}: {e}")
            return 0

    def search_file_contents(self, query: str, top_k: int = 10, folder: Optional[str] = None) -> List[Dict[str, Any]]:
        """
        Семантический поиск по содержимому файлов RAG-базы.

        Находит наиболее релевантные фрагменты файлов по запросу.
        Исключает мягко удалённые файлы из результатов.

        Args:
            query: Поисковый запрос
            top_k: Максимум результатов
            folder: Фильтр по папке (опционально)

        Returns:
            Список результатов с file_name, chunk_text, score, metadata
        """
        if self.files_collection.count() == 0:
            return []

        try:
            query_embedding = self._encode_to_list(query)
            results = self.files_collection.query(
                query_embeddings=[query_embedding],
                n_results=min(top_k * 2, self.files_collection.count()),
                include=["documents", "metadatas", "distances"],
            )

            if not results or "ids" not in results or not results["ids"] or not results["ids"][0]:
                return []

            items = []
            for idx in range(len(results["ids"][0])):
                meta = results["metadatas"][0][idx] if results.get("metadatas") and results["metadatas"][0] else {}

                # Исключаем мягко удалённые файлы
                if meta.get("deleted_at") or meta.get("status") == "deleted":
                    continue

                file_name = meta.get("file_name", meta.get("filename", "unknown"))

                # Фильтр по папке
                if folder:
                    file_folder = file_name.rsplit("/", 1)[0] if "/" in file_name else ""
                    if file_folder != folder:
                        continue

                distance = results["distances"][0][idx] if results.get("distances") and results["distances"][0] else 1.0
                score = max(0.0, 1.0 - distance)
                doc_text = results["documents"][0][idx] if results.get("documents") and results["documents"][0] else ""

                items.append({
                    "file_name": file_name,
                    "chunk_text": doc_text,
                    "score": round(score, 4),
                    "chunk_index": int(meta.get("chunk_index", 0)),
                    "metadata": {k: v for k, v in meta.items() if k not in ("deleted_at",)},
                })

                if len(items) >= top_k:
                    break

            return items
        except Exception as e:
            logger.error(f"Ошибка поиска по содержимому файлов: {e}")
            return []

    def list_contradictions(self, top_k: int = 50) -> List[Dict[str, Any]]:
        """
        Получение списка обнаруженных противоречий между знаниями.

        Сканирует коллекцию обучения и находит записи с пометкой conflict_detected.
        Возвращает пары конфликтующих знаний.

        Args:
            top_k: Максимум записей

        Returns:
            Список словарей с информацией о противоречиях
        """
        try:
            data = self.learnings_collection.get(include=["metadatas", "documents"])
            if not data or "ids" not in data or not data["ids"]:
                return []

            contradictions = []
            for idx, doc_id in enumerate(data["ids"]):
                meta = data["metadatas"][idx] if idx < len(data.get("metadatas", [])) else {}
                if not isinstance(meta, dict):
                    continue

                # Только записи с обнаруженными противоречиями
                if not meta.get("conflict_detected"):
                    continue

                contradictions_json = meta.get("contradictions_json", "")
                if not contradictions_json:
                    continue

                try:
                    conflict_list = json.loads(contradictions_json)
                except (ValueError, TypeError):
                    continue

                doc_text = data["documents"][idx] if idx < len(data.get("documents", [])) else ""
                for conflict in conflict_list:
                    contradictions.append({
                        "new_learning_id": doc_id,
                        "existing_learning_id": conflict.get("id", ""),
                        "new_text": doc_text,
                        "existing_text": conflict.get("text", ""),
                        "similarity": conflict.get("similarity", 0.0),
                        "model_name": meta.get("model_name", ""),
                        "detected_at": meta.get("created_at", ""),
                    })

                if len(contradictions) >= top_k:
                    break

            return contradictions
        except Exception as e:
            logger.error(f"Ошибка получения списка противоречий: {e}")
            return []

    def list_deleted_files(self) -> List[str]:
        """
        Получение списка мягко удалённых файлов для отображения в корзине.

        Сканирует коллекцию файлов и возвращает уникальные имена файлов
        с пометкой status=deleted или наличием deleted_at.

        Returns:
            Список уникальных имён удалённых файлов
        """
        try:
            data = self.files_collection.get(include=["metadatas"])
            if not data or "ids" not in data or not data["ids"]:
                return []

            deleted_names: set = set()
            for idx in range(len(data["ids"])):
                meta = data["metadatas"][idx] if idx < len(data.get("metadatas", [])) else {}
                if not isinstance(meta, dict):
                    continue
                if meta.get("deleted_at") or meta.get("status") == "deleted":
                    file_name = meta.get("file_name", meta.get("filename", ""))
                    if file_name:
                        deleted_names.add(file_name)

            return sorted(deleted_names)
        except Exception as e:
            logger.error(f"Ошибка получения списка удалённых файлов: {e}")
            return []

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
        embedding = self._encode_to_list(text)

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

        # === Детекция противоречий (Eternal RAG: раздел 8) ===
        # Ищем семантически похожие знания с отличающимся текстом.
        # Ошибки детекции не блокируют добавление знания.
        contradictions = self._detect_contradictions(
            text=text,
            embedding=embedding,
            model_name=model_name,
            workspace_id=normalized_workspace,
            exclude_id=previous_version_id,
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
            # Семантические противоречия с другими знаниями
            "contradictions_count": len(contradictions),
            "contradictions_json": json.dumps(contradictions, ensure_ascii=False) if contradictions else "",
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
                "contradictions_count": len(contradictions),
                "previous_version_id": previous_version_id or "",
            },
        )

        if contradictions:
            logger.info(
                f"Обнаружено {len(contradictions)} противоречий для знания модели {model_name}"
            )

        logger.info(
            f"Добавлено знание для модели {model_name} (категория: {category}, версия: {next_version}, "
            f"конфликт: {conflict_detected}, противоречий: {len(contradictions)})"
        )
        return learning_id
    
    def search_learnings(self, query: str, model_name: str,
                         top_k: int = 5, category: Optional[str] = None,
                         workspace_id: Optional[str] = None,
                         min_priority: Optional[str] = None) -> List[Dict[str, Any]]:
        """
        Поиск релевантных знаний для конкретной модели LLM.
        Возвращает структурированные результаты: text, score, source, metadata.
        """
        start_ts = time.perf_counter()
        if self.learnings_collection.count() == 0:
            self._record_search_metrics(start_ts=start_ts, results_count=0, is_error=False)
            return []
        
        query_embedding = self._encode_to_list(query)
        
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
                # ID документов из ChromaDB — нужны для Graph Engine
                # (autoCreateGraphRelationships использует id для создания связей)
                ids = results.get('ids', [[]])[0]
                items: List[Dict[str, Any]] = []
                for i, doc in enumerate(docs):
                    dist = dists[i] if i < len(dists) else 1.0
                    semantic_relevance = max(0.0, 1.0 - dist)
                    keyword_relevance = self._keyword_relevance(query=query, text=doc)
                    relevance = blend_relevance_scores(semantic_relevance, keyword_relevance)
                    meta = metas[i] if i < len(metas) else {}
                    doc_id = ids[i] if i < len(ids) else ""
                    if self._is_active_learning(meta):
                        score = build_rank_score(relevance, meta)
                        items.append({"id": doc_id, "text": doc, "score": score, "source": "learnings", "metadata": meta})
                if min_priority:
                    threshold = resolve_priority_score(min_priority)
                    items = [
                        item for item in items
                        if resolve_priority_score((item.get("metadata") or {}).get("priority", "normal")) >= threshold
                    ]
                items.sort(key=lambda x: x["score"], reverse=True)
                self._record_search_metrics(start_ts=start_ts, results_count=len(items), is_error=False)
                return items
        except Exception as e:
            logger.error(f"Ошибка поиска знаний для модели {model_name}: {e}")
            self._record_search_metrics(start_ts=start_ts, results_count=0, is_error=True)
        
        return []

    def _record_search_metrics(self, start_ts: float, results_count: int, is_error: bool) -> None:
        """Атомарно обновляет метрики retrieval для мониторинга."""
        latency_ms = (time.perf_counter() - start_ts) * 1000.0
        with self._metrics_lock:
            self._retrieval_metrics["search_requests_total"] += 1
            self._retrieval_metrics["search_latency_ms_total"] += latency_ms
            self._retrieval_metrics["search_results_total"] += max(results_count, 0)
            if is_error:
                self._retrieval_metrics["search_errors_total"] += 1

    def get_retrieval_metrics(self) -> Dict[str, float]:
        """Возвращает срез retrieval-метрик с вычисленным средним временем ответа."""
        with self._metrics_lock:
            requests_total = self._retrieval_metrics["search_requests_total"]
            latency_total = self._retrieval_metrics["search_latency_ms_total"]
            avg_latency = latency_total / requests_total if requests_total > 0 else 0.0
            return {
                "search_requests_total": int(requests_total),
                "search_errors_total": int(self._retrieval_metrics["search_errors_total"]),
                "search_results_total": int(self._retrieval_metrics["search_results_total"]),
                "search_latency_ms_avg": round(avg_latency, 3),
            }
    
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
            # Используем плоский dict для простых AND-условий —
            # Qdrant поддерживает несколько ключей в одном where-фильтре как неявный AND
            where_filter: Dict[str, Any] = {"model_name": model_name}
            if workspace_id:
                where_filter["workspace_id"] = workspace_id
            if category:
                where_filter["category"] = category
            
            results = self.learnings_collection.get(where=where_filter, include=["metadatas"])
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

    def get_embedding_status(self) -> Dict[str, Any]:
        """
        Возвращает статус модели эмбеддингов для мониторинга в UI.

        Информация включает имя модели, размерность вектора, версию и
        количество документов по коллекциям. Используется UI-индикатором
        состояния эмбеддингов (Eternal RAG: раздел 5.8 Monitoring Engine).
        """
        return {
            "model_name": settings.EMBEDDING_MODEL,
            "model_version": settings.EMBEDDING_MODEL_VERSION,
            "vector_size": self._vector_size,
            "status": "loaded",
            "collections": {
                "facts": self.facts_collection.count(),
                "files": self.files_collection.count(),
                "learnings": self.learnings_collection.count(),
            },
        }

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
