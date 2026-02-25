"""
Graph Engine — движок графа знаний (Eternal RAG: раздел 5.4).

Управляет связями между знаниями агента: документами, навыками, фактами.
Связи хранятся в Qdrant-коллекции как записи с метаданными source_id/target_id.

Типы связей:
  - relates_to: общая тематическая связь
  - contradicts: противоречие между знаниями
  - depends_on: зависимость (A зависит от B)
  - supersedes: замена (A заменяет B)
  - derived_from: производное (A получено из B)

Обход графа (traversal) выполняется через BFS с ограничением глубины.

Будущая интеграция: при подключении Neo4j связи будут дублироваться
в Neo4j для более эффективного обхода графов больших размеров.
"""

from __future__ import annotations

import logging
import uuid
from collections import deque
from datetime import datetime, timezone
from typing import Any, Dict, List, Optional, Set

from .config import settings

logger = logging.getLogger(__name__)


class GraphEngine:
    """
    Движок графа знаний.

    Хранит связи между узлами (знаниями) в Qdrant-коллекции.
    Поддерживает CRUD операции, поиск соседей и обход графа.

    Атрибуты:
        collection: Qdrant-коллекция для связей.
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

    def create_relationship(
        self,
        source_id: str,
        target_id: str,
        relationship_type: str,
        source_type: str = "knowledge",
        target_type: str = "knowledge",
        metadata: Optional[Dict[str, Any]] = None,
        workspace_id: Optional[str] = None,
    ) -> Dict[str, Any]:
        """
        Создаёт связь между двумя узлами графа знаний.

        Валидирует тип связи по списку допустимых типов из конфигурации.
        Embedding строится по описательному тексту связи для возможности
        семантического поиска связей.
        """
        # Валидация типа связи
        allowed_types = settings.GRAPH_RELATIONSHIP_TYPES
        if relationship_type not in allowed_types:
            raise ValueError(
                f"Недопустимый тип связи: '{relationship_type}'. "
                f"Допустимые: {', '.join(allowed_types)}"
            )

        # Нельзя создавать связь узла с самим собой
        if source_id == target_id:
            raise ValueError("Нельзя создать связь узла с самим собой")

        rel_id = str(uuid.uuid4())
        now = datetime.now(timezone.utc).isoformat()

        # Описательный текст для embedding: позволяет искать связи семантически
        description = f"{source_type}:{source_id} {relationship_type} {target_type}:{target_id}"
        embedding = self._encode(description)

        rel_metadata: Dict[str, Any] = {
            "type": "relationship",
            "source_id": source_id,
            "target_id": target_id,
            "relationship_type": relationship_type,
            "source_type": source_type,
            "target_type": target_type,
            "workspace_id": workspace_id or "",
            "created_at": now,
        }
        # Добавляем пользовательские метаданные с префиксом для избежания коллизий
        if metadata:
            for key, value in metadata.items():
                if isinstance(value, (str, int, float, bool)):
                    rel_metadata[f"meta_{key}"] = value

        self.collection.add(
            embeddings=[embedding],
            documents=[description],
            metadatas=[rel_metadata],
            ids=[rel_id],
        )

        logger.info(
            "[GRAPH-ENGINE] Связь создана: %s (%s) -[%s]-> (%s) %s",
            source_id, source_type, relationship_type, target_type, target_id,
        )

        return {
            "id": rel_id,
            "status": "ok",
            "message": "Связь создана",
        }

    def get_relationship(self, rel_id: str) -> Optional[Dict[str, Any]]:
        """Получает связь по ID."""
        result = self.collection.get(ids=[rel_id], include=["metadatas"])
        if not result["ids"]:
            return None
        return self._meta_to_relationship(rel_id, result["metadatas"][0])

    def delete_relationship(self, rel_id: str) -> bool:
        """Удаляет связь из графа знаний."""
        result = self.collection.get(ids=[rel_id], include=["metadatas"])
        if not result["ids"]:
            return False

        self.collection.delete(ids=[rel_id])
        logger.info("[GRAPH-ENGINE] Связь удалена: id=%s", rel_id)
        return True

    def list_relationships(
        self,
        workspace_id: Optional[str] = None,
        relationship_type: Optional[str] = None,
    ) -> List[Dict[str, Any]]:
        """Возвращает список связей с фильтрацией."""
        where_filter: Dict[str, Any] = {"type": "relationship"}
        if workspace_id:
            where_filter["workspace_id"] = workspace_id
        if relationship_type:
            where_filter["relationship_type"] = relationship_type

        result = self.collection.get(
            where=where_filter,
            include=["metadatas"],
        )

        relationships = []
        for i, rel_id in enumerate(result["ids"]):
            relationships.append(
                self._meta_to_relationship(rel_id, result["metadatas"][i])
            )
        return relationships

    def get_neighbors(
        self,
        node_id: str,
        relationship_type: Optional[str] = None,
        max_results: Optional[int] = None,
    ) -> List[Dict[str, Any]]:
        """
        Находит все связи, в которых node_id участвует как source или target.

        Зачем: для обхода графа нужно найти все связанные узлы.
        Qdrant не поддерживает OR-фильтры по разным полям, поэтому
        выполняются два запроса: по source_id и target_id.
        """
        limit = max_results or settings.GRAPH_MAX_NEIGHBORS
        results: List[Dict[str, Any]] = []

        # Поиск связей, где node_id — источник
        source_filter: Dict[str, Any] = {
            "type": "relationship",
            "source_id": node_id,
        }
        if relationship_type:
            source_filter["relationship_type"] = relationship_type

        source_result = self.collection.get(
            where=source_filter, include=["metadatas"],
        )
        for i, rel_id in enumerate(source_result["ids"]):
            results.append(
                self._meta_to_relationship(rel_id, source_result["metadatas"][i])
            )

        # Поиск связей, где node_id — цель
        target_filter: Dict[str, Any] = {
            "type": "relationship",
            "target_id": node_id,
        }
        if relationship_type:
            target_filter["relationship_type"] = relationship_type

        target_result = self.collection.get(
            where=target_filter, include=["metadatas"],
        )
        for i, rel_id in enumerate(target_result["ids"]):
            # Избегаем дубликатов (если source_id == target_id, что невозможно,
            # но для надёжности проверяем)
            if rel_id not in {r["id"] for r in results}:
                results.append(
                    self._meta_to_relationship(rel_id, target_result["metadatas"][i])
                )

        return results[:limit]

    def traverse(
        self,
        start_node_id: str,
        max_depth: Optional[int] = None,
        relationship_types: Optional[List[str]] = None,
        max_nodes: int = 50,
    ) -> Dict[str, Any]:
        """
        Обход графа знаний в ширину (BFS) от стартового узла.

        Возвращает структуру с узлами и их связями на каждом уровне глубины.
        Ограничен по глубине (max_depth) и количеству узлов (max_nodes).

        Зачем: при retrieval нужно находить все связанные знания,
        чтобы формировать полный контекст для ответа модели.
        """
        depth_limit = min(
            max_depth or settings.GRAPH_MAX_DEPTH,
            settings.GRAPH_MAX_DEPTH,
        )

        visited: Set[str] = set()
        # Очередь BFS: (node_id, текущая_глубина)
        queue: deque[tuple[str, int]] = deque([(start_node_id, 0)])
        visited.add(start_node_id)

        nodes: List[Dict[str, Any]] = []
        total_relationships = 0
        max_depth_reached = 0

        while queue and len(nodes) < max_nodes:
            current_id, current_depth = queue.popleft()

            if current_depth > depth_limit:
                continue

            # Получаем связи текущего узла
            neighbors = self.get_neighbors(
                current_id,
                relationship_type=relationship_types[0] if relationship_types and len(relationship_types) == 1 else None,
            )

            # Фильтруем по типам связей, если указано несколько
            if relationship_types and len(relationship_types) > 1:
                neighbors = [
                    n for n in neighbors
                    if n.get("relationship_type") in relationship_types
                ]

            node_entry = {
                "node_id": current_id,
                "depth": current_depth,
                "relationships": neighbors,
            }
            nodes.append(node_entry)
            total_relationships += len(neighbors)
            max_depth_reached = max(max_depth_reached, current_depth)

            # Добавляем соседей в очередь для следующего уровня
            if current_depth < depth_limit:
                for rel in neighbors:
                    # Определяем ID соседнего узла
                    neighbor_id = (
                        rel["target_id"]
                        if rel["source_id"] == current_id
                        else rel["source_id"]
                    )
                    if neighbor_id not in visited and len(nodes) + len(queue) < max_nodes:
                        visited.add(neighbor_id)
                        queue.append((neighbor_id, current_depth + 1))

        return {
            "start_node_id": start_node_id,
            "nodes": nodes,
            "total_relationships": total_relationships,
            "max_depth_reached": max_depth_reached,
        }

    def create_contradiction_relationship(
        self,
        new_id: str,
        existing_id: str,
        similarity: float,
        workspace_id: Optional[str] = None,
    ) -> Dict[str, Any]:
        """
        Создаёт связь типа 'contradicts' между противоречащими знаниями.

        Используется при обнаружении противоречий в Learning Engine
        (Eternal RAG: раздел 8).
        """
        return self.create_relationship(
            source_id=new_id,
            target_id=existing_id,
            relationship_type="contradicts",
            source_type="knowledge",
            target_type="knowledge",
            metadata={"similarity": similarity},
            workspace_id=workspace_id,
        )

    def _meta_to_relationship(self, rel_id: str, meta: Dict[str, Any]) -> Dict[str, Any]:
        """Преобразует метаданные Qdrant в структуру RelationshipItem для API."""
        # Собираем пользовательские метаданные (с префиксом meta_)
        extra_metadata = {
            k.replace("meta_", "", 1): v
            for k, v in meta.items()
            if k.startswith("meta_")
        }
        return {
            "id": rel_id,
            "source_id": meta.get("source_id", ""),
            "target_id": meta.get("target_id", ""),
            "relationship_type": meta.get("relationship_type", ""),
            "source_type": meta.get("source_type", "knowledge"),
            "target_type": meta.get("target_type", "knowledge"),
            "metadata": extra_metadata,
            "workspace_id": meta.get("workspace_id", ""),
            "created_at": meta.get("created_at", ""),
        }
