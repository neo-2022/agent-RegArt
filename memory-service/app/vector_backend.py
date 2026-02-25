"""Утилиты выбора backend векторного хранилища для memory-service."""

from typing import Final

VECTOR_BACKEND_QDRANT: Final[str] = "qdrant"
SUPPORTED_VECTOR_BACKENDS: Final[tuple[str, ...]] = (VECTOR_BACKEND_QDRANT,)


def resolve_vector_backend(raw_backend: str | None) -> str:
    """
    Нормализует и валидирует значение backend векторного хранилища.

    На текущем этапе поддерживается только Qdrant.
    """
    normalized = (raw_backend or VECTOR_BACKEND_QDRANT).strip().lower()
    if normalized in SUPPORTED_VECTOR_BACKENDS:
        return normalized
    raise ValueError(
        "Неподдерживаемый VECTOR_BACKEND: "
        f"{raw_backend!r}. Допустимые значения: {', '.join(SUPPORTED_VECTOR_BACKENDS)}"
    )
