"""Утилиты выбора backend векторного хранилища для memory-service."""

from typing import Final

VECTOR_BACKEND_CHROMA: Final[str] = "chroma"
VECTOR_BACKEND_QDRANT: Final[str] = "qdrant"
SUPPORTED_VECTOR_BACKENDS: Final[tuple[str, ...]] = (
    VECTOR_BACKEND_CHROMA,
    VECTOR_BACKEND_QDRANT,
)


def resolve_vector_backend(raw_backend: str | None) -> str:
    """
    Нормализует и валидирует значение backend векторного хранилища.

    Почему это важно:
    - исключаем магические строки в коде и централизуем поддерживаемые значения;
    - даём раннюю и явную ошибку при неверной конфигурации окружения.
    """
    normalized = (raw_backend or VECTOR_BACKEND_CHROMA).strip().lower()
    if normalized in SUPPORTED_VECTOR_BACKENDS:
        return normalized
    raise ValueError(
        "Неподдерживаемый VECTOR_BACKEND: "
        f"{raw_backend!r}. Допустимые значения: {', '.join(SUPPORTED_VECTOR_BACKENDS)}"
    )
