import json
import logging
import sys
import uuid
from contextlib import asynccontextmanager
from contextvars import ContextVar
from typing import List

from fastapi import FastAPI, HTTPException, Request, status
from fastapi.responses import JSONResponse
from starlette.middleware.base import BaseHTTPMiddleware

from .config import settings
from .memory import memory_store
from .ttl import TTLManager
from . import models

correlation_id_var: ContextVar[str] = ContextVar("correlation_id", default="")


class JSONFormatter(logging.Formatter):
    """JSON-форматтер для структурированного логирования."""
    def format(self, record: logging.LogRecord) -> str:
        log_data = {
            "time": self.formatTime(record, self.datefmt),
            "level": record.levelname,
            "сервис": "memory-service",
            "msg": record.getMessage(),
        }
        cid = correlation_id_var.get("")
        if cid:
            log_data["correlation_id"] = cid
        if record.exc_info and record.exc_info[1]:
            log_data["ошибка"] = str(record.exc_info[1])
        return json.dumps(log_data, ensure_ascii=False)


def setup_logging() -> None:
    """Настройка структурированного JSON-логирования."""
    handler = logging.StreamHandler(sys.stdout)
    handler.setFormatter(JSONFormatter())
    root = logging.getLogger()
    root.handlers.clear()
    root.addHandler(handler)
    root.setLevel(logging.INFO)


setup_logging()
logger = logging.getLogger(__name__)


class CorrelationIDMiddleware(BaseHTTPMiddleware):
    """Миддлвар для пропагации X-Request-ID через все запросы."""
    async def dispatch(self, request: Request, call_next):
        cid = request.headers.get("X-Request-ID", str(uuid.uuid4()))
        correlation_id_var.set(cid)
        response = await call_next(request)
        response.headers["X-Request-ID"] = cid
        return response


ttl_manager = TTLManager(memory_store)


@asynccontextmanager
async def lifespan(app: FastAPI):
    """
    Действия при запуске и остановке приложения.
    """
    logger.info("Сервис памяти запущен")
    logger.info(f"Статистика: фактов {memory_store.get_stats()['facts_count']}, "
                f"файловых чанков {memory_store.get_stats()['files_count']}")
    ttl_manager.start_scheduler()
    yield
    ttl_manager.stop_scheduler()
    logger.info("Сервис памяти остановлен")


app = FastAPI(
    title="Memory Service (RAG)",
    description="Сервис для долговременной памяти агентов: добавление фактов, поиск, индексация файлов",
    version="1.0.0",
    lifespan=lifespan
)
app.add_middleware(CorrelationIDMiddleware)


@app.get("/health", tags=["Health"])
async def health_check():
    """Проверка работоспособности сервиса."""
    return {"status": "ok", "service": "memory-service"}


@app.post("/facts", response_model=models.FactAddResponse, tags=["Facts"])
async def add_fact(request: models.FactAddRequest):
    """
    Добавить новый факт в память.
    """
    try:
        fact_id = memory_store.add_fact(request.text, request.metadata)
        return models.FactAddResponse(id=fact_id)
    except Exception as e:
        logger.exception("Ошибка при добавлении факта")
        raise HTTPException(status_code=500, detail=str(e))


@app.post("/search", response_model=models.SearchResponse, tags=["Search"])
async def search(request: models.SearchRequest):
    """
    Поиск релевантных фактов и/или фрагментов файлов.
    """
    try:
        results = memory_store.search_facts(
            query=request.query,
            top_k=request.top_k,
            agent_name=request.agent_name,
            include_files=request.include_files,
            workspace_id=request.workspace_id,
        )
        return models.SearchResponse(results=results, count=len(results))
    except Exception as e:
        logger.exception("Ошибка при поиске")
        raise HTTPException(status_code=500, detail=str(e))


@app.post("/files/chunks", response_model=models.FileChunkAddResponse, tags=["Files"])
async def add_file_chunk(request: models.FileChunkAddRequest):
    """
    Добавить фрагмент файла в память.
    """
    try:
        chunk_id = memory_store.add_file_chunk(request.text, request.metadata)
        return models.FileChunkAddResponse(id=chunk_id)
    except Exception as e:
        logger.exception("Ошибка при добавлении фрагмента файла")
        raise HTTPException(status_code=500, detail=str(e))


@app.get("/files", tags=["Files"])
async def list_files():
    """
    Получить список всех загруженных файлов с количеством чанков.
    """
    try:
        files = memory_store.list_files()
        return files
    except Exception as e:
        logger.exception("Ошибка при получении списка файлов")
        raise HTTPException(status_code=500, detail=str(e))


@app.delete("/files", tags=["Files"])
async def delete_file_by_name(name: str):
    """
    Удалить все фрагменты файла по имени.
    """
    try:
        deleted = memory_store.delete_file_by_name(name)
        return {"deleted_count": deleted, "status": "ok"}
    except Exception as e:
        logger.exception("Ошибка при удалении файла по имени")
        raise HTTPException(status_code=500, detail=str(e))


@app.delete("/files/{file_id}", response_model=models.FileDeleteResponse, tags=["Files"])
async def delete_file(file_id: str):
    """
    Удалить все фрагменты, принадлежащие указанному файлу.
    """
    try:
        deleted = memory_store.delete_file_chunks(file_id)
        return models.FileDeleteResponse(deleted_count=deleted)
    except Exception as e:
        logger.exception("Ошибка при удалении фрагментов файла")
        raise HTTPException(status_code=500, detail=str(e))


@app.get("/stats", response_model=models.StatsResponse, tags=["Stats"])
async def get_stats():
    """Получить статистику по коллекциям."""
    stats = memory_store.get_stats()
    return models.StatsResponse(**stats)


# === Эндпоинты системы обучения агентов ===
# Система обучения позволяет каждой модели LLM накапливать знания
# из диалогов с пользователем. Каждая модель имеет свою отдельную
# базу знаний, которая используется для обогащения контекста
# при каждом новом запросе.


@app.post("/learnings", response_model=models.LearningAddResponse, tags=["Learnings"])
async def add_learning(request: models.LearningAddRequest):
    """
    Добавить новое знание для модели LLM.
    
    Вызывается автоматически agent-service после каждого успешного
    диалога. Извлечённые знания сохраняются в ChromaDB
    с привязкой к конкретной модели.
    """
    try:
        learning_id = memory_store.add_learning(
            text=request.text,
            model_name=request.model_name,
            agent_name=request.agent_name,
            category=request.category,
            metadata=request.metadata,
            workspace_id=request.workspace_id,
        )
        learning_meta = memory_store.get_learning_metadata(learning_id)
        return models.LearningAddResponse(
            id=learning_id,
            version=int(learning_meta.get("version", 1)),
            learning_key=str(learning_meta.get("learning_key", "")),
            conflict_detected=bool(learning_meta.get("conflict_detected", False)),
            previous_version_id=learning_meta.get("previous_version_id"),
        )
    except Exception as e:
        logger.exception("Ошибка при добавлении знания")
        raise HTTPException(status_code=500, detail=str(e))


@app.post("/learnings/search", response_model=models.LearningSearchResponse, tags=["Learnings"])
async def search_learnings(request: models.LearningSearchRequest):
    """
    Поиск релевантных знаний для модели.
    
    Вызывается agent-service перед каждым запросом к LLM.
    Найденные знания добавляются в системный промпт
    для обогащения контекста модели.
    """
    try:
        results = memory_store.search_learnings(
            query=request.query,
            model_name=request.model_name,
            top_k=request.top_k,
            category=request.category,
            workspace_id=request.workspace_id,
        )
        return models.LearningSearchResponse(
            results=results,
            count=len(results),
            model_name=request.model_name
        )
    except Exception as e:
        logger.exception("Ошибка при поиске знаний")
        raise HTTPException(status_code=500, detail=str(e))


@app.get("/learnings/stats", response_model=models.LearningStatsResponse, tags=["Learnings"])
async def get_learning_stats():
    """
    Получить статистику обучения по моделям.
    
    Показывает общее количество знаний, разбивку по моделям
    и категориям. Используется для мониторинга обучения.
    """
    try:
        stats = memory_store.get_learning_stats()
        return models.LearningStatsResponse(**stats)
    except Exception as e:
        logger.exception("Ошибка получения статистики обучения")
        raise HTTPException(status_code=500, detail=str(e))


@app.delete("/learnings/{model_name}", tags=["Learnings"])
async def delete_learnings(model_name: str, category: str = None, workspace_id: str = None):
    """
    Удалить знания конкретной модели.
    
    Используется для сброса обучения модели,
    например, при смене модели на агенте.
    """
    try:
        deleted = memory_store.delete_model_learnings(model_name, category, workspace_id)
        return {"deleted_count": deleted, "model_name": model_name, "status": "ok"}
    except Exception as e:
        logger.exception("Ошибка удаления знаний")
        raise HTTPException(status_code=500, detail=str(e))


@app.get("/learnings/versions/{model_name}", response_model=models.LearningVersionsResponse, tags=["Learnings"])
async def get_learning_versions(model_name: str, category: str = None, workspace_id: str = None):
    """Получить историю версий знаний модели с фильтрами по категории/workspace."""
    try:
        versions = memory_store.list_learning_versions(
            model_name=model_name,
            category=category,
            workspace_id=workspace_id,
        )
        return models.LearningVersionsResponse(
            model_name=model_name,
            category=category,
            workspace_id=workspace_id,
            versions=versions,
            count=len(versions),
        )
    except Exception as e:
        logger.exception("Ошибка получения истории версий знаний")
        raise HTTPException(status_code=500, detail=str(e))


@app.get("/audit/logs", response_model=models.AuditLogsResponse, tags=["Maintenance"])
async def get_audit_logs(top_k: int = 100, workspace_id: str = None, model_name: str = None):
    """Получить аудит операций памяти с фильтрами по workspace/model."""
    try:
        logs = memory_store.list_audit_logs(top_k=top_k, workspace_id=workspace_id, model_name=model_name)
        return models.AuditLogsResponse(logs=logs, count=len(logs))
    except Exception as e:
        logger.exception("Ошибка получения audit logs")
        raise HTTPException(status_code=500, detail=str(e))


@app.get("/metrics/retrieval", response_model=models.RetrievalMetricsResponse, tags=["Maintenance"])
async def get_retrieval_metrics():
    """Получить агрегированные метрики retrieval (latency/errors/объём выдачи)."""
    try:
        metrics = memory_store.get_retrieval_metrics()
        return models.RetrievalMetricsResponse(**metrics)
    except Exception as e:
        logger.exception("Ошибка получения retrieval metrics")
        raise HTTPException(status_code=500, detail=str(e))


@app.post("/reindex", tags=["Maintenance"])
async def reindex(collection: str = "all", force: bool = False):
    """Запустить переиндексацию документов."""
    try:
        if collection == "all":
            total = 0
            for col in ["facts", "files", "learnings"]:
                total += ttl_manager.reindex_collection(col, force=force)
            return {"reindexed_count": total, "status": "ok"}
        else:
            count = ttl_manager.reindex_collection(collection, force=force)
            return {"reindexed_count": count, "status": "ok"}
    except Exception as e:
        logger.exception("Ошибка переиндексации")
        raise HTTPException(status_code=500, detail=str(e))


@app.get("/ttl/expired", tags=["Maintenance"])
async def get_expired(collection: str = "all"):
    """Получить список документов с истёкшим TTL."""
    from .ttl import DEFAULT_FACTS_TTL, DEFAULT_FILES_TTL, DEFAULT_LEARNINGS_TTL
    ttl_map = {"facts": DEFAULT_FACTS_TTL, "files": DEFAULT_FILES_TTL, "learnings": DEFAULT_LEARNINGS_TTL}
    collections = [collection] if collection != "all" else list(ttl_map.keys())
    result = {"expired_count": 0, "by_collection": {}}
    for col in collections:
        ttl = ttl_map.get(col, 0)
        expired = ttl_manager.get_expired_ids(col, ttl)
        result["by_collection"][col] = len(expired)
        result["expired_count"] += len(expired)
    return result


@app.delete("/ttl/expired", tags=["Maintenance"])
async def cleanup_expired(collection: str = "all"):
    """Удалить документы с истёкшим TTL."""
    result = ttl_manager.cleanup_expired(collection)
    return {"deleted_count": result["total_deleted"], "status": "ok"}


@app.get("/reindex/status", tags=["Maintenance"])
async def reindex_status():
    """Проверить, нужна ли переиндексация."""
    return ttl_manager.check_reindex_needed()


@app.exception_handler(Exception)
async def generic_exception_handler(request, exc):
    """Глобальный обработчик исключений."""
    logger.exception(f"Необработанное исключение: {exc}")
    return JSONResponse(
        status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
        content={"error": "Internal server error", "detail": str(exc)}
    )
