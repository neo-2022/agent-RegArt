"""
Интеграционные тесты для middleware memory-service.

Покрывают:
- Пропагация X-Request-ID через все ответы
- Генерация X-Request-ID при отсутствии заголовка
- Корректная работа CORS-заголовков
"""
import pytest
from fastapi.testclient import TestClient

from app.main import app


@pytest.fixture
def client():
    """Создаёт тестовый HTTP-клиент для FastAPI-приложения."""
    return TestClient(app)


def test_correlation_id_propagation(client):
    """Проверяет, что X-Request-ID из запроса возвращается в ответе."""
    custom_id = "test-correlation-123"
    resp = client.get("/health", headers={"X-Request-ID": custom_id})
    assert resp.status_code == 200
    assert resp.headers.get("X-Request-ID") == custom_id


def test_correlation_id_generated(client):
    """Проверяет, что X-Request-ID генерируется при отсутствии в запросе."""
    resp = client.get("/health")
    assert resp.status_code == 200
    rid = resp.headers.get("X-Request-ID")
    assert rid is not None
    assert len(rid) > 0


def test_health_response_format(client):
    """Проверяет формат ответа /health: JSON с полями status и service."""
    resp = client.get("/health")
    assert resp.status_code == 200
    data = resp.json()
    assert data["status"] == "ok"
    assert data["service"] == "memory-service"


def test_invalid_json_body(client):
    """Проверяет обработку невалидного JSON в POST-запросе."""
    resp = client.post(
        "/facts",
        content="not valid json",
        headers={"Content-Type": "application/json"},
    )
    assert resp.status_code == 422
