import logging
from contextlib import asynccontextmanager
from typing import List

from fastapi import FastAPI, HTTPException, status
from fastapi.responses import JSONResponse

from .config import settings
from .memory import memory_store
from . import models

logger = logging.getLogger(__name__)


@asynccontextmanager
async def lifespan(app: FastAPI):
    """
    Действия при запуске и остановке приложения.
    """
    # При запуске: проверяем, что модель эмбеддингов загружена (она уже загружена в memory_store)
    logger.info("Сервис памяти запущен")
    logger.info(f"Статистика: фактов {memory_store.get_stats()['facts_count']}, "
                f"файловых чанков {memory_store.get_stats()['files_count']}")
    yield
    # При остановке: можно закрыть соединения, но ChromaDB не требует явного закрытия
    logger.info("Сервис памяти остановлен")


app = FastAPI(
    title="Memory Service (RAG)",
    description="Сервис для долговременной памяти агентов: добавление фактов, поиск, индексация файлов",
    version="1.0.0",
    lifespan=lifespan
)


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
            include_files=request.include_files
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
            metadata=request.metadata
        )
        return models.LearningAddResponse(id=learning_id)
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
            category=request.category
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
async def delete_learnings(model_name: str, category: str = None):
    """
    Удалить знания конкретной модели.
    
    Используется для сброса обучения модели,
    например, при смене модели на агенте.
    """
    try:
        deleted = memory_store.delete_model_learnings(model_name, category)
        return {"deleted_count": deleted, "model_name": model_name, "status": "ok"}
    except Exception as e:
        logger.exception("Ошибка удаления знаний")
        raise HTTPException(status_code=500, detail=str(e))


@app.exception_handler(Exception)
async def generic_exception_handler(request, exc):
    """Глобальный обработчик исключений."""
    logger.exception(f"Необработанное исключение: {exc}")
    return JSONResponse(
        status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
        content={"error": "Internal server error", "detail": str(exc)}
    )
