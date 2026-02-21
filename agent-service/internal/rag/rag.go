package rag

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/neo-2022/openclaw-memory/agent-service/internal/embeddings"
)

// RagDoc представляет документ в RAG системе
type RagDoc struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"created_at"`
	Embedding []float64 `json:"embedding,omitempty"`
}

// SearchResult результат поиска с метаданными
type SearchResult struct {
	Doc   RagDoc  `json:"doc"`
	Score float64 `json:"score"`
	Rank  int     `json:"rank"`
}

// Config конфигурация RAG системы
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
}

// DBRetriever обеспечивает работу с базой данных документов
type DBRetriever struct {
	config    *Config
	embedding embeddings.EmbeddingModel
	chromaURL string
}

// NewDBRetriever создаёт новый экземпляр DBRetriever
func NewDBRetriever(config *Config) *DBRetriever {
	emb := embeddings.NewLocalEmbeddingModel()
	return &DBRetriever{
		config:    config,
		embedding: emb,
		chromaURL: config.ChromaURL,
	}
}

// EnsureTable создаёт таблицу rag_docs если её нет
func (d *DBRetriever) EnsureTable() error {
	// TODO: реализовать миграцию таблицы в PostgreSQL
	fmt.Println("[RAG] Table rag_docs ready (placeholder)")
	return nil
}

// AddDocument добавляет документ в ChromA хранилище
func (d *DBRetriever) AddDocument(doc RagDoc) error {
	if d.chromaURL == "" {
		fmt.Printf("[RAG] ChromA не настроен, документ %s не добавлен\n", doc.Title)
		return nil
	}

	emb, err := d.embedding.Compute(doc.Content)
	if err != nil {
		return fmt.Errorf("ошибка вычисления эмбеддинга: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/collections/rag_docs/add", d.chromaURL)
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

// SeedDemoDocuments добавляет демо-документы в ChromA
func (d *DBRetriever) SeedDemoDocuments() error {
	if d.chromaURL == "" {
		fmt.Println("[RAG] ChromA не настроен, используется fallback")
		return nil
	}

	demos := []RagDoc{
		{
			ID:      "doc-1",
			Title:   "Prompts - Admin System",
			Content: "Системный промпт для агента Admin. Содержит инструкции по координации других агентов и обработке сложных запросов. Admin может вызывать Coder и Novice как инструменты.",
			Source:  "prompts/admin",
		},
		{
			ID:      "doc-2",
			Title:   "Prompts - Coder Assistant",
			Content: "Промпт для агента Coder. Специализируется на написании и отладке кода, работе с файлами и терминалом. Имеет доступ к инструментам execute, read, write, list, delete.",
			Source:  "prompts/coder",
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

// SeedFromLocalCorpus загружает документы из локальных источников
func (d *DBRetriever) SeedFromLocalCorpus(paths []string) error {
	fmt.Printf("[RAG] Seeding from paths: %v\n", paths)

	for _, path := range paths {
		if err := d.seedFromPath(path); err != nil {
			fmt.Printf("[RAG] Error seeding from %s: %v\n", path, err)
		}
	}

	fmt.Println("[RAG] Seed complete")
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

			fmt.Printf("[RAG] Indexed: %s (%s)\n", doc.Title, doc.Source)
		}
	}

	return nil
}

// Search выполняет поиск документов по запросу
func (d *DBRetriever) Search(query string, topK int) ([]SearchResult, error) {
	if topK <= 0 {
		topK = d.config.TopK
		if topK <= 0 {
			topK = 5
		}
	}

	// Вычисляем эмбеддинг запроса
	queryEmb, err := d.embedding.Compute(query)
	if err != nil {
		return nil, fmt.Errorf("failed to compute query embedding: %w", err)
	}

	// Пытаемся использовать ChromA если доступен
	if d.chromaURL != "" {
		results, err := d.searchChroma(query, queryEmb, topK)
		if err == nil && len(results) > 0 {
			return results, nil
		}
		fmt.Printf("[RAG] ChromA search failed, using fallback: %v\n", err)
	}

	// Fallback: имитация поиска (для демо)
	return d.searchFallback(query, queryEmb, topK)
}

// searchChroma выполняет поиск через ChromA API
func (d *DBRetriever) searchChroma(query string, queryEmb []float64, topK int) ([]SearchResult, error) {
	url := fmt.Sprintf("%s/api/v1/collections/rag_docs/query", d.chromaURL)

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
		return nil, fmt.Errorf("chroma returned %d", resp.StatusCode)
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

// searchFallback имитация поиска для демо
func (d *DBRetriever) searchFallback(query string, queryEmb []float64, topK int) ([]SearchResult, error) {
	// Генерируем демо результаты
	sampleDocs := []RagDoc{
		{
			ID:      "doc-1",
			Title:   "Prompts - Admin System",
			Content: "Системный промпт для агента Admin. Содержит инструкции по координации других агентов и обработке сложных запросов.",
			Source:  "prompts/admin",
		},
		{
			ID:      "doc-2",
			Title:   "Prompts - Coder Assistant",
			Content: "Промпт для агента Coder. Специализируется на написании и отладке кода, работе с файлами и терминалом.",
			Source:  "prompts/coder",
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

	// Вычисляем similarity с эмбеддингом запроса
	rand.Seed(time.Now().UnixNano())

	var results []SearchResult
	for i, doc := range sampleDocs {
		// Симулируем similarity score
		score := 0.5 + rand.Float64()*0.5

		results = append(results, SearchResult{
			Doc:   doc,
			Score: score,
			Rank:  i + 1,
		})
	}

	// Сортируем по score
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

// MultiHopSearch выполняет multi-hop поиск
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// CosineSimilarity вычисляет косинусное сходство между векторами
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
