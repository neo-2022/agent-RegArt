"""
Тесты для memory-service (FastAPI).
Используют TestClient для проверки эндпоинтов без запуска сервера.
"""
import pytest
from fastapi.testclient import TestClient

from app.main import app


@pytest.fixture
def client():
    return TestClient(app)


def test_health(client):
    resp = client.get("/health")
    assert resp.status_code == 200
    data = resp.json()
    assert data["status"] == "ok"
    assert data["service"] == "memory-service"


def test_stats(client):
    resp = client.get("/stats")
    assert resp.status_code == 200
    data = resp.json()
    assert "facts_count" in data
    assert "files_count" in data
    assert "learnings_count" in data


def test_add_fact(client):
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
    resp = client.post("/facts", json={"text": "", "metadata": {"source": "test"}})
    assert resp.status_code in (422, 500)


def test_search_empty(client):
    resp = client.post("/search", json={"query": "nonexistent query xyz123"})
    assert resp.status_code == 200
    data = resp.json()
    assert "results" in data
    assert "count" in data


def test_search_with_params(client):
    resp = client.post("/search", json={
        "query": "test",
        "top_k": 3,
        "include_files": False,
    })
    assert resp.status_code == 200
    data = resp.json()
    assert data["count"] >= 0


def test_add_file_chunk(client):
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
    resp = client.get("/files")
    assert resp.status_code == 200


def test_add_learning(client):
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
    resp = client.get("/learnings/stats")
    assert resp.status_code == 200
    data = resp.json()
    assert "total_learnings" in data
    assert "by_model" in data
    assert "by_category" in data
