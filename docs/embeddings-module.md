# Модуль эмбеддингов

## Обзор

Модуль_embeddings_ обеспечивает вычисление векторных представлений (эмбеддингов) для текстовых документов и запросов. Эмбеддинги используются для семантического поиска в RAG системе.

## Компоненты

### EmbeddingModel (интерфейс)

Базовый интерфейс для всех моделей эмбеддингов:

```go
type EmbeddingModel interface {
    Compute(text string) ([]float64, error)
    Dimension() int
}
```

Методы:
- `Compute(text string)` - вычисляет эмбеддинг для текста
- `Dimension()` - возвращает размерность вектора

### LocalEmbeddingModel

Локальная модель эмбеддингов, работающая на CPU без внешних зависимостей.

#### Особенности:

1. **Детерминированность** - одинаковый текст всегда даёт одинаковый эмбеддинг (используется SHA256 хеш как seed)

2. **Нормализация** - все вектора нормализуются к единичной длине

3. **Размерность** - 384 измерения (стандарт для many models)

4. **Box-Muller transform** - генерация нормального распределения для качественных эмбеддингов

## Алгоритмы

### Генерация эмбеддинга

1. **Хеширование** - SHA256 от текста -> seed для генератора случайных чисел
2. **Генерация вектора** - Box-Muller transform для нормального распределения
3. **Нормализация** - деление на L2 норму для получения единичного вектора

### Косинусное сходство

Для сравнения эмбеддингов используется косинусное сходство:

```
cosine(A, B) = (A · B) / (||A|| * ||B||)
```

Значения от -1 (противоположные) до 1 (идентичные), где 1 означает полное совпадение.

## Использование

### Создание модели

```go
model := embeddings.NewLocalEmbeddingModel()
```

### Вычисление эмбеддинга

```go
embedding, err := model.Compute("текст для векторизации")
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Размерность: %d\n", model.Dimension())
fmt.Printf("Первые 5 значений: %v\n", embedding[:5])
```

### Получение размера

```go
dim := model.Dimension() // 384
```

## Планы развития

### SentenceTransformer

Планируется добавить поддержку SentenceTransformer для более качественных эмбеддингов:

```go
type SentenceTransformerEmbedding struct {
    *LocalEmbeddingModel
}

func NewSentenceTransformerEmbedding(modelName string) *SentenceTransformerEmbedding
```

Модели:
- `sentence-transformers/all-MiniLM-L6-v2` - быстрая, 384-dim
- `sentence-transformers/all-mpnet-base-v2` - качественная, 768-dim

### OpenAI Embeddings

Интеграция с OpenAI API:

```go
type OpenAIEmbedding struct {
    APIKey string
    Model  string // text-embedding-3-small или text-embedding-3-large
}
```

### Кэширование

Планируется кэширование вычисленных эмбеддингов:
- In-memory кэш для часто используемых текстов
- Персистентный кэш в Redis/PostgreSQL

## Метрики

- `embedding_compute_ms` - время вычисления одного эмбеддинга
- `embedding_cache_hit_ratio` - доля взятых из кэша
- `embedding_dimension` - размерность используемой модели
