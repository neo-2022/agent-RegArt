import pytest

from app.vector_backend import (
    VECTOR_BACKEND_CHROMA,
    VECTOR_BACKEND_QDRANT,
    resolve_vector_backend,
)


def test_resolve_vector_backend_uses_default_chroma_for_empty_value():
    """Пустое значение должно безопасно приводиться к backend по умолчанию."""
    assert resolve_vector_backend(None) == VECTOR_BACKEND_CHROMA
    assert resolve_vector_backend("") == VECTOR_BACKEND_CHROMA


def test_resolve_vector_backend_accepts_supported_values_case_insensitive():
    """Поддерживаемые значения backend должны приниматься в разных регистрах."""
    assert resolve_vector_backend("chroma") == VECTOR_BACKEND_CHROMA
    assert resolve_vector_backend("QDRANT") == VECTOR_BACKEND_QDRANT


def test_resolve_vector_backend_rejects_unknown_values():
    """Некорректный backend должен давать раннюю и явную ошибку конфигурации."""
    with pytest.raises(ValueError, match="VECTOR_BACKEND"):
        resolve_vector_backend("milvus")
