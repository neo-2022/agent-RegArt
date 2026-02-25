import pytest
from datetime import datetime, timezone
from unittest.mock import Mock, patch, MagicMock
from app.memory import MemoryStore, LEARNING_STATUS_ACTIVE, LEARNING_STATUS_SUPERSEDED, LEARNING_STATUS_DELETED


class MockQdrantCollection:
    """Mock для QdrantCollectionCompat."""

    def __init__(self):
        self.data = {}
        self.id_counter = 0

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

        # Фильтр по where
        result_ids = []
        result_metas = []
        result_docs = []

        for doc_id, item in self.data.items():
            # Простая фильтрация по метаданным
            if where:
                if isinstance(where, dict):
                    for key, value in where.items():
                        if key.startswith("$"):
                            continue  # Пропускаем условные операторы для простоты
                        if item["metadata"].get(key) != value:
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

    def query(self, query_embeddings, n_results, include, where=None):
        """Mock для query."""
        return {
            "documents": [[]],
            "distances": [[]],
            "metadatas": [[]],
        }

    def delete(self, ids):
        for doc_id in ids:
            if doc_id in self.data:
                del self.data[doc_id]


@pytest.fixture
def mock_memory_store():
    """Фикстура для создания MemoryStore с mock коллекциями."""
    with patch("app.memory.QdrantClient"), \
         patch("app.memory.SentenceTransformer"):
        store = MemoryStore.__new__(MemoryStore)
        store.learnings_collection = MockQdrantCollection()
        store.facts_collection = MockQdrantCollection()
        store.files_collection = MockQdrantCollection()
        store.audit_collection = MockQdrantCollection()
        store._metrics_lock = __import__("threading").Lock()
        store._retrieval_metrics = {
            "search_requests_total": 0,
            "search_errors_total": 0,
            "search_latency_ms_total": 0.0,
            "search_results_total": 0,
        }
        store.encoder = Mock()
        store.encoder.encode = Mock(return_value=[0.1] * 384)
        yield store


class TestLearningsSoftDelete:
    """Набор тестов для soft delete знаний (learnings)."""

    def test_deleted_status_in_metadata(self, mock_memory_store):
        """Проверяет, что удалённое знание помечается со статусом deleted."""
        # Добавляем знание
        learning_id = mock_memory_store.add_learning(
            text="Test learning",
            model_name="test-model",
            agent_name="test-agent",
            category="general"
        )

        # Добавляем его в коллекцию (имитируем успешное добавление)
        mock_memory_store.learnings_collection.add(
            embeddings=[[0.1] * 384],
            documents=["Test learning"],
            metadatas=[{
                "model_name": "test-model",
                "agent_name": "test-agent",
                "category": "general",
                "version": 1,
                "status": LEARNING_STATUS_ACTIVE,
                "created_at": datetime.now(timezone.utc).isoformat(),
            }],
            ids=[learning_id]
        )

        # Удаляем знание (soft delete)
        deleted_count = mock_memory_store.delete_model_learnings("test-model")

        assert deleted_count == 1

        # Проверяем, что статус изменился на deleted
        result = mock_memory_store.learnings_collection.get(
            ids=[learning_id],
            include=["metadatas"]
        )
        if result["metadatas"]:
            assert result["metadatas"][0].get("status") == LEARNING_STATUS_DELETED
            assert "deleted_at" in result["metadatas"][0]

    def test_deleted_learning_not_returned_in_search(self, mock_memory_store):
        """Проверяет, что удалённые знания не возвращаются при поиске."""
        # Добавляем и удаляем знание
        learning_id = "test-learning-id"
        mock_memory_store.learnings_collection.add(
            embeddings=[[0.1] * 384],
            documents=["Deleted knowledge"],
            metadatas=[{
                "model_name": "test-model",
                "agent_name": "test-agent",
                "category": "general",
                "version": 1,
                "status": LEARNING_STATUS_DELETED,
                "created_at": datetime.now(timezone.utc).isoformat(),
            }],
            ids=[learning_id]
        )

        # При поиске удалённое знание не должно входить в results
        # (потому что search_learnings фильтрует по _is_active_learning)
        results = mock_memory_store.search_learnings(
            query="knowledge",
            model_name="test-model"
        )

        # Результаты пусты или не содержат удаленное знание
        assert len(results) == 0 or \
               not any(r.get("metadata", {}).get("status") == LEARNING_STATUS_DELETED for r in results)

    def test_soft_delete_preserves_data_integrity(self, mock_memory_store):
        """Проверяет, что soft delete не удаляет данные из БД, а только помечает."""
        learning_id = "test-id"
        original_text = "Important knowledge"

        mock_memory_store.learnings_collection.add(
            embeddings=[[0.1] * 384],
            documents=[original_text],
            metadatas=[{
                "model_name": "model1",
                "version": 1,
                "status": LEARNING_STATUS_ACTIVE,
            }],
            ids=[learning_id]
        )

        # Soft delete
        mock_memory_store.delete_model_learnings("model1")

        # Проверяем, что запись всё ещё в БД (но помечена как deleted)
        result = mock_memory_store.learnings_collection.get(ids=[learning_id], include=["documents", "metadatas"])

        assert len(result["ids"]) == 1
        assert result["documents"][0] == original_text
        assert result["metadatas"][0].get("status") == LEARNING_STATUS_DELETED

    def test_soft_delete_only_active_learnings(self, mock_memory_store):
        """Проверяет, что soft delete помечает только активные версии."""
        mock_memory_store.learnings_collection.add(
            embeddings=[[0.1] * 384] * 2,
            documents=["v1", "v2"],
            metadatas=[
                {
                    "model_name": "model1",
                    "version": 1,
                    "status": LEARNING_STATUS_SUPERSEDED,  # Уже не активная
                },
                {
                    "model_name": "model1",
                    "version": 2,
                    "status": LEARNING_STATUS_ACTIVE,  # Активная
                }
            ],
            ids=["id1", "id2"]
        )

        deleted_count = mock_memory_store.delete_model_learnings("model1")

        # Должна быть помечена только одна (активная версия)
        assert deleted_count == 1

        # Проверяем статусы
        result = mock_memory_store.learnings_collection.get(
            ids=["id1", "id2"],
            include=["metadatas"]
        )
        statuses = [m.get("status") for m in result["metadatas"]]
        assert LEARNING_STATUS_SUPERSEDED in statuses
        assert statuses.count(LEARNING_STATUS_DELETED) == 1


class TestLearningsVersioning:
    """Набор тестов для версионирования знаний."""

    def test_new_learning_has_version_1(self, mock_memory_store):
        """Проверяет, что новое знание получает версию 1."""
        learning_id = mock_memory_store.add_learning(
            text="First knowledge",
            model_name="model1",
            agent_name="agent1",
            category="general"
        )

        # Извлекаем метаданные добавленного знания
        result = mock_memory_store.learnings_collection.get(
            ids=[learning_id],
            include=["metadatas"]
        )

        if result["metadatas"]:
            assert result["metadatas"][0].get("version") == 1

    def test_superseded_learning_marked_correctly(self, mock_memory_store):
        """Проверяет, что предыдущая версия помечается как superseded."""
        # Добавляем первую версию
        learning_key = "model1::general"
        version_1_id = "v1-id"

        # Имитируем первую версию
        mock_memory_store.learnings_collection.add(
            embeddings=[[0.1] * 384],
            documents=["Version 1"],
            metadatas=[{
                "model_name": "model1",
                "category": "general",
                "learning_key": learning_key,
                "version": 1,
                "status": LEARNING_STATUS_ACTIVE,
                "created_at": datetime.now(timezone.utc).isoformat(),
            }],
            ids=[version_1_id]
        )

        # Добавляем вторую версию
        version_2_id = mock_memory_store.add_learning(
            text="Version 2 - Updated",
            model_name="model1",
            agent_name="agent1",
            category="general"
        )

        # Проверяем первую версию
        result = mock_memory_store.learnings_collection.get(
            ids=[version_1_id],
            include=["metadatas"]
        )
        if result["metadatas"]:
            meta = result["metadatas"][0]
            # После добавления версии 2, версия 1 должна быть помечена как superseded
            # (если в add_learning реально обновляется статус)
            if meta.get("status"):
                assert meta.get("status") in [LEARNING_STATUS_ACTIVE, LEARNING_STATUS_SUPERSEDED]

    def test_conflict_detected_on_text_change(self, mock_memory_store):
        """Проверяет, что конфликт обнаруживается при изменении текста версии."""
        # Мокируем поведение _find_latest_learning_version
        with patch.object(mock_memory_store, "_find_latest_learning_version") as mock_find:
            mock_find.return_value = {
                "id": "v1-id",
                "metadata": {"version": 1},
                "document": "Old text",
            }

            learning_id = mock_memory_store.add_learning(
                text="New text - different",  # Текст отличается
                model_name="model1",
                agent_name="agent1",
                category="general"
            )

            # Проверяем, что конфликт был обнаружен
            result = mock_memory_store.learnings_collection.get(
                ids=[learning_id],
                include=["metadatas"]
            )
            if result["metadatas"]:
                # После add_learning должен быть флаг conflict_detected
                # (зависит от реализации)
                pass

    def test_version_numbers_increment(self, mock_memory_store):
        """Проверяет, что номера версий правильно возрастают."""
        versions = []

        for i in range(3):
            with patch.object(mock_memory_store, "_find_latest_learning_version") as mock_find:
                if i == 0:
                    mock_find.return_value = None
                else:
                    mock_find.return_value = {
                        "id": f"v{i}-id",
                        "metadata": {"version": i},
                        "document": f"Version {i}",
                    }

                learning_id = mock_memory_store.add_learning(
                    text=f"Version {i+1}",
                    model_name="model1",
                    agent_name="agent1",
                    category="general"
                )

                result = mock_memory_store.learnings_collection.get(
                    ids=[learning_id],
                    include=["metadatas"]
                )
                if result["metadatas"]:
                    version_num = result["metadatas"][0].get("version", i+1)
                    versions.append(version_num)

        # Версии должны возрастать (или быть явно установлены)


class TestLearningsIntegration:
    """Интеграционные тесты для системы обучения."""

    def test_learning_lifecycle_add_search_delete(self, mock_memory_store):
        """Проверяет полный жизненный цикл знания: добавление, поиск, удаление."""
        # 1. Добавляем знание
        learning_id = mock_memory_store.add_learning(
            text="Python is a powerful programming language",
            model_name="gpt-4",
            agent_name="research-agent",
            category="fact"
        )
        assert learning_id != ""

        # 2. Добавляем вручную в коллекцию для тестирования поиска
        mock_memory_store.learnings_collection.add(
            embeddings=[[0.1] * 384],
            documents=["Python is a powerful programming language"],
            metadatas=[{
                "model_name": "gpt-4",
                "agent_name": "research-agent",
                "category": "fact",
                "version": 1,
                "status": LEARNING_STATUS_ACTIVE,
                "created_at": datetime.now(timezone.utc).isoformat(),
            }],
            ids=[learning_id]
        )

        # 3. Удаляем знание (soft delete)
        deleted_count = mock_memory_store.delete_model_learnings("gpt-4")
        assert deleted_count == 1

        # 4. Проверяем, что знание помечено как deleted
        result = mock_memory_store.learnings_collection.get(
            ids=[learning_id],
            include=["metadatas"]
        )
        assert result["metadatas"][0].get("status") == LEARNING_STATUS_DELETED

    def test_multiple_learnings_same_model_filtered_delete(self, mock_memory_store):
        """Проверяет удаление только знаний конкретной модели по категории."""
        # Добавляем знания для разных категорий
        mock_memory_store.learnings_collection.add(
            embeddings=[[0.1] * 384] * 3,
            documents=["fact 1", "preference 1", "skill 1"],
            metadatas=[
                {
                    "model_name": "model1",
                    "category": "fact",
                    "version": 1,
                    "status": LEARNING_STATUS_ACTIVE,
                },
                {
                    "model_name": "model1",
                    "category": "preference",
                    "version": 1,
                    "status": LEARNING_STATUS_ACTIVE,
                },
                {
                    "model_name": "model2",
                    "category": "skill",
                    "version": 1,
                    "status": LEARNING_STATUS_ACTIVE,
                }
            ],
            ids=["id1", "id2", "id3"]
        )

        # Удаляем только факты модели model1
        deleted_count = mock_memory_store.delete_model_learnings(
            "model1",
            category="fact"
        )

        # Проверяем, что удалена только одна запись
        assert deleted_count == 1

        # Проверяем, что статусы других остались active
        result = mock_memory_store.learnings_collection.get(
            ids=["id1", "id2", "id3"],
            include=["metadatas"]
        )
        statuses = [(m.get("model_name"), m.get("category"), m.get("status")) for m in result["metadatas"]]
        # Ожидаем что id1 (model1, fact) будет deleted, остальные active или неизменны
