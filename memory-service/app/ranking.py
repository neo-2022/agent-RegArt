from datetime import datetime, timezone
from typing import Any, Dict

from .config import settings


def _safe_float(value: Any, default: float) -> float:
    """Безопасно приводит значение к float для защиты от некорректных метаданных."""
    try:
        return float(value)
    except (TypeError, ValueError):
        return default


def _recency_score(created_at: str) -> float:
    """
    Возвращает нормированный recency score [0..1].

    Логика:
    - если timestamp отсутствует/битый, возвращаем 0.5 как нейтральную оценку;
    - чем «моложе» запись относительно окна RECENCY_WINDOW_DAYS, тем ближе к 1.
    """
    if not created_at:
        return 0.5
    try:
        parsed = datetime.fromisoformat(created_at)
        if parsed.tzinfo is None:
            parsed = parsed.replace(tzinfo=timezone.utc)
        age_days = max((datetime.now(timezone.utc) - parsed).total_seconds() / 86400.0, 0.0)
    except ValueError:
        return 0.5

    window = max(float(settings.RECENCY_WINDOW_DAYS), 1.0)
    return max(0.0, min(1.0, 1.0 - (age_days / window)))


def build_rank_score(relevance_score: float, metadata: Dict[str, Any]) -> float:
    """
    Композитный score retrieval по факторам из спецификации:
    relevance, importance, reliability, recency, frequency.

    Все коэффициенты берутся из env-конфига, чтобы избежать магических констант.
    """
    relevance = max(0.0, min(1.0, relevance_score))
    importance = max(0.0, min(1.0, _safe_float(metadata.get("importance"), 0.5)))
    reliability = max(0.0, min(1.0, _safe_float(metadata.get("reliability"), 0.5)))
    frequency = max(0.0, min(1.0, _safe_float(metadata.get("frequency"), 0.5)))
    recency = _recency_score(str(metadata.get("created_at", "")))

    total = (
        relevance * settings.RANK_WEIGHT_RELEVANCE
        + importance * settings.RANK_WEIGHT_IMPORTANCE
        + reliability * settings.RANK_WEIGHT_RELIABILITY
        + recency * settings.RANK_WEIGHT_RECENCY
        + frequency * settings.RANK_WEIGHT_FREQUENCY
    )
    return round(max(0.0, min(1.0, total)), 4)


def _clamp01(value: float) -> float:
    return max(0.0, min(1.0, value))


def blend_relevance_scores(semantic_relevance: float, keyword_relevance: float) -> float:
    """
    Объединяет семантическую и keyword-релевантность в единый сигнал relevance.

    Зачем:
    - семантика лучше ловит смысл запроса;
    - keyword-сигнал повышает точность при совпадении терминов/идентификаторов.

    Все коэффициенты управляются через env (`SEARCH_SEMANTIC_WEIGHT`, `SEARCH_KEYWORD_WEIGHT`)
    и не захардкожены в бизнес-логике.
    """
    semantic = _clamp01(float(semantic_relevance))
    keyword = _clamp01(float(keyword_relevance))
    semantic_weight = max(float(settings.SEARCH_SEMANTIC_WEIGHT), 0.0)
    keyword_weight = max(float(settings.SEARCH_KEYWORD_WEIGHT), 0.0)
    total_weight = semantic_weight + keyword_weight
    if total_weight <= 0.0:
        return semantic
    blended = (semantic * semantic_weight + keyword * keyword_weight) / total_weight
    return round(_clamp01(blended), 4)
