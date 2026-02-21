# Модуль векторного хранилища (Chroma)

## Обзор

Модуль `vector-store` обеспечивает интеграцию с Chroma - векторной базой данных для семантического поиска. Chroma позволяет эффективно хранить и искать эмбеддинги документов.

## Компоненты

### ChromaStore

Основная структура для работы с Chroma:

```go
type ChromaStore struct {
    URL string  // URL Chroma сервера
}
```

## Методы

### NewChromaStore(url string) *ChromaStore

Создаёт новое подключение к Chroma.

Параметры:
- `url` - URL Chroma сервера (например, `http://localhost:8000`)

### AddDocuments(docs []map[string]interface{}) error

Добавляет документы в коллекцию.

Структура документа:
```go
{
    "id": "unique-id",
    "content": "текст документа",
    "metadata": {
        "title": "Заголовок",
        "source": "источник",
        "created_at": "2024-01-01"
    }
}
```

### Search(query string, n int) ([]map[string]interface{}, error)

Выполняет семантический поиск по запросу.

Параметры:
- `query` - текстовый запрос
- `n` - количество результатов

Возвращает массив найденных документов с метаданными и расстоянием.

## Использование

### Подключение к Chroma

```go
store := vector_store.NewChromaStore("http://localhost:8000")
```

### Добавление документов

```go
docs := []map[string]interface{}{
    {
        "id": "doc-1",
        "content": "Это пример документа для поиска",
        "metadata": map[string]interface{}{
            "title": "Пример",
            "source": "тест",
        },
    },
}

err := store.AddDocuments(docs)
if err != nil {
    log.Fatal(err)
}
```

### Поиск

```go
results, err := store.Search("документ поиск", 5)
if err != nil {
    log.Fatal(err)
}

for _, r := range results {
    fmt.Printf("ID: %s, Distance: %f\n", r["id"], r["distance"])
}
```

## API Chroma

Модуль взаимодействует с Chroma REST API:

### Создание коллекции

```
POST /api/v1/collections
{
    "name": "rag_docs",
    "get_or_create": true
}
```

### Добавление документов

```
POST /api/v1/collections/rag_docs/add
{
    "ids": ["id-1", "id-2"],
    "embeddings": [[0.1, 0.2, ...], [0.3, 0.4, ...]],
    "metadatas": [{"title": "Doc 1"}, {"title": "Doc 2"}],
    "documents": ["Текст 1", "Текст 2"]
}
```

### Поиск

```
POST /api/v1/collections/rag_docs/query
{
    "query_embeddings": [[0.1, 0.2, ...]],
    "n_results": 5
}
```

## Интеграция с RAG

В RAG модуле Chroma используется для:

1. **Хранение эмбеддингов** - документы индексируются при загрузке
2. **Семантический поиск** - быстрый поиск по косинусному сходству
3. **Обновление** - инкрементальное добавление новых документов

## Планы развития

### Поддержка нескольких коллекций

- Отдельные коллекции для разных типов документов
- Кросс-коллекционный поиск

### Weaviate/Qdrant адаптеры

Планируется добавить поддержку других векторных баз:
- Weaviate
- Qdrant  
- Pinecone
- Milvus

### Гибридный поиск

Комбинирование:
- Dense embeddings (Chroma) - семантика
- Sparse (BM25) - точное совпадение слов
- Reranking - переранжирование результатов

## Метрики

- `chroma_query_latency_ms` - время запроса к Chroma
- `chroma_indexed_docs` - количество проиндексированных документов
- `chroma_cache_hit` - использование кэша
