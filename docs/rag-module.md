# RAG модуль

## Обзор

Модуль RAG (Retrieval-Augmented Generation) обеспечивает полноценный поиск документов и формирование контекста для LLM.

## Компоненты

### RagDoc - Структура документа

Представляет документ в RAG системе с метаданными:
- `ID` - уникальный идентификатор
- `Title` - заголовок документа
- `Content` - текстовое содержимое
- `Source` - источник документа (prompts, uploads, memory)
- `CreatedAt` - время создания
- `Embedding` - векторное представление документа

### SearchResult - Результат поиска

Структура результата поиска с оценкой релевантности:
- `Doc` - найденный документ
- `Score` - оценка релевантности (0-1)
- `Rank` - позиция в рейтинге

### Config - Конфигурация

Параметры RAG системы:
- `DBHost`, `DBPort`, `DBUser`, `DBPassword`, `DBName` - подключение к PostgreSQL
- `ChromaURL` - URL векторной базы ChromA
- `EmbeddingModel` - модель для эмбеддингов
- `TopK` - количество результатов поиска
- `EnableMultiHop` - включить multi-hop поиск

## Основные функции

### NewDBRetriever(config *Config) *DBRetriever

Создаёт новый экземпляр RAG ретривера с заданной конфигурацией.

### EnsureTable() error

Создаёт таблицу rag_docs в базе данных PostgreSQL (если ещё не существует).

### SeedFromLocalCorpus(paths []string) error

Загружает документы из локальных источников:
- `agent-service/prompts/` - промпты агентов
- `agent-service/uploads/` - загруженные файлы

Для каждого документа вычисляется эмбеддинг и сохраняется вместе с содержимым.

### Search(query string, topK int) ([]SearchResult, error)

Выполняет поиск документов по запросу:
1. Вычисляет эмбеддинг запроса
2. Пытается использовать ChromA для семантического поиска
3. При недоступности ChromA использует fallback (эмуляция)

Возвращает топ-K наиболее релевантных документов с оценками.

### MultiHopSearch(query string, hops int) ([]SearchResult, error)

Выполняет многоходовый (multi-hop) поиск:
1. Первый хоп - поиск по оригинальному запросу
2. Последующие хопы - расширение запроса контекстом из предыдущих результатов
3. Дедупликация и пересортировка результатов

Параметр `hops` определяет количество хопов (по умолчанию 2).

## Алгоритмы

### Векторный поиск (Chroma)

При наличии доступа к ChromA используется семантический поиск:
1. Отправляется запрос с эмбеддингом
2. ChromA возвращает документы по косинусному сходству
3. Результаты сортируются по убыванию релевантности

### Hybrid Search (план)

Планируется реализация гибридного поиска:
- BM25 для точного совпадения слов
- Семантический поиск по эмбеддингам
- Комбинированный скор: 0.3 * BM25 + 0.7 * semantic

### Косинусное сходство

```go
CosineSimilarity(a, b []float64) float64
```

Вычисляет косинусное сходство между двумя векторами:
- Скалярное произведение / (норма a * норма b)

## Пример использования

```go
config := &rag.Config{
    ChromaURL:  "http://localhost:8000",
    TopK:       5,
    EnableMultiHop: true,
}

retriever := rag.NewDBRetriever(config)

// Инициализация БД
retriever.EnsureTable()

// Загрузка документов
retriever.SeedFromLocalCorpus([]string{"prompts", "uploads"})

// Поиск
results, err := retriever.Search("как написать код на питоне", 5)
for _, r := range results {
    fmt.Printf("Doc: %s, Score: %.2f\n", r.Doc.Title, r.Score)
}
```

## Интеграция с LLM

Результаты поиска передаются в Engine для формирования контекста:

```go
engine := rag.NewEngine(retriever)
context := engine.BuildContext(query, docs)
// context передаётся в LLM вместе с системным промптом
```

## Метрики (план)

Планируется сбор метрик:
- `rag_search_latency_ms` - время поиска
- `rag_retrieved_docs_count` - количество найденных документов
- `rag_embedding_compute_ms` - время вычисления эмбеддингов
- `rag_multi_hop_count` - количество multi-hop итераций
