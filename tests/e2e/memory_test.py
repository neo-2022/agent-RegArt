"""E2E tests for memory-service."""
import os
import pytest
import requests

BASE = os.getenv("MEMORY_URL", "http://localhost:8001")
TIMEOUT = 5


def _url(path: str) -> str:
    return f"{BASE}{path}"


@pytest.fixture(autouse=True)
def skip_if_unavailable():
    try:
        requests.get(_url("/health"), timeout=2)
    except requests.ConnectionError:
        pytest.skip("memory-service not available")


class TestHealth:
    def test_health(self):
        r = requests.get(_url("/health"), timeout=TIMEOUT)
        assert r.status_code == 200
        data = r.json()
        assert data["status"] == "ok"


class TestFacts:
    def test_add_and_search(self):
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
    def test_stats(self):
        r = requests.get(_url("/stats"), timeout=TIMEOUT)
        assert r.status_code == 200
        data = r.json()
        assert "facts_count" in data
        assert "files_count" in data


class TestTTL:
    def test_expired_endpoint(self):
        r = requests.get(_url("/ttl/expired"), timeout=TIMEOUT)
        assert r.status_code == 200
        data = r.json()
        assert "expired_count" in data

    def test_reindex_status(self):
        r = requests.get(_url("/reindex/status"), timeout=TIMEOUT)
        assert r.status_code == 200
        data = r.json()
        assert "needs_reindex" in data


class TestLearnings:
    def test_add_learning(self):
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
        r = requests.post(
            _url("/learnings/search"),
            json={"query": "E2E test", "top_k": 3},
            timeout=TIMEOUT,
        )
        assert r.status_code == 200

    def test_learning_stats(self):
        r = requests.get(_url("/learnings/stats"), timeout=TIMEOUT)
        assert r.status_code == 200
