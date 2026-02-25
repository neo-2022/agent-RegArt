"""
Skill Engine — движок навыков агента (Eternal RAG: раздел 5.3, 7).

Навыки хранят структурированные знания агента: цель, шаги, примеры,
ограничения, источники, confidence и version. Индексируются в Qdrant
для семантического поиска и автоматического применения при retrieval.

Жизненный цикл навыка:
  создание → индексация → поиск → применение → обновление (версионирование)

При обновлении навыка создаётся новая версия, старая помечается как superseded.
Confidence обновляется при каждом успешном применении навыка.
"""

from __future__ import annotations

import json
import logging
import uuid
from datetime import datetime, timezone
from typing import Any, Dict, List, Optional

from .config import settings

logger = logging.getLogger(__name__)

# Статусы навыков
SKILL_STATUS_ACTIVE = "active"
SKILL_STATUS_SUPERSEDED = "superseded"
SKILL_STATUS_DELETED = "deleted"


class SkillEngine:
    """
    Движок управления навыками агента.

    Использует Qdrant-коллекцию для хранения и семантического поиска навыков.
    Каждый навык — точка в Qdrant с embedding цели (goal) и метаданными.

    Атрибуты:
        collection: Qdrant-коллекция для навыков.
        encoder: SentenceTransformer для создания embeddings.
    """

    def __init__(self, collection: Any, encoder: Any) -> None:
        self.collection = collection
        self.encoder = encoder

    def _encode(self, text: str) -> List[float]:
        """Создаёт embedding для текста через encoder."""
        raw = self.encoder.encode(text)
        if hasattr(raw, "tolist"):
            return raw.tolist()
        return list(raw)

    def _build_skill_document(self, goal: str, steps: List[str],
                              examples: List[str], constraints: List[str]) -> str:
        """
        Формирует текстовое представление навыка для индексации.

        Зачем: embedding строится по объединённому тексту цели, шагов,
        примеров и ограничений — это даёт более точный семантический поиск,
        чем индексация только по цели.
        """
        parts = [goal]
        if steps:
            parts.append("Шаги: " + "; ".join(steps))
        if examples:
            parts.append("Примеры: " + "; ".join(examples))
        if constraints:
            parts.append("Ограничения: " + "; ".join(constraints))
        return " | ".join(parts)

    def create_skill(
        self,
        goal: str,
        steps: Optional[List[str]] = None,
        examples: Optional[List[str]] = None,
        constraints: Optional[List[str]] = None,
        sources: Optional[List[str]] = None,
        confidence: Optional[float] = None,
        tags: Optional[List[str]] = None,
        model_name: Optional[str] = None,
        workspace_id: Optional[str] = None,
    ) -> Dict[str, Any]:
        """
        Создаёт новый навык и индексирует его в Qdrant.

        Возвращает dict с id, version, status.
        """
        skill_id = f"skill-{uuid.uuid4().hex[:12]}"
        now = datetime.now(timezone.utc).isoformat()
        actual_confidence = confidence if confidence is not None else settings.SKILL_CONFIDENCE_DEFAULT

        # Формируем документ для индексации
        document = self._build_skill_document(
            goal, steps or [], examples or [], constraints or []
        )
        embedding = self._encode(document)

        # Метаданные навыка — все структурированные поля хранятся в Qdrant payload
        metadata: Dict[str, Any] = {
            "type": "skill",
            "goal": goal,
            "steps": json.dumps(steps or [], ensure_ascii=False),
            "examples": json.dumps(examples or [], ensure_ascii=False),
            "constraints": json.dumps(constraints or [], ensure_ascii=False),
            "sources": json.dumps(sources or [], ensure_ascii=False),
            "confidence": actual_confidence,
            "version": 1,
            "tags": json.dumps(tags or [], ensure_ascii=False),
            "status": SKILL_STATUS_ACTIVE,
            "model_name": model_name or "",
            "workspace_id": workspace_id or "",
            "usage_count": 0,
            "created_at": now,
            "updated_at": now,
            # canonical_id для версионирования: все версии одного навыка
            # связаны через один canonical_id
            "canonical_id": skill_id,
        }

        self.collection.add(
            embeddings=[embedding],
            documents=[document],
            metadatas=[metadata],
            ids=[skill_id],
        )

        logger.info(
            "[SKILL-ENGINE] Навык создан: id=%s, goal=%s, confidence=%.2f",
            skill_id, goal[:80], actual_confidence,
        )

        return {
            "id": skill_id,
            "version": 1,
            "status": "ok",
            "message": "Навык создан",
        }

    def get_skill(self, skill_id: str) -> Optional[Dict[str, Any]]:
        """Получает навык по ID. Возвращает None если не найден."""
        result = self.collection.get(ids=[skill_id], include=["metadatas", "documents"])
        if not result["ids"]:
            return None

        meta = result["metadatas"][0]
        return self._meta_to_skill_item(skill_id, meta)

    def list_skills(
        self,
        workspace_id: Optional[str] = None,
        status: str = SKILL_STATUS_ACTIVE,
    ) -> List[Dict[str, Any]]:
        """Возвращает список навыков с фильтрацией по workspace и статусу."""
        where_filter: Dict[str, Any] = {"type": "skill", "status": status}
        if workspace_id:
            where_filter["workspace_id"] = workspace_id

        result = self.collection.get(
            where=where_filter,
            include=["metadatas", "documents"],
        )

        skills = []
        for i, skill_id in enumerate(result["ids"]):
            meta = result["metadatas"][i]
            skills.append(self._meta_to_skill_item(skill_id, meta))
        return skills

    def update_skill(
        self,
        skill_id: str,
        goal: Optional[str] = None,
        steps: Optional[List[str]] = None,
        examples: Optional[List[str]] = None,
        constraints: Optional[List[str]] = None,
        sources: Optional[List[str]] = None,
        confidence: Optional[float] = None,
        tags: Optional[List[str]] = None,
    ) -> Optional[Dict[str, Any]]:
        """
        Обновляет навык, создавая новую версию.

        Старая версия помечается как superseded, создаётся новая запись
        с увеличенным номером версии и тем же canonical_id.
        """
        current = self.collection.get(ids=[skill_id], include=["metadatas", "documents"])
        if not current["ids"]:
            return None

        old_meta = current["metadatas"][0]

        # Проверяем, что навык не удалён
        if old_meta.get("status") == SKILL_STATUS_DELETED:
            return None

        # Помечаем текущую версию как superseded
        old_meta["status"] = SKILL_STATUS_SUPERSEDED
        old_meta["updated_at"] = datetime.now(timezone.utc).isoformat()
        self.collection.update(ids=[skill_id], metadatas=[old_meta])

        # Создаём новую версию
        new_version = int(old_meta.get("version", 1)) + 1
        new_id = f"skill-{uuid.uuid4().hex[:12]}"
        now = datetime.now(timezone.utc).isoformat()

        # Мержим поля: берём новые значения или сохраняем старые
        new_goal = goal if goal is not None else old_meta.get("goal", "")
        new_steps = steps if steps is not None else json.loads(old_meta.get("steps", "[]"))
        new_examples = examples if examples is not None else json.loads(old_meta.get("examples", "[]"))
        new_constraints = constraints if constraints is not None else json.loads(old_meta.get("constraints", "[]"))
        new_sources = sources if sources is not None else json.loads(old_meta.get("sources", "[]"))
        new_confidence = confidence if confidence is not None else float(old_meta.get("confidence", settings.SKILL_CONFIDENCE_DEFAULT))
        new_tags = tags if tags is not None else json.loads(old_meta.get("tags", "[]"))

        document = self._build_skill_document(new_goal, new_steps, new_examples, new_constraints)
        embedding = self._encode(document)

        new_meta: Dict[str, Any] = {
            "type": "skill",
            "goal": new_goal,
            "steps": json.dumps(new_steps, ensure_ascii=False),
            "examples": json.dumps(new_examples, ensure_ascii=False),
            "constraints": json.dumps(new_constraints, ensure_ascii=False),
            "sources": json.dumps(new_sources, ensure_ascii=False),
            "confidence": new_confidence,
            "version": new_version,
            "tags": json.dumps(new_tags, ensure_ascii=False),
            "status": SKILL_STATUS_ACTIVE,
            "model_name": old_meta.get("model_name", ""),
            "workspace_id": old_meta.get("workspace_id", ""),
            "usage_count": int(old_meta.get("usage_count", 0)),
            "created_at": old_meta.get("created_at", now),
            "updated_at": now,
            "canonical_id": old_meta.get("canonical_id", skill_id),
            "previous_version_id": skill_id,
        }

        self.collection.add(
            embeddings=[embedding],
            documents=[document],
            metadatas=[new_meta],
            ids=[new_id],
        )

        logger.info(
            "[SKILL-ENGINE] Навык обновлён: id=%s → %s, v%d → v%d",
            skill_id, new_id, new_version - 1, new_version,
        )

        return {
            "id": new_id,
            "version": new_version,
            "previous_version_id": skill_id,
            "status": "ok",
            "message": "Навык обновлён",
        }

    def delete_skill(self, skill_id: str) -> bool:
        """
        Мягкое удаление навыка (помечает как deleted, не удаляет из Qdrant).

        Зачем: сохраняем историю навыков для аудита и возможного восстановления.
        """
        current = self.collection.get(ids=[skill_id], include=["metadatas"])
        if not current["ids"]:
            return False

        meta = current["metadatas"][0]
        if meta.get("status") == SKILL_STATUS_DELETED:
            return False

        meta["status"] = SKILL_STATUS_DELETED
        meta["deleted_at"] = datetime.now(timezone.utc).isoformat()
        meta["updated_at"] = datetime.now(timezone.utc).isoformat()
        self.collection.update(ids=[skill_id], metadatas=[meta])

        logger.info("[SKILL-ENGINE] Навык удалён (soft): id=%s", skill_id)
        return True

    def search_skills(
        self,
        query: str,
        top_k: Optional[int] = None,
        min_confidence: Optional[float] = None,
        workspace_id: Optional[str] = None,
    ) -> List[Dict[str, Any]]:
        """
        Семантический поиск навыков по запросу.

        Ищет только активные навыки с confidence >= min_confidence.
        Возвращает список навыков, отсортированных по релевантности.
        """
        actual_top_k = top_k or settings.SKILL_SEARCH_TOP_K
        embedding = self._encode(query)

        where_filter: Dict[str, Any] = {"type": "skill", "status": SKILL_STATUS_ACTIVE}
        if workspace_id:
            where_filter["workspace_id"] = workspace_id

        try:
            result = self.collection.query(
                query_embeddings=[embedding],
                n_results=actual_top_k,
                where=where_filter,
                include=["metadatas", "documents", "distances"],
            )
        except Exception as exc:
            logger.error("[SKILL-ENGINE] Ошибка поиска навыков: %s", exc)
            return []

        skills = []
        ids_list = result.get("ids", [[]])[0]
        metas_list = result.get("metadatas", [[]])[0]
        distances_list = result.get("distances", [[]])[0]

        for i, skill_id in enumerate(ids_list):
            meta = metas_list[i] if i < len(metas_list) else {}

            # Фильтрация по минимальному confidence
            skill_confidence = float(meta.get("confidence", 0))
            threshold = min_confidence if min_confidence is not None else settings.SKILL_CONFIDENCE_MIN
            if skill_confidence < threshold:
                continue

            skill_item = self._meta_to_skill_item(skill_id, meta)
            # Добавляем relevance score (distance → similarity)
            distance = distances_list[i] if i < len(distances_list) else 1.0
            skill_item["relevance"] = round(max(0.0, 1.0 - float(distance)), 4)
            skills.append(skill_item)

        return skills

    def record_usage(self, skill_id: str) -> bool:
        """
        Фиксирует использование навыка: увеличивает usage_count и confidence.

        Зачем: навыки, которые часто применяются, получают более высокий
        confidence — это имитирует укрепление памяти при повторном использовании.
        """
        current = self.collection.get(ids=[skill_id], include=["metadatas"])
        if not current["ids"]:
            return False

        meta = current["metadatas"][0]
        if meta.get("status") != SKILL_STATUS_ACTIVE:
            return False

        usage = int(meta.get("usage_count", 0)) + 1
        meta["usage_count"] = usage

        # Плавное повышение confidence при использовании.
        # Формула: confidence += (1.0 - confidence) * 0.05
        # Это обеспечивает быстрый рост при низком confidence
        # и замедление при приближении к 1.0
        current_confidence = float(meta.get("confidence", settings.SKILL_CONFIDENCE_DEFAULT))
        confidence_boost = 0.05
        new_confidence = min(1.0, current_confidence + (1.0 - current_confidence) * confidence_boost)
        meta["confidence"] = round(new_confidence, 4)
        meta["updated_at"] = datetime.now(timezone.utc).isoformat()

        self.collection.update(ids=[skill_id], metadatas=[meta])

        logger.info(
            "[SKILL-ENGINE] Навык использован: id=%s, usage=%d, confidence=%.4f",
            skill_id, usage, new_confidence,
        )
        return True

    def create_from_dialog(
        self,
        dialog_text: str,
        model_name: Optional[str] = None,
        workspace_id: Optional[str] = None,
    ) -> Dict[str, Any]:
        """
        Извлекает навык из текста диалога (Eternal RAG: раздел 7).

        Используется эвристический подход для извлечения структуры навыка:
        - Цель — первое предложение или инструкция.
        - Шаги — нумерованные или маркированные пункты.
        - Примеры — строки после ключевых слов «например», «пример».

        Полноценное извлечение навыков через LLM — задача Learning Engine (PR 4).
        """
        lines = dialog_text.strip().split("\n")
        goal = lines[0].strip() if lines else dialog_text[:200]

        steps: List[str] = []
        examples: List[str] = []
        constraints: List[str] = []

        for line in lines[1:]:
            stripped = line.strip()
            if not stripped:
                continue
            # Нумерованные шаги: "1.", "2.", "шаг 1", "- "
            if (stripped[:2].replace(".", "").isdigit()
                    or stripped.lower().startswith("шаг")
                    or stripped.startswith("- ")):
                steps.append(stripped.lstrip("0123456789.-) ").strip())
            # Примеры
            elif any(kw in stripped.lower() for kw in ["например", "пример", "example"]):
                examples.append(stripped)
            # Ограничения
            elif any(kw in stripped.lower() for kw in ["нельзя", "ограничение", "не допускается", "запрещено"]):
                constraints.append(stripped)

        return self.create_skill(
            goal=goal,
            steps=steps,
            examples=examples,
            constraints=constraints,
            sources=["dialog"],
            model_name=model_name,
            workspace_id=workspace_id,
        )

    def _meta_to_skill_item(self, skill_id: str, meta: Dict[str, Any]) -> Dict[str, Any]:
        """Преобразует метаданные Qdrant в структуру SkillItem для API."""
        return {
            "id": skill_id,
            "goal": meta.get("goal", ""),
            "steps": json.loads(meta.get("steps", "[]")),
            "examples": json.loads(meta.get("examples", "[]")),
            "constraints": json.loads(meta.get("constraints", "[]")),
            "sources": json.loads(meta.get("sources", "[]")),
            "confidence": float(meta.get("confidence", settings.SKILL_CONFIDENCE_DEFAULT)),
            "version": int(meta.get("version", 1)),
            "tags": json.loads(meta.get("tags", "[]")),
            "status": meta.get("status", SKILL_STATUS_ACTIVE),
            "model_name": meta.get("model_name", ""),
            "workspace_id": meta.get("workspace_id", ""),
            "usage_count": int(meta.get("usage_count", 0)),
            "created_at": meta.get("created_at", ""),
            "updated_at": meta.get("updated_at", ""),
        }
