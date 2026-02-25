"""
Тесты для Graph Engine (Eternal RAG: раздел 5.4).

Покрывает: CRUD связей, поиск соседей, BFS-обход графа,
валидация типов связей, создание противоречий.
"""

import pytest
from unittest.mock import Mock
from app.graph_engine import GraphEngine


class MockGraphCollection:
    """Mock для Qdrant-коллекции графа знаний."""

    def __init__(self):
        self.data = {}

    def count(self):
        return len(self.data)

    def add(self, embeddings, documents, metadatas, ids):
        for i, doc_id in enumerate(ids):
            self.data[doc_id] = {
                "embedding": embeddings[i] if i < len(embeddings) else [],
                "document": documents[i] if i < len(documents) else "",
                "metadata": metadatas[i] if i < len(metadatas) else {},
            }

    def update(self, ids, metadatas):
        for doc_id, metadata in zip(ids, metadatas):
            if doc_id in self.data:
                self.data[doc_id]["metadata"] = metadata

    def get(self, where=None, include=None, ids=None):
        """Возвращает записи по фильтру или ID."""
        include = include or []
        if ids:
            result_ids = []
            result_metas = []
            result_docs = []
            for doc_id in ids:
                if doc_id in self.data:
                    result_ids.append(doc_id)
                    if "metadatas" in include:
                        result_metas.append(self.data[doc_id]["metadata"])
                    if "documents" in include:
                        result_docs.append(self.data[doc_id]["document"])
            return {
                "ids": result_ids,
                "metadatas": result_metas if "metadatas" in include else [],
                "documents": result_docs if "documents" in include else [],
            }

        # Фильтр по where (плоский dict — неявный AND)
        result_ids = []
        result_metas = []
        result_docs = []

        for doc_id, item in self.data.items():
            if where and isinstance(where, dict):
                match = True
                for key, value in where.items():
                    if key.startswith("$"):
                        continue
                    if item["metadata"].get(key) != value:
                        match = False
                        break
                if not match:
                    continue
            result_ids.append(doc_id)
            if "metadatas" in include:
                result_metas.append(item["metadata"])
            if "documents" in include:
                result_docs.append(item["document"])

        return {
            "ids": result_ids,
            "metadatas": result_metas if "metadatas" in include else [],
            "documents": result_docs if "documents" in include else [],
        }

    def delete(self, ids):
        for doc_id in ids:
            if doc_id in self.data:
                del self.data[doc_id]


@pytest.fixture
def graph_engine():
    """Фикстура для создания GraphEngine с mock-коллекцией и encoder."""
    collection = MockGraphCollection()
    encoder = Mock()
    encoder.encode = Mock(return_value=[0.1] * 384)
    return GraphEngine(collection=collection, encoder=encoder)


class TestRelationshipCreate:
    """Тесты создания связей."""

    def test_create_returns_id_and_status(self, graph_engine):
        """Создание связи возвращает id и status=ok."""
        result = graph_engine.create_relationship(
            source_id="node-1",
            target_id="node-2",
            relationship_type="relates_to",
        )
        assert "id" in result
        assert result["status"] == "ok"
        assert result["id"].startswith("rel-")

    def test_create_stores_metadata(self, graph_engine):
        """Метаданные связи корректно сохраняются."""
        result = graph_engine.create_relationship(
            source_id="skill-1",
            target_id="fact-1",
            relationship_type="depends_on",
            source_type="skill",
            target_type="fact",
            workspace_id="ws-123",
        )
        rel = graph_engine.get_relationship(result["id"])
        assert rel is not None
        assert rel["source_id"] == "skill-1"
        assert rel["target_id"] == "fact-1"
        assert rel["relationship_type"] == "depends_on"
        assert rel["source_type"] == "skill"
        assert rel["target_type"] == "fact"
        assert rel["workspace_id"] == "ws-123"

    def test_create_with_custom_metadata(self, graph_engine):
        """Пользовательские метаданные сохраняются с префиксом meta_."""
        result = graph_engine.create_relationship(
            source_id="a",
            target_id="b",
            relationship_type="relates_to",
            metadata={"similarity": 0.95, "reason": "тематически связаны"},
        )
        rel = graph_engine.get_relationship(result["id"])
        assert rel["metadata"]["similarity"] == 0.95
        assert rel["metadata"]["reason"] == "тематически связаны"

    def test_create_invalid_type_raises_error(self, graph_engine):
        """Недопустимый тип связи вызывает ValueError."""
        with pytest.raises(ValueError, match="Недопустимый тип связи"):
            graph_engine.create_relationship(
                source_id="a",
                target_id="b",
                relationship_type="invalid_type",
            )

    def test_create_self_loop_raises_error(self, graph_engine):
        """Связь узла с самим собой вызывает ValueError."""
        with pytest.raises(ValueError, match="самим собой"):
            graph_engine.create_relationship(
                source_id="node-1",
                target_id="node-1",
                relationship_type="relates_to",
            )

    def test_create_all_valid_types(self, graph_engine):
        """Все допустимые типы связей создаются успешно."""
        valid_types = ["relates_to", "contradicts", "depends_on", "supersedes", "derived_from"]
        for i, rel_type in enumerate(valid_types):
            result = graph_engine.create_relationship(
                source_id=f"src-{i}",
                target_id=f"tgt-{i}",
                relationship_type=rel_type,
            )
            assert result["status"] == "ok"


class TestRelationshipGet:
    """Тесты получения связи по ID."""

    def test_get_existing_relationship(self, graph_engine):
        """Получение существующей связи."""
        result = graph_engine.create_relationship(
            source_id="a", target_id="b", relationship_type="relates_to",
        )
        rel = graph_engine.get_relationship(result["id"])
        assert rel is not None
        assert rel["source_id"] == "a"
        assert rel["target_id"] == "b"

    def test_get_nonexistent_returns_none(self, graph_engine):
        """Несуществующий ID возвращает None."""
        assert graph_engine.get_relationship("nonexistent") is None


class TestRelationshipDelete:
    """Тесты удаления связей."""

    def test_delete_existing_returns_true(self, graph_engine):
        """Удаление существующей связи возвращает True."""
        result = graph_engine.create_relationship(
            source_id="a", target_id="b", relationship_type="relates_to",
        )
        assert graph_engine.delete_relationship(result["id"]) is True

    def test_delete_removes_from_collection(self, graph_engine):
        """Удалённая связь больше не доступна через get."""
        result = graph_engine.create_relationship(
            source_id="a", target_id="b", relationship_type="relates_to",
        )
        graph_engine.delete_relationship(result["id"])
        assert graph_engine.get_relationship(result["id"]) is None

    def test_delete_nonexistent_returns_false(self, graph_engine):
        """Удаление несуществующей связи возвращает False."""
        assert graph_engine.delete_relationship("nonexistent") is False


class TestRelationshipList:
    """Тесты списка связей."""

    def test_list_all_relationships(self, graph_engine):
        """Список содержит все созданные связи."""
        graph_engine.create_relationship(
            source_id="a", target_id="b", relationship_type="relates_to",
        )
        graph_engine.create_relationship(
            source_id="c", target_id="d", relationship_type="depends_on",
        )
        rels = graph_engine.list_relationships()
        assert len(rels) == 2

    def test_list_empty_returns_empty(self, graph_engine):
        """Пустая коллекция → пустой список."""
        rels = graph_engine.list_relationships()
        assert rels == []

    def test_list_filters_by_type(self, graph_engine):
        """Фильтрация по типу связи."""
        graph_engine.create_relationship(
            source_id="a", target_id="b", relationship_type="relates_to",
        )
        graph_engine.create_relationship(
            source_id="c", target_id="d", relationship_type="contradicts",
        )
        rels = graph_engine.list_relationships(relationship_type="contradicts")
        assert len(rels) == 1
        assert rels[0]["relationship_type"] == "contradicts"

    def test_list_filters_by_workspace(self, graph_engine):
        """Фильтрация по workspace_id."""
        graph_engine.create_relationship(
            source_id="a", target_id="b", relationship_type="relates_to",
            workspace_id="ws-1",
        )
        graph_engine.create_relationship(
            source_id="c", target_id="d", relationship_type="relates_to",
            workspace_id="ws-2",
        )
        rels = graph_engine.list_relationships(workspace_id="ws-1")
        assert len(rels) == 1
        assert rels[0]["workspace_id"] == "ws-1"


class TestGetNeighbors:
    """Тесты поиска соседей узла."""

    def test_finds_outgoing_relationships(self, graph_engine):
        """Находит связи, где node_id — источник."""
        graph_engine.create_relationship(
            source_id="center", target_id="neighbor-1", relationship_type="relates_to",
        )
        neighbors = graph_engine.get_neighbors("center")
        assert len(neighbors) == 1
        assert neighbors[0]["target_id"] == "neighbor-1"

    def test_finds_incoming_relationships(self, graph_engine):
        """Находит связи, где node_id — цель."""
        graph_engine.create_relationship(
            source_id="neighbor-1", target_id="center", relationship_type="depends_on",
        )
        neighbors = graph_engine.get_neighbors("center")
        assert len(neighbors) == 1
        assert neighbors[0]["source_id"] == "neighbor-1"

    def test_finds_both_directions(self, graph_engine):
        """Находит связи в обоих направлениях."""
        graph_engine.create_relationship(
            source_id="center", target_id="out-1", relationship_type="relates_to",
        )
        graph_engine.create_relationship(
            source_id="in-1", target_id="center", relationship_type="depends_on",
        )
        neighbors = graph_engine.get_neighbors("center")
        assert len(neighbors) == 2

    def test_no_neighbors_returns_empty(self, graph_engine):
        """Узел без связей → пустой список."""
        neighbors = graph_engine.get_neighbors("isolated-node")
        assert neighbors == []

    def test_filters_by_relationship_type(self, graph_engine):
        """Фильтрация соседей по типу связи."""
        graph_engine.create_relationship(
            source_id="center", target_id="rel-1", relationship_type="relates_to",
        )
        graph_engine.create_relationship(
            source_id="center", target_id="dep-1", relationship_type="depends_on",
        )
        neighbors = graph_engine.get_neighbors("center", relationship_type="depends_on")
        assert len(neighbors) == 1
        assert neighbors[0]["relationship_type"] == "depends_on"

    def test_respects_max_results(self, graph_engine):
        """Ограничение по количеству результатов."""
        for i in range(10):
            graph_engine.create_relationship(
                source_id="hub", target_id=f"spoke-{i}", relationship_type="relates_to",
            )
        neighbors = graph_engine.get_neighbors("hub", max_results=3)
        assert len(neighbors) <= 3


class TestGraphTraversal:
    """Тесты BFS-обхода графа."""

    def test_traverse_single_node(self, graph_engine):
        """Обход от изолированного узла → только стартовый узел."""
        result = graph_engine.traverse(start_node_id="isolated")
        assert result["start_node_id"] == "isolated"
        assert len(result["nodes"]) == 1
        assert result["nodes"][0]["node_id"] == "isolated"
        assert result["nodes"][0]["depth"] == 0
        assert result["total_relationships"] == 0

    def test_traverse_depth_1(self, graph_engine):
        """Обход на глубину 1: находит прямых соседей."""
        graph_engine.create_relationship(
            source_id="root", target_id="child-1", relationship_type="relates_to",
        )
        graph_engine.create_relationship(
            source_id="root", target_id="child-2", relationship_type="relates_to",
        )
        result = graph_engine.traverse(start_node_id="root", max_depth=1)
        node_ids = [n["node_id"] for n in result["nodes"]]
        assert "root" in node_ids
        assert "child-1" in node_ids
        assert "child-2" in node_ids
        assert len(result["nodes"]) == 3

    def test_traverse_depth_2(self, graph_engine):
        """Обход на глубину 2: находит узлы через промежуточный."""
        graph_engine.create_relationship(
            source_id="root", target_id="middle", relationship_type="relates_to",
        )
        graph_engine.create_relationship(
            source_id="middle", target_id="leaf", relationship_type="relates_to",
        )
        result = graph_engine.traverse(start_node_id="root", max_depth=2)
        node_ids = [n["node_id"] for n in result["nodes"]]
        assert "root" in node_ids
        assert "middle" in node_ids
        assert "leaf" in node_ids

    def test_traverse_respects_max_depth(self, graph_engine):
        """Обход не заходит глубже max_depth."""
        # Цепочка: root → a → b → c
        graph_engine.create_relationship(
            source_id="root", target_id="a", relationship_type="relates_to",
        )
        graph_engine.create_relationship(
            source_id="a", target_id="b", relationship_type="relates_to",
        )
        graph_engine.create_relationship(
            source_id="b", target_id="c", relationship_type="relates_to",
        )
        result = graph_engine.traverse(start_node_id="root", max_depth=1)
        node_ids = [n["node_id"] for n in result["nodes"]]
        assert "root" in node_ids
        assert "a" in node_ids
        # b и c не должны быть достижимы при max_depth=1
        assert "b" not in node_ids
        assert "c" not in node_ids

    def test_traverse_no_cycles(self, graph_engine):
        """Обход не зацикливается при наличии циклов."""
        # Цикл: a → b → c → a
        graph_engine.create_relationship(
            source_id="a", target_id="b", relationship_type="relates_to",
        )
        graph_engine.create_relationship(
            source_id="b", target_id="c", relationship_type="relates_to",
        )
        graph_engine.create_relationship(
            source_id="c", target_id="a", relationship_type="relates_to",
        )
        result = graph_engine.traverse(start_node_id="a", max_depth=5)
        # Каждый узел должен появиться только один раз
        node_ids = [n["node_id"] for n in result["nodes"]]
        assert len(node_ids) == len(set(node_ids))

    def test_traverse_respects_max_nodes(self, graph_engine):
        """Обход не превышает max_nodes."""
        # Звезда: center → spoke-0..9
        for i in range(10):
            graph_engine.create_relationship(
                source_id="center", target_id=f"spoke-{i}", relationship_type="relates_to",
            )
        result = graph_engine.traverse(start_node_id="center", max_depth=1, max_nodes=5)
        assert len(result["nodes"]) <= 5

    def test_traverse_returns_relationships(self, graph_engine):
        """Обход возвращает связи каждого узла."""
        graph_engine.create_relationship(
            source_id="root", target_id="child", relationship_type="depends_on",
        )
        result = graph_engine.traverse(start_node_id="root", max_depth=1)
        root_node = next(n for n in result["nodes"] if n["node_id"] == "root")
        assert len(root_node["relationships"]) == 1
        assert root_node["relationships"][0]["relationship_type"] == "depends_on"

    def test_traverse_max_depth_reached(self, graph_engine):
        """max_depth_reached отражает реальную максимальную глубину."""
        graph_engine.create_relationship(
            source_id="root", target_id="child", relationship_type="relates_to",
        )
        result = graph_engine.traverse(start_node_id="root", max_depth=3)
        assert result["max_depth_reached"] >= 0
        assert result["max_depth_reached"] <= 3


class TestContradictionRelationship:
    """Тесты создания связей-противоречий."""

    def test_creates_contradicts_type(self, graph_engine):
        """create_contradiction_relationship создаёт связь типа 'contradicts'."""
        result = graph_engine.create_contradiction_relationship(
            new_id="new-fact",
            existing_id="old-fact",
            similarity=0.92,
        )
        assert result["status"] == "ok"
        rel = graph_engine.get_relationship(result["id"])
        assert rel["relationship_type"] == "contradicts"
        assert rel["source_id"] == "new-fact"
        assert rel["target_id"] == "old-fact"

    def test_stores_similarity_in_metadata(self, graph_engine):
        """Similarity сохраняется в метаданных связи."""
        result = graph_engine.create_contradiction_relationship(
            new_id="a",
            existing_id="b",
            similarity=0.88,
        )
        rel = graph_engine.get_relationship(result["id"])
        assert rel["metadata"]["similarity"] == 0.88

    def test_contradiction_with_workspace(self, graph_engine):
        """Противоречие с привязкой к workspace."""
        result = graph_engine.create_contradiction_relationship(
            new_id="x",
            existing_id="y",
            similarity=0.95,
            workspace_id="ws-1",
        )
        rel = graph_engine.get_relationship(result["id"])
        assert rel["workspace_id"] == "ws-1"


class TestGraphLifecycle:
    """Интеграционные тесты жизненного цикла графа."""

    def test_full_lifecycle(self, graph_engine):
        """Полный цикл: создание связей → список → соседи → обход → удаление."""
        # 1. Создание связей
        r1 = graph_engine.create_relationship(
            source_id="doc-1", target_id="doc-2", relationship_type="relates_to",
        )
        graph_engine.create_relationship(
            source_id="doc-2", target_id="doc-3", relationship_type="depends_on",
        )
        graph_engine.create_relationship(
            source_id="doc-1", target_id="doc-3", relationship_type="derived_from",
        )

        # 2. Список
        all_rels = graph_engine.list_relationships()
        assert len(all_rels) == 3

        # 3. Соседи doc-1
        neighbors = graph_engine.get_neighbors("doc-1")
        neighbor_ids = set()
        for n in neighbors:
            neighbor_ids.add(n["source_id"])
            neighbor_ids.add(n["target_id"])
        assert "doc-2" in neighbor_ids
        assert "doc-3" in neighbor_ids

        # 4. Обход от doc-1
        traversal = graph_engine.traverse(start_node_id="doc-1", max_depth=2)
        traversed_ids = {n["node_id"] for n in traversal["nodes"]}
        assert "doc-1" in traversed_ids
        assert "doc-2" in traversed_ids
        assert "doc-3" in traversed_ids

        # 5. Удаление
        assert graph_engine.delete_relationship(r1["id"]) is True
        remaining = graph_engine.list_relationships()
        assert len(remaining) == 2
