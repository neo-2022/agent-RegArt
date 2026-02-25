"""
Тесты для Skill Engine (Eternal RAG: раздел 5.3).

Покрывает: CRUD, версионирование, confidence scoring, семантический поиск,
создание навыков из диалога, запись использования.
"""

import uuid

import pytest
from unittest.mock import Mock
from app.skill_engine import SkillEngine, SKILL_STATUS_ACTIVE, SKILL_STATUS_SUPERSEDED, SKILL_STATUS_DELETED


class MockSkillCollection:
    """Mock для Qdrant-коллекции навыков."""

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

    def query(self, query_embeddings, n_results, include, where=None):
        """Mock для семантического поиска — возвращает все подходящие записи."""
        include = include or []
        result_ids = []
        result_metas = []
        result_docs = []
        result_dists = []

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
            if "distances" in include:
                result_dists.append(0.2)  # Фиксированная дистанция для тестов

        # Ограничиваем по n_results
        result_ids = result_ids[:n_results]
        result_metas = result_metas[:n_results]
        result_docs = result_docs[:n_results]
        result_dists = result_dists[:n_results]

        return {
            "ids": [result_ids],
            "metadatas": [result_metas] if "metadatas" in include else [[]],
            "documents": [result_docs] if "documents" in include else [[]],
            "distances": [result_dists] if "distances" in include else [[]],
        }

    def delete(self, ids):
        for doc_id in ids:
            if doc_id in self.data:
                del self.data[doc_id]


@pytest.fixture
def skill_engine():
    """Фикстура для создания SkillEngine с mock-коллекцией и encoder."""
    collection = MockSkillCollection()
    encoder = Mock()
    encoder.encode = Mock(return_value=[0.1] * 384)
    return SkillEngine(collection=collection, encoder=encoder)


class TestSkillCreate:
    """Тесты создания навыков."""

    def test_create_skill_returns_id_and_version(self, skill_engine):
        """Создание навыка возвращает id, version=1, status=ok."""
        result = skill_engine.create_skill(goal="Отвечать на вопросы по Python")
        assert "id" in result
        assert result["version"] == 1
        assert result["status"] == "ok"
        # ID должен быть валидным UUID (требование Qdrant)
        uuid.UUID(result["id"])

    def test_create_skill_stores_metadata(self, skill_engine):
        """Метаданные навыка корректно сохраняются в коллекции."""
        result = skill_engine.create_skill(
            goal="Генерация SQL запросов",
            steps=["Проанализировать схему", "Построить запрос"],
            examples=["SELECT * FROM users"],
            constraints=["Не использовать DELETE без WHERE"],
            sources=["документация PostgreSQL"],
            confidence=0.8,
            tags=["sql", "database"],
            model_name="gpt-4",
            workspace_id="ws-123",
        )
        skill = skill_engine.get_skill(result["id"])
        assert skill is not None
        assert skill["goal"] == "Генерация SQL запросов"
        assert skill["steps"] == ["Проанализировать схему", "Построить запрос"]
        assert skill["examples"] == ["SELECT * FROM users"]
        assert skill["constraints"] == ["Не использовать DELETE без WHERE"]
        assert skill["sources"] == ["документация PostgreSQL"]
        assert skill["confidence"] == 0.8
        assert skill["tags"] == ["sql", "database"]
        assert skill["model_name"] == "gpt-4"
        assert skill["workspace_id"] == "ws-123"
        assert skill["version"] == 1
        assert skill["status"] == SKILL_STATUS_ACTIVE

    def test_create_skill_default_confidence(self, skill_engine):
        """Без явного confidence берётся значение из конфигурации."""
        result = skill_engine.create_skill(goal="Тестовый навык")
        skill = skill_engine.get_skill(result["id"])
        assert skill["confidence"] == 0.5  # SKILL_CONFIDENCE_DEFAULT

    def test_create_skill_with_empty_lists(self, skill_engine):
        """Навык с пустыми списками шагов/примеров/ограничений."""
        result = skill_engine.create_skill(
            goal="Минимальный навык",
            steps=[],
            examples=[],
            constraints=[],
        )
        skill = skill_engine.get_skill(result["id"])
        assert skill["steps"] == []
        assert skill["examples"] == []
        assert skill["constraints"] == []

    def test_create_skill_indexes_in_collection(self, skill_engine):
        """Навык добавляется в коллекцию Qdrant."""
        initial_count = skill_engine.collection.count()
        skill_engine.create_skill(goal="Навык для индексации")
        assert skill_engine.collection.count() == initial_count + 1


class TestSkillGet:
    """Тесты получения навыка по ID."""

    def test_get_existing_skill(self, skill_engine):
        """Получение существующего навыка."""
        result = skill_engine.create_skill(goal="Тестовый навык")
        skill = skill_engine.get_skill(result["id"])
        assert skill is not None
        assert skill["goal"] == "Тестовый навык"

    def test_get_nonexistent_skill_returns_none(self, skill_engine):
        """Несуществующий ID возвращает None."""
        assert skill_engine.get_skill("nonexistent-id") is None


class TestSkillList:
    """Тесты получения списка навыков."""

    def test_list_active_skills(self, skill_engine):
        """Список содержит только активные навыки."""
        skill_engine.create_skill(goal="Навык 1")
        skill_engine.create_skill(goal="Навык 2")
        skills = skill_engine.list_skills()
        assert len(skills) == 2
        for s in skills:
            assert s["status"] == SKILL_STATUS_ACTIVE

    def test_list_empty_returns_empty(self, skill_engine):
        """Пустая коллекция → пустой список."""
        skills = skill_engine.list_skills()
        assert skills == []

    def test_list_filters_by_workspace(self, skill_engine):
        """Фильтрация по workspace_id."""
        skill_engine.create_skill(goal="ws1 навык", workspace_id="ws-1")
        skill_engine.create_skill(goal="ws2 навык", workspace_id="ws-2")
        skills = skill_engine.list_skills(workspace_id="ws-1")
        assert len(skills) == 1
        assert skills[0]["workspace_id"] == "ws-1"


class TestSkillUpdate:
    """Тесты обновления навыков (версионирование)."""

    def test_update_creates_new_version(self, skill_engine):
        """Обновление создаёт новую версию с увеличенным номером."""
        create_result = skill_engine.create_skill(goal="Исходный навык")
        update_result = skill_engine.update_skill(
            skill_id=create_result["id"],
            goal="Обновлённый навык",
        )
        assert update_result is not None
        assert update_result["version"] == 2
        assert update_result["previous_version_id"] == create_result["id"]

    def test_update_marks_old_as_superseded(self, skill_engine):
        """Старая версия помечается как superseded."""
        create_result = skill_engine.create_skill(goal="Версия 1")
        skill_engine.update_skill(skill_id=create_result["id"], goal="Версия 2")

        old_skill = skill_engine.get_skill(create_result["id"])
        assert old_skill is not None
        assert old_skill["status"] == SKILL_STATUS_SUPERSEDED

    def test_update_preserves_unchanged_fields(self, skill_engine):
        """Неизменённые поля наследуются от старой версии."""
        create_result = skill_engine.create_skill(
            goal="Навык с шагами",
            steps=["Шаг 1", "Шаг 2"],
            tags=["тег1"],
        )
        update_result = skill_engine.update_skill(
            skill_id=create_result["id"],
            goal="Обновлённая цель",  # Меняем только goal
        )

        new_skill = skill_engine.get_skill(update_result["id"])
        assert new_skill["goal"] == "Обновлённая цель"
        assert new_skill["steps"] == ["Шаг 1", "Шаг 2"]  # Сохранились
        assert new_skill["tags"] == ["тег1"]  # Сохранились

    def test_update_nonexistent_returns_none(self, skill_engine):
        """Обновление несуществующего навыка → None."""
        result = skill_engine.update_skill(skill_id="nonexistent", goal="test")
        assert result is None

    def test_update_deleted_skill_returns_none(self, skill_engine):
        """Обновление удалённого навыка → None."""
        create_result = skill_engine.create_skill(goal="Будет удалён")
        skill_engine.delete_skill(create_result["id"])
        result = skill_engine.update_skill(skill_id=create_result["id"], goal="test")
        assert result is None

    def test_multiple_updates_increment_version(self, skill_engine):
        """Серия обновлений корректно увеличивает версию."""
        r1 = skill_engine.create_skill(goal="v1")
        r2 = skill_engine.update_skill(skill_id=r1["id"], goal="v2")
        r3 = skill_engine.update_skill(skill_id=r2["id"], goal="v3")

        assert r1["version"] == 1
        assert r2["version"] == 2
        assert r3["version"] == 3


class TestSkillDelete:
    """Тесты мягкого удаления навыков."""

    def test_delete_marks_as_deleted(self, skill_engine):
        """Удаление помечает навык как deleted."""
        result = skill_engine.create_skill(goal="Навык для удаления")
        assert skill_engine.delete_skill(result["id"]) is True

        skill = skill_engine.get_skill(result["id"])
        assert skill["status"] == SKILL_STATUS_DELETED

    def test_delete_nonexistent_returns_false(self, skill_engine):
        """Удаление несуществующего навыка → False."""
        assert skill_engine.delete_skill("nonexistent") is False

    def test_double_delete_returns_false(self, skill_engine):
        """Повторное удаление уже удалённого навыка → False."""
        result = skill_engine.create_skill(goal="test")
        skill_engine.delete_skill(result["id"])
        assert skill_engine.delete_skill(result["id"]) is False

    def test_deleted_skill_not_in_active_list(self, skill_engine):
        """Удалённый навык не появляется в списке активных."""
        r1 = skill_engine.create_skill(goal="Активный")
        r2 = skill_engine.create_skill(goal="Будет удалён")
        skill_engine.delete_skill(r2["id"])

        skills = skill_engine.list_skills()
        skill_ids = [s["id"] for s in skills]
        assert r1["id"] in skill_ids
        assert r2["id"] not in skill_ids


class TestSkillSearch:
    """Тесты семантического поиска навыков."""

    def test_search_returns_results(self, skill_engine):
        """Поиск находит существующие навыки."""
        skill_engine.create_skill(goal="Генерация SQL запросов", confidence=0.8)
        results = skill_engine.search_skills(query="SQL")
        assert len(results) >= 1
        assert results[0]["goal"] == "Генерация SQL запросов"

    def test_search_includes_relevance_score(self, skill_engine):
        """Результаты поиска содержат оценку релевантности."""
        skill_engine.create_skill(goal="Тестовый навык", confidence=0.8)
        results = skill_engine.search_skills(query="тест")
        assert len(results) >= 1
        assert "relevance" in results[0]
        assert 0.0 <= results[0]["relevance"] <= 1.0

    def test_search_filters_by_min_confidence(self, skill_engine):
        """Поиск исключает навыки с низким confidence."""
        skill_engine.create_skill(goal="Уверенный навык", confidence=0.9)
        skill_engine.create_skill(goal="Неуверенный навык", confidence=0.1)

        results = skill_engine.search_skills(query="навык", min_confidence=0.5)
        goals = [r["goal"] for r in results]
        assert "Уверенный навык" in goals
        assert "Неуверенный навык" not in goals

    def test_search_empty_collection(self, skill_engine):
        """Поиск в пустой коллекции → пустой результат."""
        results = skill_engine.search_skills(query="anything")
        assert results == []


class TestSkillUsage:
    """Тесты записи использования навыков."""

    def test_record_usage_increments_count(self, skill_engine):
        """Запись использования увеличивает usage_count."""
        result = skill_engine.create_skill(goal="Часто используемый навык")
        skill_engine.record_usage(result["id"])
        skill = skill_engine.get_skill(result["id"])
        assert skill["usage_count"] == 1

    def test_record_usage_increases_confidence(self, skill_engine):
        """Запись использования повышает confidence."""
        result = skill_engine.create_skill(goal="test", confidence=0.5)
        skill_engine.record_usage(result["id"])
        skill = skill_engine.get_skill(result["id"])
        assert skill["confidence"] > 0.5

    def test_record_usage_nonexistent_returns_false(self, skill_engine):
        """Запись использования несуществующего навыка → False."""
        assert skill_engine.record_usage("nonexistent") is False

    def test_record_usage_deleted_returns_false(self, skill_engine):
        """Запись использования удалённого навыка → False."""
        result = skill_engine.create_skill(goal="deleted")
        skill_engine.delete_skill(result["id"])
        assert skill_engine.record_usage(result["id"]) is False

    def test_multiple_usages_accumulate(self, skill_engine):
        """Множественные записи использования суммируются."""
        result = skill_engine.create_skill(goal="popular skill", confidence=0.5)
        for _ in range(5):
            skill_engine.record_usage(result["id"])
        skill = skill_engine.get_skill(result["id"])
        assert skill["usage_count"] == 5
        assert skill["confidence"] > 0.5

    def test_confidence_never_exceeds_one(self, skill_engine):
        """Confidence не превышает 1.0 даже при многократном использовании."""
        result = skill_engine.create_skill(goal="test", confidence=0.99)
        for _ in range(100):
            skill_engine.record_usage(result["id"])
        skill = skill_engine.get_skill(result["id"])
        assert skill["confidence"] <= 1.0


class TestSkillFromDialog:
    """Тесты создания навыка из диалога."""

    def test_extracts_goal_from_first_line(self, skill_engine):
        """Цель навыка извлекается из первой строки диалога."""
        dialog = "Как развернуть приложение на Kubernetes\n1. Создать Dockerfile\n2. Написать манифест"
        result = skill_engine.create_from_dialog(dialog_text=dialog)
        skill = skill_engine.get_skill(result["id"])
        assert skill["goal"] == "Как развернуть приложение на Kubernetes"

    def test_extracts_numbered_steps(self, skill_engine):
        """Нумерованные шаги извлекаются из диалога."""
        dialog = "Деплой на K8s\n1. Создать Dockerfile\n2. Написать manifest\n3. Применить kubectl apply"
        result = skill_engine.create_from_dialog(dialog_text=dialog)
        skill = skill_engine.get_skill(result["id"])
        assert len(skill["steps"]) == 3

    def test_extracts_examples(self, skill_engine):
        """Примеры извлекаются по ключевым словам."""
        dialog = "Работа с API\nНапример, GET /users возвращает список"
        result = skill_engine.create_from_dialog(dialog_text=dialog)
        skill = skill_engine.get_skill(result["id"])
        assert len(skill["examples"]) >= 1

    def test_extracts_constraints(self, skill_engine):
        """Ограничения извлекаются по ключевым словам."""
        dialog = "SQL запросы\nНельзя использовать DELETE без WHERE"
        result = skill_engine.create_from_dialog(dialog_text=dialog)
        skill = skill_engine.get_skill(result["id"])
        assert len(skill["constraints"]) >= 1

    def test_from_dialog_sets_source(self, skill_engine):
        """Источник навыка из диалога — 'dialog'."""
        result = skill_engine.create_from_dialog(dialog_text="Простой навык")
        skill = skill_engine.get_skill(result["id"])
        assert "dialog" in skill["sources"]

    def test_from_dialog_with_model_and_workspace(self, skill_engine):
        """Навык из диалога привязывается к модели и workspace."""
        result = skill_engine.create_from_dialog(
            dialog_text="Навык",
            model_name="gpt-4",
            workspace_id="ws-1",
        )
        skill = skill_engine.get_skill(result["id"])
        assert skill["model_name"] == "gpt-4"
        assert skill["workspace_id"] == "ws-1"


class TestSkillLifecycle:
    """Интеграционные тесты жизненного цикла навыка."""

    def test_full_lifecycle(self, skill_engine):
        """Полный цикл: создание → поиск → использование → обновление → удаление."""
        # 1. Создание
        create_result = skill_engine.create_skill(
            goal="Написание REST API",
            steps=["Определить ресурсы", "Написать эндпоинты"],
            confidence=0.6,
        )
        assert create_result["version"] == 1

        # 2. Поиск
        search_results = skill_engine.search_skills(query="REST API")
        assert len(search_results) >= 1

        # 3. Использование
        skill_engine.record_usage(create_result["id"])
        skill = skill_engine.get_skill(create_result["id"])
        assert skill["usage_count"] == 1
        assert skill["confidence"] > 0.6

        # 4. Обновление (версионирование)
        update_result = skill_engine.update_skill(
            skill_id=create_result["id"],
            goal="Написание REST API с аутентификацией",
            steps=["Определить ресурсы", "Написать эндпоинты", "Добавить JWT"],
        )
        assert update_result["version"] == 2

        # Старая версия — superseded
        old = skill_engine.get_skill(create_result["id"])
        assert old["status"] == SKILL_STATUS_SUPERSEDED

        # 5. Удаление
        assert skill_engine.delete_skill(update_result["id"]) is True
        deleted = skill_engine.get_skill(update_result["id"])
        assert deleted["status"] == SKILL_STATUS_DELETED
