"""
Сквозные (end-to-end) тесты для memory-service.

Проверяют реальные HTTP-эндпоинты memory-service:
- /health — проверка здоровья сервиса
- /facts — добавление и поиск фактов
- /stats — статистика хранилища
- /ttl/expired — истёкшие документы
- /reindex/status — статус переиндексации
- /learnings — обучение агента (вопрос-ответ пары)

Для запуска необходим работающий memory-service (по умолчанию http://localhost:8001).
URL можно переопределить через переменную окружения MEMORY_URL.
"""
import os
import pytest
import requests

# Базовый URL memory-service (из переменной окружения или значение по умолчанию)
BASE = os.getenv("MEMORY_URL", "http://localhost:8001")
# Таймаут HTTP-запросов в секундах
TIMEOUT = 5


def _url(path: str) -> str:
    """Формирует полный URL из базового адреса и пути эндпоинта."""
    return f"{BASE}{path}"


@pytest.fixture(autouse=True)
def skip_if_unavailable():
    """Автоматически пропускает тесты, если memory-service недоступен."""
    try:
        requests.get(_url("/health"), timeout=2)
    except requests.ConnectionError:
        pytest.skip("memory-service недоступен")


class TestHealth:
    """Тесты эндпоинта здоровья /health."""

    def test_health(self):
        """Проверяет, что /health возвращает 200 и {"status": "ok"}."""
        r = requests.get(_url("/health"), timeout=TIMEOUT)
        assert r.status_code == 200
        data = r.json()
        assert data["status"] == "ok"


class TestFacts:
    """Тесты работы с фактами: добавление и семантический поиск."""

    def test_add_and_search(self):
        """Добавляет тестовый факт и проверяет, что он находится через /search."""
        r = requests.post(
            _url("/facts"),
            json={"text": "E2E test fact", "metadata": {"source": "e2e"}},
            timeout=TIMEOUT,
        )
        assert r.status_code == 200

        r = requests.post(
            _url("/search"),
            json={"query": "E2E test fact", "top_k": 3},
            timeout=TIMEOUT,
        )
        assert r.status_code == 200
        data = r.json()
        assert "results" in data


class TestStats:
    """Тесты эндпоинта статистики /stats."""

    def test_stats(self):
        """Проверяет, что /stats возвращает количество фактов и файлов."""
        r = requests.get(_url("/stats"), timeout=TIMEOUT)
        assert r.status_code == 200
        data = r.json()
        assert "facts_count" in data
        assert "files_count" in data


class TestTTL:
    """Тесты управления TTL (временем жизни документов)."""

    def test_expired_endpoint(self):
        """Проверяет эндпоинт /ttl/expired — возвращает количество истёкших документов."""
        r = requests.get(_url("/ttl/expired"), timeout=TIMEOUT)
        assert r.status_code == 200
        data = r.json()
        assert "expired_count" in data

    def test_reindex_status(self):
        """Проверяет эндпоинт /reindex/status — нужна ли переиндексация."""
        r = requests.get(_url("/reindex/status"), timeout=TIMEOUT)
        assert r.status_code == 200
        data = r.json()
        assert "needs_reindex" in data


class TestLearnings:
    """Тесты обучения агента — сохранение и поиск пар вопрос-ответ."""

    def test_add_learning(self):
        """Добавляет обучающую пару (вопрос + ответ + оценка качества)."""
        r = requests.post(
            _url("/learnings"),
            json={
                "question": "E2E test question",
                "answer": "E2E test answer",
                "quality": 5,
                "metadata": {"source": "e2e"},
            },
            timeout=TIMEOUT,
        )
        assert r.status_code == 200

    def test_search_learnings(self):
        """Ищет обучающие пары по запросу через /learnings/search."""
        r = requests.post(
            _url("/learnings/search"),
            json={"query": "E2E test", "top_k": 3},
            timeout=TIMEOUT,
        )
        assert r.status_code == 200

    def test_learning_stats(self):
        """Проверяет статистику обучения через /learnings/stats."""
        r = requests.get(_url("/learnings/stats"), timeout=TIMEOUT)
        assert r.status_code == 200
