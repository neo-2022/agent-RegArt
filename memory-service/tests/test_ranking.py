from datetime import datetime, timedelta, timezone

from app.ranking import build_rank_score


def test_rank_score_respects_metadata_factors():
    """Проверяет, что importance/reliability/frequency и recency влияют на итоговый score."""
    fresh_ts = datetime.now(timezone.utc).isoformat()
    stale_ts = (datetime.now(timezone.utc) - timedelta(days=365)).isoformat()

    rich_meta = {
        "importance": 1.0,
        "reliability": 1.0,
        "frequency": 1.0,
        "created_at": fresh_ts,
    }
    weak_meta = {
        "importance": 0.0,
        "reliability": 0.0,
        "frequency": 0.0,
        "created_at": stale_ts,
    }

    strong = build_rank_score(0.8, rich_meta)
    weak = build_rank_score(0.8, weak_meta)

    assert strong > weak
    assert 0.0 <= strong <= 1.0
    assert 0.0 <= weak <= 1.0


def test_rank_score_handles_invalid_timestamp_gracefully():
    """Проверяет устойчивость ранжирования к некорректному created_at."""
    meta = {
        "importance": 0.5,
        "reliability": 0.5,
        "frequency": 0.5,
        "created_at": "not-a-date",
    }
    score = build_rank_score(0.7, meta)
    assert 0.0 <= score <= 1.0
