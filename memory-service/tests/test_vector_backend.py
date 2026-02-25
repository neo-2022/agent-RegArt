import pytest

from app.vector_backend import (
    VECTOR_BACKEND_QDRANT,
    resolve_vector_backend,
)


def test_resolve_vector_backend_uses_default_qdrant_for_empty_value():
    """Пустое значение должно безопасно приводиться к backend по умолчанию."""
    assert resolve_vector_backend(None) == VECTOR_BACKEND_QDRANT
    assert resolve_vector_backend("") == VECTOR_BACKEND_QDRANT


def test_resolve_vector_backend_accepts_qdrant_case_insensitive():
    """Поддерживаемое значение backend должно приниматься в разных регистрах."""
    assert resolve_vector_backend("qdrant") == VECTOR_BACKEND_QDRANT
    assert resolve_vector_backend("QDRANT") == VECTOR_BACKEND_QDRANT


def test_resolve_vector_backend_rejects_legacy_or_unknown_values():
    """Некорректный/legacy backend должен давать раннюю ошибку конфигурации."""
    with pytest.raises(ValueError, match="VECTOR_BACKEND"):
        resolve_vector_backend("chroma")
