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
import uuid
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


def test_search_workspace_isolation(client):
    """Проверяет изоляцию поиска фактов по workspace_id."""
    client.post("/facts", json={
        "text": "workspace alpha fact",
        "metadata": {"source": "test", "workspace_id": "alpha"},
    })
    client.post("/facts", json={
        "text": "workspace beta fact",
        "metadata": {"source": "test", "workspace_id": "beta"},
    })

    only_alpha = client.post("/search", json={
        "query": "workspace fact",
        "workspace_id": "alpha",
        "top_k": 10,
    })
    assert only_alpha.status_code == 200
    alpha_results = only_alpha.json()["results"]
    assert len(alpha_results) > 0
    assert all(item["metadata"].get("workspace_id") == "alpha" for item in alpha_results)


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
    assert "version" in data
    assert data["version"] >= 1
    assert "learning_key" in data


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


def test_learning_versioning_and_soft_delete(client):
    """Проверяет versioning знаний и soft-delete (без физического удаления записей)."""
    model_name = f"versioned-model-{uuid.uuid4()}"
    payload = {
        "text": "Use markdown headers in answers",
        "model_name": model_name,
        "agent_name": "admin",
        "category": "preference",
    }

    first = client.post("/learnings", json=payload)
    assert first.status_code == 200
    first_data = first.json()
    assert first_data["version"] == 1
    assert first_data["conflict_detected"] is False

    second = client.post("/learnings", json=payload)
    assert second.status_code == 200
    second_data = second.json()
    assert second_data["version"] == 2
    assert second_data["previous_version_id"] == first_data["id"]

    search = client.post("/learnings/search", json={
        "query": "markdown headers",
        "model_name": model_name,
        "top_k": 10,
    })
    assert search.status_code == 200
    search_data = search.json()
    # В выдаче должна быть только активная версия (v2)
    versions = [item["metadata"].get("version") for item in search_data["results"]]
    assert 2 in versions
    assert 1 not in versions

    deleted = client.delete(f"/learnings/{model_name}")
    assert deleted.status_code == 200
    assert deleted.json()["deleted_count"] >= 1

    search_after = client.post("/learnings/search", json={
        "query": "markdown headers",
        "model_name": model_name,
        "top_k": 10,
    })
    assert search_after.status_code == 200
    assert search_after.json()["count"] == 0


def test_learning_conflict_detection(client):
    """Проверяет детект конфликта, когда одно и то же знание обновляется и меняет текст."""
    base_payload = {
        "model_name": f"conflict-model-{uuid.uuid4()}",
        "agent_name": "admin",
        "category": "fact",
    }

    first = client.post("/learnings", json={**base_payload, "text": "Python version is 3.12"})
    assert first.status_code == 200
    first_data = first.json()
    assert first_data["version"] == 1

    second = client.post("/learnings", json={**base_payload, "text": "Python version is 3.11"})
    assert second.status_code == 200
    second_data = second.json()
    assert second_data["version"] == 2
    assert second_data["conflict_detected"] is True
    assert second_data["previous_version_id"] == first_data["id"]


def test_learning_workspace_isolation(client):
    """Проверяет, что learnings изолированы по workspace_id."""
    model_name = f"ws-model-{uuid.uuid4()}"

    alpha = client.post("/learnings", json={
        "text": "Use concise answers",
        "model_name": model_name,
        "agent_name": "admin",
        "workspace_id": "alpha",
        "category": "preference",
    })
    assert alpha.status_code == 200

    beta = client.post("/learnings", json={
        "text": "Use detailed answers",
        "model_name": model_name,
        "agent_name": "admin",
        "workspace_id": "beta",
        "category": "preference",
    })
    assert beta.status_code == 200

    alpha_search = client.post("/learnings/search", json={
        "query": "answers",
        "model_name": model_name,
        "workspace_id": "alpha",
        "top_k": 10,
    })
    assert alpha_search.status_code == 200
    for item in alpha_search.json()["results"]:
        assert item["metadata"].get("workspace_id") == "alpha"


def test_learning_versions_endpoint(client):
    """Проверяет эндпоинт истории версий /learnings/versions/{model_name}."""
    model_name = f"versions-model-{uuid.uuid4()}"
    payload = {
        "model_name": model_name,
        "agent_name": "admin",
        "workspace_id": "ws-v",
        "category": "fact",
    }

    first = client.post("/learnings", json={**payload, "text": "Nginx port is 80"})
    assert first.status_code == 200
    second = client.post("/learnings", json={**payload, "text": "Nginx port is 8080"})
    assert second.status_code == 200

    versions = client.get(f"/learnings/versions/{model_name}?workspace_id=ws-v&category=fact")
    assert versions.status_code == 200
    data = versions.json()
    assert data["count"] >= 2
    assert data["versions"][0]["version"] >= data["versions"][1]["version"]
    statuses = {v["status"] for v in data["versions"]}
    assert "active" in statuses
    assert "superseded" in statuses


def test_audit_logs_endpoint(client):
    """Проверяет, что /audit/logs возвращает события и поддерживает фильтр workspace."""
    model_name = f"audit-model-{uuid.uuid4()}"
    workspace_id = f"audit-ws-{uuid.uuid4()}"

    added = client.post("/learnings", json={
        "text": "Enable audit trail",
        "model_name": model_name,
        "agent_name": "admin",
        "workspace_id": workspace_id,
        "category": "fact",
    })
    assert added.status_code == 200

    logs = client.get(f"/audit/logs?workspace_id={workspace_id}&model_name={model_name}&top_k=10")
    assert logs.status_code == 200
    data = logs.json()
    assert data["count"] >= 1
    assert any(item["event_type"] == "learning_added" for item in data["logs"])
    assert all(item.get("workspace_id") == workspace_id for item in data["logs"])


def test_retrieval_metrics_endpoint(client):
    """Проверяет endpoint агрегированных retrieval-метрик."""
    client.post("/facts", json={
        "text": "metrics fact",
        "metadata": {"workspace_id": "metrics-ws", "source": "test"},
    })
    client.post("/search", json={
        "query": "metrics",
        "workspace_id": "metrics-ws",
        "top_k": 5,
    })

    metrics = client.get("/metrics/retrieval")
    assert metrics.status_code == 200
    data = metrics.json()
    assert data["search_requests_total"] >= 1
    assert data["search_latency_ms_avg"] >= 0


def test_backup_checks_endpoint(client):
    """Проверяет endpoint готовности backup/recovery."""
    resp = client.get("/backup/checks")
    assert resp.status_code == 200
    data = resp.json()
    assert "pg_dump_available" in data
    assert "qdrant_snapshot_enabled" in data
    assert "neo4j_backup_enabled" in data
    assert "minio_versioning_enabled" in data
    assert "restore_test_enabled" in data


def test_search_min_priority_filter(client):
    """Проверяет фильтрацию retrieval по минимальному приоритету памяти."""
    client.post("/facts", json={
        "text": "priority archived sample",
        "metadata": {"source": "test", "priority": "archived", "workspace_id": "prio-ws"},
    })
    client.post("/facts", json={
        "text": "priority critical sample",
        "metadata": {"source": "test", "priority": "critical", "workspace_id": "prio-ws"},
    })

    response = client.post("/search", json={
        "query": "priority sample",
        "top_k": 10,
        "workspace_id": "prio-ws",
        "min_priority": "critical",
    })

    assert response.status_code == 200
    data = response.json()
    assert data["count"] >= 1
    assert all(item["metadata"].get("priority") == "critical" for item in data["results"])
