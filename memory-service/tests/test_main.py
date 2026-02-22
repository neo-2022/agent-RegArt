"""
Юнит-тесты для memory-service (FastAPI).

Используют TestClient для проверки HTTP-эндпоинтов
без запуска реального сервера. Покрывают:
- /health — проверка здоровья сервиса
- /stats — статистика коллекций
- /facts — добавление фактов
- /search — поиск фактов
- /files/chunks — добавление фрагментов файлов
- /files — список файлов
- /learnings — система обучения агентов
"""
import pytest
from fastapi.testclient import TestClient

from app.main import app


@pytest.fixture
def client():
    """Создаёт тестовый HTTP-клиент для FastAPI-приложения."""
    return TestClient(app)


def test_health(client):
    """Проверяет эндпоинт /health: статус 200, status=ok, service=memory-service."""
    resp = client.get("/health")
    assert resp.status_code == 200
    data = resp.json()
    assert data["status"] == "ok"
    assert data["service"] == "memory-service"


def test_stats(client):
    """Проверяет эндпоинт /stats: наличие полей facts_count, files_count, learnings_count."""
    resp = client.get("/stats")
    assert resp.status_code == 200
    data = resp.json()
    assert "facts_count" in data
    assert "files_count" in data
    assert "learnings_count" in data


def test_add_fact(client):
    """Проверяет добавление факта через POST /facts: статус ok, возвращается ID."""
    resp = client.post("/facts", json={
        "text": "Test fact for unit test",
        "metadata": {"source": "test"},
    })
    assert resp.status_code == 200
    data = resp.json()
    assert data["status"] == "ok"
    assert "id" in data
    assert len(data["id"]) > 0


def test_add_fact_empty_text(client):
    """Проверяет валидацию: пустой текст факта → ошибка 422 или 500."""
    resp = client.post("/facts", json={"text": "", "metadata": {"source": "test"}})
    assert resp.status_code in (422, 500)


def test_search_empty(client):
    """Проверяет поиск по несуществующему запросу: возвращает results и count."""
    resp = client.post("/search", json={"query": "nonexistent query xyz123"})
    assert resp.status_code == 200
    data = resp.json()
    assert "results" in data
    assert "count" in data


def test_search_with_params(client):
    """Проверяет поиск с параметрами top_k и include_files."""
    resp = client.post("/search", json={
        "query": "test",
        "top_k": 3,
        "include_files": False,
    })
    assert resp.status_code == 200
    data = resp.json()
    assert data["count"] >= 0


def test_add_file_chunk(client):
    """Проверяет добавление фрагмента файла через POST /files/chunks."""
    resp = client.post("/files/chunks", json={
        "text": "This is a test file chunk content",
        "metadata": {
            "agent": "admin",
            "filename": "test.txt",
            "file_id": "test-file-001",
            "chunk": 0,
        },
    })
    assert resp.status_code == 200
    data = resp.json()
    assert data["status"] == "ok"
    assert "id" in data


def test_list_files(client):
    """Проверяет получение списка файлов через GET /files."""
    resp = client.get("/files")
    assert resp.status_code == 200


def test_add_learning(client):
    """Проверяет добавление знания для модели через POST /learnings."""
    resp = client.post("/learnings", json={
        "text": "User prefers Russian language",
        "model_name": "test-model",
        "agent_name": "admin",
        "category": "preference",
    })
    assert resp.status_code == 200
    data = resp.json()
    assert data["status"] == "ok"
    assert "id" in data


def test_search_learnings(client):
    """Проверяет поиск знаний для модели через POST /learnings/search."""
    resp = client.post("/learnings/search", json={
        "query": "language preference",
        "model_name": "test-model",
        "top_k": 3,
    })
    assert resp.status_code == 200
    data = resp.json()
    assert "results" in data
    assert "count" in data
    assert data["model_name"] == "test-model"


def test_learning_stats(client):
    """Проверяет статистику обучения через GET /learnings/stats."""
    resp = client.get("/learnings/stats")
    assert resp.status_code == 200
    data = resp.json()
    assert "total_learnings" in data
    assert "by_model" in data
    assert "by_category" in data
