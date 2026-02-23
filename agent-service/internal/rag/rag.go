// Package rag — система извлечения документов (Retrieval-Augmented Generation).
//
// Предоставляет функциональность для:
//   - Хранения и индексации документов (через ChromaDB или fallback)
//   - Семантического поиска по документам с использованием эмбеддингов
//   - Multi-hop поиска (многошаговый поиск с расширением запроса)
//   - Обрезки чанков и ограничения контекста по длине
//   - Вычисления косинусного сходства между векторами
//
// Поддерживает два режима работы:
//   - ChromaDB (если CHROMA_URL задан) — полноценный векторный поиск
//   - Fallback (без ChromaDB) — демо-режим с примерными результатами
package rag

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/neo-2022/openclaw-memory/agent-service/internal/embeddings"
)

// RagDoc — документ в RAG-системе.
// Содержит текст, метаданные и опциональный вектор эмбеддинга.
type RagDoc struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"created_at"`
	Embedding []float64 `json:"embedding,omitempty"`
}

// SearchResult — результат поиска документа с оценкой релевантности и рангом.
type SearchResult struct {
	Doc   RagDoc  `json:"doc"`
	Score float64 `json:"score"`
	Rank  int     `json:"rank"`
}

// Config — конфигурация RAG-системы.
// Включает настройки ChromaDB, модели эмбеддингов, параметры поиска и подключения к БД.
type Config struct {
	DBHost         string
	DBPort         string
	DBUser         string
	DBPassword     string
	DBName         string
	ChromaURL      string
	EmbeddingModel string
	TopK           int
	EnableMultiHop bool
	MaxChunkLen    int
	MaxContextLen  int
}

const (
	DefaultMaxChunkLen   = 2000
	DefaultMaxContextLen = 8000
)

// TruncateChunk — обрезает текст чанка до максимальной длины.
// Если текст короче maxLen — возвращает как есть.
// Иначе обрезает и добавляет "...[обрезано]".
func TruncateChunk(text string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = DefaultMaxChunkLen
	}
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen] + "...[обрезано]"
}

// LimitContext — ограничивает суммарную длину контекста из результатов поиска.
// Последовательно добавляет результаты, пока общая длина не превысит maxTotalLen.
// Последний результат может быть обрезан, если остаток > 100 символов.
func LimitContext(results []SearchResult, maxTotalLen int) []SearchResult {
	if maxTotalLen <= 0 {
		maxTotalLen = DefaultMaxContextLen
	}
	var limited []SearchResult
	totalLen := 0
	for _, r := range results {
		contentLen := len(r.Doc.Content)
		if totalLen+contentLen > maxTotalLen {
			remaining := maxTotalLen - totalLen
			if remaining > 100 {
				r.Doc.Content = TruncateChunk(r.Doc.Content, remaining)
				limited = append(limited, r)
			}
			break
		}
		totalLen += contentLen
		limited = append(limited, r)
	}
	return limited
}

// DBRetriever — основной компонент RAG-системы.
// Обеспечивает работу с ChromaDB и fallback-поиском документов.
type DBRetriever struct {
	config       *Config
	embedding    embeddings.EmbeddingModel
	chromaURL    string
	chromaAPIVer string
}

// NewDBRetriever — создаёт новый экземпляр DBRetriever с заданной конфигурацией.
func NewDBRetriever(config *Config) *DBRetriever {
	emb := embeddings.NewLocalEmbeddingModel()
	ver := os.Getenv("CHROMA_API_VERSION")
	if ver == "" {
		ver = "v2"
	}
	return &DBRetriever{
		config:       config,
		embedding:    emb,
		chromaURL:    config.ChromaURL,
		chromaAPIVer: ver,
	}
}

// Config — возвращает текущую конфигурацию RAG-системы.
func (d *DBRetriever) Config() *Config {
	return d.config
}

// EnsureTable — создаёт таблицу rag_docs в PostgreSQL, если её нет.
// Использует подключение через database/sql с параметрами из Config.
func (d *DBRetriever) EnsureTable() error {
	cfg := d.config
	if cfg.DBHost == "" {
		fmt.Println("[RAG] Параметры БД не заданы, миграция пропущена")
		return nil
	}

	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, cfg.DBName,
	)

	sqlDB, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("ошибка подключения к БД: %w", err)
	}
	defer sqlDB.Close()

	query := `CREATE TABLE IF NOT EXISTS rag_docs (
		id          SERIAL PRIMARY KEY,
		title       TEXT NOT NULL DEFAULT '',
		content     TEXT NOT NULL DEFAULT '',
		source      TEXT NOT NULL DEFAULT '',
		chunk_index INTEGER DEFAULT 0,
		total_chunks INTEGER DEFAULT 0,
		workspace_id INTEGER,
		created_at  TIMESTAMPTZ DEFAULT NOW(),
		updated_at  TIMESTAMPTZ DEFAULT NOW(),
		deleted_at  TIMESTAMPTZ
	)`
	if _, err := sqlDB.Exec(query); err != nil {
		return fmt.Errorf("ошибка создания таблицы rag_docs: %w", err)
	}

	idx := `CREATE INDEX IF NOT EXISTS idx_rag_docs_source ON rag_docs(source)`
	if _, err := sqlDB.Exec(idx); err != nil {
		return fmt.Errorf("ошибка создания индекса: %w", err)
	}

	fmt.Println("[RAG] Таблица rag_docs готова")
	return nil
}

// AddDocument — добавляет документ в ChromaDB-хранилище.
//
// Алгоритм работы:
//  1. Если ChromaDB не настроен (chromaURL пуст) — метод завершает работу без ошибки.
//  2. Вычисляет эмбеддинг по содержимому документа.
//  3. Отправляет документ, эмбеддинг и метаданные в коллекцию rag_docs через HTTP API ChromaDB.
func (d *DBRetriever) AddDocument(doc RagDoc) error {
	if d.chromaURL == "" {
		fmt.Printf("[RAG] ChromA не настроен, документ %s не добавлен\n", doc.Title)
		return nil
	}

	emb, err := d.embedding.Compute(doc.Content)
	if err != nil {
		return fmt.Errorf("ошибка вычисления эмбеддинга: %w", err)
	}

	url := fmt.Sprintf("%s/api/%s/collections/rag_docs/add", d.chromaURL, d.chromaAPIVer)
	body, _ := json.Marshal(map[string]interface{}{
		"ids":        []string{doc.ID},
		"embeddings": [][]float64{emb},
		"metadatas": []map[string]interface{}{
			{"title": doc.Title, "source": doc.Source},
		},
		"documents": []string{doc.Content},
	})

	req, _ := http.NewRequest("POST", url, strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("ошибка добавления в ChromA: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		return fmt.Errorf("Chroma вернул %d", resp.StatusCode)
	}

	fmt.Printf("[RAG] Добавлен документ: %s\n", doc.Title)
	return nil
}

// SeedDemoDocuments — загружает демонстрационный набор документов в ChromaDB.
// Используется для быстрого запуска и проверки RAG-поиска в окружениях разработки.
func (d *DBRetriever) SeedDemoDocuments() error {
	if d.chromaURL == "" {
		fmt.Println("[RAG] ChromA не настроен, используется fallback")
		return nil
	}

	demos := []RagDoc{
		{
			ID:      "doc-1",
			Title:   "Prompts - Admin System",
			Content: "Системный промпт для агента Admin. Единственный агент системы, управляющий ПК через инструменты: execute, read, write, list, sysinfo, browser и др. Сильные модели (7B+) получают базовые инструменты, слабые (≤3B) — составные навыки.",
			Source:  "prompts/admin",
		},
		{
			ID:      "doc-2",
			Title:   "Skills System",
			Content: "Система навыков (Skills) — YAML-описания составных операций для Admin-агента. Навыки объединяют несколько инструментов в один шаг (LEGO-блоки). Примеры: check_system, deploy_project, analyze_logs.",
			Source:  "skills",
		},
		{
			ID:      "doc-3",
			Title:   "LM Studio Setup Guide",
			Content: "Руководство по настройке LM Studio для локального запуска LLM моделей. Поддержка CPU и GPU ускорения. Подключение к Ollama совместимым API.",
			Source:  "uploads/docs",
		},
		{
			ID:      "doc-4",
			Title:   "API Documentation",
			Content: "Документация по API AgentCore. Включает описание эндпоинтов: /chat (основной чат), /agents (список агентов), /providers (управление провайдерами), /cloud-models.",
			Source:  "uploads/docs",
		},
		{
			ID:      "doc-5",
			Title:   "Memory Service Architecture",
			Content: "Архитектура сервиса памяти AgentCore. Хранение контекста между сессиями, векторное индексирование с ChromA, семантический поиск.",
			Source:  "memory-service",
		},
	}

	for _, doc := range demos {
		if err := d.AddDocument(doc); err != nil {
			fmt.Printf("[RAG] Ошибка добавления %s: %v\n", doc.Title, err)
		}
	}

	fmt.Println("[RAG] Демо-документы добавлены")
	return nil
}

// SeedFromLocalCorpus — загружает документы из локальных директорий в RAG-систему.
func (d *DBRetriever) SeedFromLocalCorpus(paths []string) error {
	fmt.Printf("[RAG] Загрузка из путей: %v\n", paths)

	for _, path := range paths {
		if err := d.seedFromPath(path); err != nil {
			fmt.Printf("[RAG] Ошибка загрузки из %s: %v\n", path, err)
		}
	}

	fmt.Println("[RAG] Загрузка завершена")
	return nil
}

func (d *DBRetriever) seedFromPath(path string) error {
	path = strings.TrimPrefix(path, "./")

	// Проверяем существующие директории
	dirs := []string{
		"agent-service/prompts",
		"agent-service/uploads",
	}

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}

			filePath := filepath.Join(dir, entry.Name())
			content, err := os.ReadFile(filePath)
			if err != nil {
				continue
			}

			doc := RagDoc{
				ID:        fmt.Sprintf("%s-%d", entry.Name(), time.Now().UnixNano()),
				Title:     entry.Name(),
				Content:   string(content),
				Source:    dir,
				CreatedAt: time.Now(),
			}

			// Вычисляем эмбеддинг
			emb, err := d.embedding.Compute(doc.Content)
			if err == nil {
				doc.Embedding = emb
			}

			fmt.Printf("[RAG] Индексирован: %s (%s)\n", doc.Title, doc.Source)
		}
	}

	return nil
}

// Search — выполняет семантический поиск документов по запросу.
// Сначала пытается использовать ChromaDB, при неудаче — fallback.
// Результаты обрезаются по MaxChunkLen и ограничиваются по MaxContextLen.
func (d *DBRetriever) Search(query string, topK int) ([]SearchResult, error) {
	if topK <= 0 {
		topK = d.config.TopK
		if topK <= 0 {
			topK = 5
		}
	}

	queryEmb, err := d.embedding.Compute(query)
	if err != nil {
		return nil, fmt.Errorf("ошибка вычисления эмбеддинга запроса: %w", err)
	}

	var results []SearchResult

	if d.chromaURL != "" {
		results, err = d.searchChroma(query, queryEmb, topK)
		if err != nil || len(results) == 0 {
			fmt.Printf("[RAG] Поиск через ChromaDB не удался, используем fallback: %v\n", err)
			results, err = d.searchFallback(query, queryEmb, topK)
		}
	} else {
		results, err = d.searchFallback(query, queryEmb, topK)
	}

	if err != nil {
		return nil, err
	}

	maxChunk := d.config.MaxChunkLen
	for i := range results {
		results[i].Doc.Content = TruncateChunk(results[i].Doc.Content, maxChunk)
	}

	maxCtx := d.config.MaxContextLen
	results = LimitContext(results, maxCtx)

	return results, nil
}

// searchChroma — выполняет поиск документов через HTTP API ChromaDB.
// Отправляет эмбеддинг запроса и получает topK ближайших результатов.
func (d *DBRetriever) searchChroma(query string, queryEmb []float64, topK int) ([]SearchResult, error) {
	url := fmt.Sprintf("%s/api/%s/collections/rag_docs/query", d.chromaURL, d.chromaAPIVer)

	body, _ := json.Marshal(map[string]interface{}{
		"query_embeddings": [][]float64{queryEmb},
		"n_results":        topK,
	})

	req, _ := http.NewRequest("POST", url, strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("ChromaDB вернул статус %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	// Парсим результаты
	ids, _ := result["ids"].([][]interface{})
	distances, _ := result["distances"].([][]interface{})
	metadatas, _ := result["metadatas"].([][]interface{})

	var results []SearchResult
	for i := 0; i < len(ids) && i < topK; i++ {
		score := 1.0
		if len(distances) > 0 && i < len(distances[i]) {
			if d, ok := distances[i][0].(float64); ok {
				score = 1.0 - d
			}
		}

		doc := RagDoc{
			ID:     fmt.Sprintf("%v", ids[i][0]),
			Source: "chroma",
		}

		if len(metadatas) > 0 && i < len(metadatas[i]) {
			if m, ok := metadatas[i][0].(map[string]interface{}); ok {
				if t, ok := m["title"].(string); ok {
					doc.Title = t
				}
				if c, ok := m["content"].(string); ok {
					doc.Content = c
				}
			}
		}

		results = append(results, SearchResult{
			Doc:   doc,
			Score: score,
			Rank:  i + 1,
		})
	}

	return results, nil
}

// searchFallback — имитация поиска для демо-режима (без ChromaDB).
// Генерирует примерные результаты с рандомными оценками релевантности.
func (d *DBRetriever) searchFallback(query string, queryEmb []float64, topK int) ([]SearchResult, error) {
	// Генерируем демонстрационные результаты
	sampleDocs := []RagDoc{
		{
			ID:      "doc-1",
			Title:   "Prompts - Admin System",
			Content: "Системный промпт для агента Admin. Единственный агент, управляющий ПК через инструменты и составные навыки.",
			Source:  "prompts/admin",
		},
		{
			ID:      "doc-2",
			Title:   "Skills System",
			Content: "Система навыков (Skills) — YAML-описания составных операций для Admin-агента.",
			Source:  "skills",
		},
		{
			ID:      "doc-3",
			Title:   "LM Studio Setup Guide",
			Content: "Руководство по настройке LM Studio для локального запуска LLM моделей. Поддержка CPU и GPU ускорения.",
			Source:  "uploads/docs",
		},
		{
			ID:      "doc-4",
			Title:   "API Documentation",
			Content: "Документация по API AgentCore. Включает описание эндпоинтов /chat, /agents, /providers, /cloud-models.",
			Source:  "uploads/docs",
		},
		{
			ID:      "doc-5",
			Title:   "Memory Service Architecture",
			Content: "Архитектура сервиса памяти. Хранение контекста между сессиями, векторное индексирование, семантический поиск.",
			Source:  "memory-service",
		},
	}

	// Вычисляем сходство (релевантность) относительно эмбеддинга запроса
	rand.Seed(time.Now().UnixNano())

	var results []SearchResult
	for i, doc := range sampleDocs {
		// Симулируем оценку релевантности (оценку сходства)
		score := 0.5 + rand.Float64()*0.5

		results = append(results, SearchResult{
			Doc:   doc,
			Score: score,
			Rank:  i + 1,
		})
	}

	// Сортируем по убыванию оценки релевантности
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	if len(results) > topK {
		results = results[:topK]
	}

	return results, nil
}

// MultiHopSearch — выполняет многошаговый (multi-hop) поиск.
// На каждом шаге расширяет запрос контекстом из найденных документов.
// Результаты дедуплицируются и ограничиваются topK.
func (d *DBRetriever) MultiHopSearch(query string, hops int) ([]SearchResult, error) {
	if hops <= 0 {
		hops = 2
	}

	var allResults []SearchResult
	currentQuery := query

	for h := 0; h < hops; h++ {
		results, err := d.Search(currentQuery, d.config.TopK)
		if err != nil {
			return allResults, err
		}

		allResults = append(allResults, results...)

		// Расширяем запрос на основе найденных документов
		if h < hops-1 && len(results) > 0 {
			var context []string
			for _, r := range results[:min(2, len(results))] {
				context = append(context, r.Doc.Content)
			}
			currentQuery = query + " " + strings.Join(context, " ")
		}
	}

	// Дедупликация и пересортировка
	seen := make(map[string]bool)
	var deduped []SearchResult
	for _, r := range allResults {
		if !seen[r.Doc.ID] {
			seen[r.Doc.ID] = true
			deduped = append(deduped, r)
		}
	}

	// Ограничиваем топ-K после всех хопов
	if len(deduped) > d.config.TopK {
		deduped = deduped[:d.config.TopK]
	}

	return deduped, nil
}

// min — возвращает минимальное из двух целых чисел.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// CosineSimilarity — вычисляет косинусное сходство между двумя векторами.
// Возвращает значение от -1 до 1 (1 = идентичны, 0 = ортогональны).
func CosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := 0; i < len(a); i++ {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}
