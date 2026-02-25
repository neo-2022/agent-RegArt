from pydantic import BaseModel, Field
from typing import Optional, List, Dict, Any


# Лимиты размеров входных данных
MAX_TEXT_LENGTH = 50000
MAX_QUERY_LENGTH = 5000


class FactAddRequest(BaseModel):
    """Запрос на добавление факта."""
    text: str = Field(..., description="Текст факта", min_length=1, max_length=MAX_TEXT_LENGTH)
    metadata: Optional[Dict[str, Any]] = Field(default_factory=dict, description="Метаданные")


class FactAddResponse(BaseModel):
    """Ответ на добавление факта."""
    id: str
    status: str = "ok"
    message: str = "Fact added"


class SearchRequest(BaseModel):
    """Запрос на поиск."""
    query: str = Field(..., description="Поисковый запрос", min_length=1, max_length=MAX_QUERY_LENGTH)
    top_k: Optional[int] = Field(5, description="Количество результатов", ge=1, le=50)
    agent_name: Optional[str] = Field(None, description="Фильтр по имени агента")
    workspace_id: Optional[str] = Field(None, description="Фильтр по workspace (изоляция контекста)")
    include_files: bool = Field(False, description="Включать ли фрагменты файлов")
    min_priority: Optional[str] = Field(None, description="Минимальный приоритет памяти: critical|pinned|reinforced|normal|archived")


class SearchResultItem(BaseModel):
    """Структурированный результат поиска."""
    text: str = Field(..., description="Текст найденного документа")
    score: float = Field(0.0, description="Оценка релевантности (0..1)")
    source: str = Field("facts", description="Источник: facts, files, learnings")
    metadata: Dict[str, Any] = Field(default_factory=dict, description="Метаданные документа")


class SearchResponse(BaseModel):
    """Ответ на поиск — структурированный формат с оценками и метаданными."""
    results: List[SearchResultItem] = Field(default_factory=list)
    count: int


class FileChunkAddRequest(BaseModel):
    """Запрос на добавление фрагмента файла."""
    text: str = Field(..., description="Текст фрагмента", min_length=1, max_length=MAX_TEXT_LENGTH)
    metadata: Dict[str, Any] = Field(..., description="Метаданные (agent, filename, file_id, chunk)")


class FileChunkAddResponse(BaseModel):
    """Ответ на добавление фрагмента."""
    id: str
    status: str = "ok"


class FileDeleteRequest(BaseModel):
    """Запрос на удаление фрагментов файла."""
    file_id: str = Field(..., description="Идентификатор файла")


class FileDeleteResponse(BaseModel):
    """Ответ на удаление."""
    deleted_count: int
    status: str = "ok"


class StatsResponse(BaseModel):
    """Статистика."""
    facts_count: int
    files_count: int
    learnings_count: int = 0


class ErrorResponse(BaseModel):
    """Ответ с ошибкой."""
    error: str
    detail: Optional[str] = None


# === Модели для системы обучения агентов ===
# Система обучения позволяет каждой модели LLM накапливать знания
# из взаимодействий с пользователем. Знания привязаны к конкретной модели
# (model_name), а не к агенту, потому что одна и та же модель может
# использоваться разными агентами, и знания должны переноситься.

class LearningAddRequest(BaseModel):
    """Запрос на добавление знания (факта, полученного в процессе обучения).
    
    Знание извлекается автоматически из диалога после каждого успешного
    взаимодействия. Привязывается к конкретной модели LLM.
    """
    text: str = Field(..., description="Текст знания (факт, правило, предпочтение пользователя)", min_length=1, max_length=MAX_TEXT_LENGTH)
    model_name: str = Field(..., description="Имя модели LLM, которая получила это знание", min_length=1)
    agent_name: str = Field(..., description="Имя агента, в контексте которого получено знание", min_length=1)
    workspace_id: Optional[str] = Field(None, description="Идентификатор workspace для изоляции памяти", min_length=1)
    category: str = Field("general", description="Категория знания: general, preference, fact, skill, correction")
    metadata: Optional[Dict[str, Any]] = Field(default_factory=dict, description="Дополнительные метаданные")


class ContradictionItem(BaseModel):
    """Элемент обнаруженного противоречия (Eternal RAG: раздел 8)."""
    id: str = Field(..., description="ID существующего знания, с которым обнаружено противоречие")
    text: str = Field(..., description="Текст существующего знания")
    similarity: float = Field(..., description="Косинусная близость [0..1]")
    learning_key: str = Field("", description="Ключ знания")


class LearningAddResponse(BaseModel):
    """Ответ на добавление знания."""
    id: str
    version: int = 1
    learning_key: str
    conflict_detected: bool = False
    previous_version_id: Optional[str] = None
    contradictions: List[ContradictionItem] = Field(default_factory=list, description="Обнаруженные противоречия с существующими знаниями")
    status: str = "ok"
    message: str = "Learning added"


class LearningSearchRequest(BaseModel):
    """Запрос на поиск знаний для конкретной модели.
    
    Поиск выполняется по семантической близости к запросу,
    с фильтрацией по имени модели.
    """
    query: str = Field(..., description="Поисковый запрос (контекст текущего диалога)", min_length=1, max_length=MAX_QUERY_LENGTH)
    model_name: str = Field(..., description="Имя модели, для которой ищем знания", min_length=1)
    workspace_id: Optional[str] = Field(None, description="Идентификатор workspace для изоляции памяти", min_length=1)
    top_k: Optional[int] = Field(5, description="Количество результатов", ge=1, le=20)
    category: Optional[str] = Field(None, description="Фильтр по категории знания")
    min_priority: Optional[str] = Field(None, description="Минимальный приоритет знаний: critical|pinned|reinforced|normal|archived")


class LearningSearchResponse(BaseModel):
    """Ответ на поиск знаний — структурированный формат."""
    results: List[SearchResultItem] = Field(default_factory=list)
    count: int
    model_name: str


class LearningStatsResponse(BaseModel):
    """Статистика обучения по моделям."""
    total_learnings: int
    by_model: Dict[str, int] = Field(default_factory=dict)
    by_category: Dict[str, int] = Field(default_factory=dict)


class LearningDeleteRequest(BaseModel):
    """Запрос на удаление знаний модели."""
    model_name: str = Field(..., description="Имя модели, знания которой удалить")
    category: Optional[str] = Field(None, description="Удалить только определённую категорию (опционально)")


class LearningVersionItem(BaseModel):
    """Элемент истории версий знания."""
    id: str
    version: int
    status: str
    text: str
    metadata: Dict[str, Any] = Field(default_factory=dict)


class LearningVersionsResponse(BaseModel):
    """Ответ со списком версий знания по модели/категории/workspace."""
    model_name: str
    category: Optional[str] = None
    workspace_id: Optional[str] = None
    versions: List[LearningVersionItem] = Field(default_factory=list)
    count: int


class AuditLogItem(BaseModel):
    """Запись аудита операций памяти."""
    id: str
    event_type: str
    model_name: Optional[str] = None
    workspace_id: Optional[str] = None
    learning_id: Optional[str] = None
    created_at: str
    details: Dict[str, Any] = Field(default_factory=dict)


class AuditLogsResponse(BaseModel):
    """Ответ со списком событий аудита."""
    logs: List[AuditLogItem] = Field(default_factory=list)
    count: int


class RetrievalMetricsResponse(BaseModel):
    """Агрегированные метрики retrieval для мониторинга производительности."""
    search_requests_total: int
    search_errors_total: int
    search_results_total: int
    search_latency_ms_avg: float


class BackupChecksResponse(BaseModel):
    """Результат инфраструктурных backup-checks (наличие инструментов и флагов)."""
    pg_dump_available: bool
    qdrant_snapshot_enabled: bool
    neo4j_backup_enabled: bool
    minio_versioning_enabled: bool
    restore_test_enabled: bool


class FileRenameRequest(BaseModel):
    """Запрос на переименование файла в RAG-базе знаний."""
    old_name: str = Field(..., description="Текущее имя файла", min_length=1)
    new_name: str = Field(..., description="Новое имя файла", min_length=1)


class FileRenameResponse(BaseModel):
    """Ответ на переименование файла."""
    old_name: str
    new_name: str
    chunks_updated: int
    status: str = "ok"


class EmbeddingStatusResponse(BaseModel):
    """Статус модели эмбеддингов для мониторинга (Eternal RAG: раздел 5.8)."""
    model_name: str = Field(..., description="Имя модели эмбеддингов")
    model_version: str = Field(..., description="Версия модели")
    vector_size: int = Field(..., description="Размерность вектора")
    status: str = Field("loaded", description="Статус: loaded / error")
    collections: Dict[str, int] = Field(default_factory=dict, description="Количество документов по коллекциям")


# === Модели для расширенного управления файлами RAG ===
# Поддержка перемещения, мягкого удаления, закрепления и поиска по содержимому.

class FileMoveRequest(BaseModel):
    """Запрос на перемещение файла между папками в RAG-базе знаний."""
    file_name: str = Field(..., description="Имя файла для перемещения", min_length=1)
    target_folder: str = Field(..., description="Целевая папка", min_length=1)


class FileMoveResponse(BaseModel):
    """Ответ на перемещение файла."""
    old_path: str
    new_path: str
    chunks_updated: int
    status: str = "ok"


class FileSoftDeleteRequest(BaseModel):
    """Запрос на мягкое удаление файла (пометка deleted_at вместо физического удаления)."""
    file_name: str = Field(..., description="Имя файла для мягкого удаления", min_length=1)


class FileSoftDeleteResponse(BaseModel):
    """Ответ на мягкое удаление файла."""
    file_name: str
    chunks_marked: int
    status: str = "ok"


class FileRestoreRequest(BaseModel):
    """Запрос на восстановление мягко удалённого файла."""
    file_name: str = Field(..., description="Имя файла для восстановления", min_length=1)


class FileRestoreResponse(BaseModel):
    """Ответ на восстановление файла."""
    file_name: str
    chunks_restored: int
    status: str = "ok"


class FilePinRequest(BaseModel):
    """Запрос на закрепление файла (pinned — показывается первым, не удаляется по TTL)."""
    file_name: str = Field(..., description="Имя файла для закрепления", min_length=1)


class FilePinResponse(BaseModel):
    """Ответ на закрепление/открепление файла."""
    file_name: str
    pinned: bool
    chunks_updated: int
    status: str = "ok"


class FileContentSearchRequest(BaseModel):
    """Запрос на семантический поиск внутри файлов RAG-базы."""
    query: str = Field(..., description="Поисковый запрос по содержимому файлов", min_length=1, max_length=MAX_QUERY_LENGTH)
    top_k: int = Field(10, description="Максимум результатов", ge=1, le=50)
    folder: Optional[str] = Field(None, description="Фильтр по папке")


class FileContentSearchResult(BaseModel):
    """Результат поиска по содержимому файла."""
    file_name: str
    chunk_text: str
    score: float
    chunk_index: int = 0
    metadata: Dict[str, Any] = Field(default_factory=dict)


class FileContentSearchResponse(BaseModel):
    """Ответ на поиск по содержимому файлов."""
    results: List[FileContentSearchResult] = Field(default_factory=list)
    count: int
    query: str


class ContradictionListItem(BaseModel):
    """Элемент списка обнаруженных противоречий между знаниями."""
    new_learning_id: str = Field(..., description="ID нового знания")
    existing_learning_id: str = Field(..., description="ID существующего знания")
    new_text: str = Field(..., description="Текст нового знания")
    existing_text: str = Field(..., description="Текст существующего знания")
    similarity: float = Field(..., description="Косинусная близость")
    model_name: str = Field("", description="Модель, к которой относятся знания")
    detected_at: str = Field("", description="Время обнаружения")


class ContradictionsResponse(BaseModel):
    """Ответ со списком обнаруженных противоречий."""
    contradictions: List[ContradictionListItem] = Field(default_factory=list)
    count: int
