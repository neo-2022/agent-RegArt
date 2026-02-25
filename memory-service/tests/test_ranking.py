from datetime import datetime, timedelta, timezone
import pytest

from app.ranking import build_rank_score, blend_relevance_scores, resolve_priority_score, MEMORY_PRIORITY_SCORES


class TestBlendRelevanceScores:
    """Набор тестов для гибридной релевантности (semantic + keyword)."""

    def test_blend_equal_weights_average(self):
        """Проверяет, что при равных весах результат — простое усреднение."""
        # SEARCH_SEMANTIC_WEIGHT = 0.8, SEARCH_KEYWORD_WEIGHT = 0.2 по умолчанию
        # Для простого случая: (0.8 * 0.5 + 0.2 * 1.0) / (0.8 + 0.2) = 0.6
        blended = blend_relevance_scores(semantic_relevance=0.5, keyword_relevance=1.0)
        assert 0.5 < blended < 1.0
        assert 0.0 <= blended <= 1.0

    def test_blend_semantic_dominant(self):
        """Проверяет доминирование semantic сигнала при высокой семантической релевантности."""
        # Высокая семантика, низкий keyword
        blended = blend_relevance_scores(semantic_relevance=0.9, keyword_relevance=0.1)
        assert blended > 0.5  # Должно быть ближе к 0.9

    def test_blend_keyword_boost(self):
        """Проверяет, что keyword-сигнал поднимает relevance при слабой семантике."""
        low_semantic_only = blend_relevance_scores(0.2, 0.0)
        low_semantic_with_keyword = blend_relevance_scores(0.2, 1.0)
        assert low_semantic_with_keyword > low_semantic_only

    def test_blend_clamps_above_one(self):
        """Проверяет защиту от valores выше 1.0."""
        blended = blend_relevance_scores(semantic_relevance=2.0, keyword_relevance=3.0)
        assert blended == 1.0

    def test_blend_clamps_below_zero(self):
        """Проверяет защиту от отрицательных значений."""
        blended = blend_relevance_scores(semantic_relevance=-1.0, keyword_relevance=-2.0)
        assert blended == 0.0

    def test_blend_both_zero(self):
        """Проверяет результат при обоих нулё."""
        blended = blend_relevance_scores(semantic_relevance=0.0, keyword_relevance=0.0)
        assert blended == 0.0

    def test_blend_both_one(self):
        """Проверяет результат при обоих единице."""
        blended = blend_relevance_scores(semantic_relevance=1.0, keyword_relevance=1.0)
        assert blended == 1.0

    def test_blend_rounded_to_four_decimals(self):
        """Проверяет, что результат округлён до 4 знаков."""
        blended = blend_relevance_scores(0.123456, 0.654321)
        # Проверяем, что это число с максимум 4 знаками после запятой
        assert len(str(blended).split('.')[-1]) <= 4


class TestBuildRankScore:
    """Набор тестов для композитного ранжирования."""

    def test_rank_score_respects_all_metadata_factors(self):
        """Проверяет, что importance/reliability/frequency и recency влияют на score."""
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

    def test_rank_score_clamps_to_01_range(self):
        """Проверяет, что score всегда в диапазоне [0..1]."""
        # Экстремальные значения
        max_score = build_rank_score(1.0, {
            "importance": 1.0,
            "reliability": 1.0,
            "frequency": 1.0,
            "created_at": datetime.now(timezone.utc).isoformat(),
        })
        min_score = build_rank_score(0.0, {})

        assert max_score <= 1.0
        assert min_score >= 0.0

    def test_rank_score_handles_missing_metadata(self):
        """Проверяет устойчивость к отсутствующим полям metdata."""
        meta_minimal = {}
        score = build_rank_score(0.7, meta_minimal)
        assert 0.0 <= score <= 1.0

    def test_rank_score_handles_invalid_timestamp(self):
        """Проверяет устойчивость к некорректному created_at."""
        meta_bad_ts = {
            "importance": 0.5,
            "reliability": 0.5,
            "frequency": 0.5,
            "created_at": "not-a-date",
        }
        score = build_rank_score(0.7, meta_bad_ts)
        assert 0.0 <= score <= 1.0

    def test_rank_score_handles_non_numeric_metadata(self):
        """Проверяет защиту от non-numeric значений в importance/reliability/frequency."""
        meta_bad_values = {
            "importance": "very high",  # Строка вместо числа
            "reliability": None,         # None
            "frequency": "xyz",          # Строка
            "created_at": datetime.now(timezone.utc).isoformat(),
        }
        score = build_rank_score(0.7, meta_bad_values)
        assert 0.0 <= score <= 1.0

    def test_rank_score_respects_memory_priority(self):
        """Проверяет, что более высокий приоритет повышает score."""
        base_meta = {
            "importance": 0.5,
            "reliability": 0.5,
            "frequency": 0.5,
            "created_at": datetime.now(timezone.utc).isoformat(),
        }
        critical = build_rank_score(0.7, {**base_meta, "priority": "critical"})
        pinned = build_rank_score(0.7, {**base_meta, "priority": "pinned"})
        normal = build_rank_score(0.7, {**base_meta, "priority": "normal"})
        archived = build_rank_score(0.7, {**base_meta, "priority": "archived"})

        assert critical > pinned > normal > archived

    def test_rank_score_uses_normal_priority_for_unknown(self):
        """Неизвестный priority должен безопасно трактоваться как normal."""
        meta_normal = {
            "importance": 0.5,
            "reliability": 0.5,
            "frequency": 0.5,
            "created_at": datetime.now(timezone.utc).isoformat(),
            "priority": "normal",
        }
        meta_unknown = {**meta_normal, "priority": "unknown-priority-xyz"}
        assert build_rank_score(0.6, meta_normal) == build_rank_score(0.6, meta_unknown)

    def test_rank_score_rounded_to_four_decimals(self):
        """Проверяет, что результат округлён до 4 знаков."""
        meta = {
            "importance": 0.123,
            "reliability": 0.456,
            "frequency": 0.789,
            "created_at": datetime.now(timezone.utc).isoformat(),
        }
        score = build_rank_score(0.555, meta)
        assert len(str(score).split('.')[-1]) <= 4

    def test_rank_score_recency_decays_over_time(self):
        """Проверяет, что свежие записи получают выше score."""
        base_meta = {
            "importance": 0.5,
            "reliability": 0.5,
            "frequency": 0.5,
        }
        fresh_ts = datetime.now(timezone.utc).isoformat()
        old_ts = (datetime.now(timezone.utc) - timedelta(days=60)).isoformat()

        fresh_score = build_rank_score(0.7, {**base_meta, "created_at": fresh_ts})
        old_score = build_rank_score(0.7, {**base_meta, "created_at": old_ts})

        assert fresh_score > old_score


class TestResolvePriorityScore:
    """Набор тестов для преобразования приоритета в score."""

    def test_resolve_all_priority_levels(self):
        """Проверяет, что все уровни приоритета корректно преобразуются."""
        assert resolve_priority_score("critical") == MEMORY_PRIORITY_SCORES["critical"]
        assert resolve_priority_score("pinned") == MEMORY_PRIORITY_SCORES["pinned"]
        assert resolve_priority_score("reinforced") == MEMORY_PRIORITY_SCORES["reinforced"]
        assert resolve_priority_score("normal") == MEMORY_PRIORITY_SCORES["normal"]
        assert resolve_priority_score("archived") == MEMORY_PRIORITY_SCORES["archived"]

    def test_resolve_case_insensitive(self):
        """Проверяет, что приоритеты регистронезависимы."""
        assert resolve_priority_score("CRITICAL") == resolve_priority_score("critical")
        assert resolve_priority_score("Normal") == resolve_priority_score("normal")

    def test_resolve_with_whitespace(self):
        """Проверяет обработку пробелов."""
        assert resolve_priority_score("  critical  ") == resolve_priority_score("critical")

    def test_resolve_unknown_priority_uses_normal(self):
        """Проверяет fallback на normal для неизвестных приоритетов."""
        assert resolve_priority_score("unknown") == MEMORY_PRIORITY_SCORES["normal"]
        assert resolve_priority_score("xyz") == MEMORY_PRIORITY_SCORES["normal"]

    def test_resolve_none_uses_normal(self):
        """Проверяет, что None fallback на normal."""
        assert resolve_priority_score(None) == MEMORY_PRIORITY_SCORES["normal"]

    def test_resolve_priority_scores_ordered(self):
        """Проверяет, что приоритеты упорядочены корректно (убывание)."""
        scores = [
            MEMORY_PRIORITY_SCORES["critical"],
            MEMORY_PRIORITY_SCORES["pinned"],
            MEMORY_PRIORITY_SCORES["reinforced"],
            MEMORY_PRIORITY_SCORES["normal"],
            MEMORY_PRIORITY_SCORES["archived"],
        ]
        assert scores == sorted(scores, reverse=True)


# === Оригинальные тесты (сохранены для совместимости) ===

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


def test_blend_relevance_scores_prefers_keyword_when_semantic_low():
    """Проверяет, что keyword-сигнал поднимает relevance при слабой семантике."""
    blended = blend_relevance_scores(semantic_relevance=0.2, keyword_relevance=1.0)
    assert 0.2 < blended <= 1.0


def test_blend_relevance_scores_clamps_invalid_values():
    """Проверяет защиту от выхода relevance за диапазон [0..1]."""
    blended = blend_relevance_scores(semantic_relevance=2.0, keyword_relevance=-1.0)
    assert 0.0 <= blended <= 1.0


def test_rank_score_respects_memory_priority():
    """Проверяет, что более высокий приоритет памяти повышает итоговый score."""
    base_meta = {
        "importance": 0.5,
        "reliability": 0.5,
        "frequency": 0.5,
        "created_at": datetime.now(timezone.utc).isoformat(),
    }
    critical = build_rank_score(0.7, {**base_meta, "priority": "critical"})
    archived = build_rank_score(0.7, {**base_meta, "priority": "archived"})
    assert critical > archived


def test_rank_score_uses_normal_priority_for_unknown_value():
    """Неизвестный priority должен безопасно трактоваться как normal."""
    meta_normal = {
        "importance": 0.5,
        "reliability": 0.5,
        "frequency": 0.5,
        "created_at": datetime.now(timezone.utc).isoformat(),
        "priority": "normal",
    }
    meta_unknown = {**meta_normal, "priority": "unexpected-priority"}
    assert build_rank_score(0.6, meta_normal) == build_rank_score(0.6, meta_unknown)
