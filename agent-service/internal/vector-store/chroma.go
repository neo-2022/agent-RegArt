// Package vector_store — клиент для работы с ChromaDB (векторная база данных).
//
// Предоставляет обёртку над ChromaDB API для хранения и поиска
// документов с использованием векторных эмбеддингов.
// Используется RAG-системой agent-service для семантического поиска.
package vector_store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

// ChromaStore — клиент для взаимодействия с ChromaDB.
type ChromaStore struct {
	URL        string
	Collection string
	APIVersion string
}

// NewChromaStore — создаёт новый клиент ChromaDB с указанным URL подключения.
func NewChromaStore(url string) *ChromaStore {
	ver := os.Getenv("CHROMA_API_VERSION")
	if ver == "" {
		ver = "v2"
	}
	return &ChromaStore{URL: url, Collection: "rag_docs", APIVersion: ver}
}

// AddDocuments — добавляет документы в векторное хранилище ChromaDB.
// Каждый документ представлен как map с полями: id, content, embedding, metadata.
func (c *ChromaStore) AddDocuments(docs []map[string]interface{}) error {
	if c.URL == "" {
		return fmt.Errorf("ChromaDB URL не задан")
	}
	if len(docs) == 0 {
		return nil
	}

	ids := make([]string, 0, len(docs))
	documents := make([]string, 0, len(docs))
	embeddings := make([][]float64, 0, len(docs))
	metadatas := make([]map[string]interface{}, 0, len(docs))

	for _, doc := range docs {
		id, _ := doc["id"].(string)
		content, _ := doc["content"].(string)
		emb, _ := doc["embedding"].([]float64)
		meta, _ := doc["metadata"].(map[string]interface{})
		if id == "" || content == "" {
			continue
		}
		ids = append(ids, id)
		documents = append(documents, content)
		if len(emb) > 0 {
			embeddings = append(embeddings, emb)
		}
		if meta == nil {
			meta = map[string]interface{}{}
		}
		metadatas = append(metadatas, meta)
	}

	if len(ids) == 0 {
		return nil
	}

	payload := map[string]interface{}{
		"ids":       ids,
		"documents": documents,
		"metadatas": metadatas,
	}
	if len(embeddings) == len(ids) {
		payload["embeddings"] = embeddings
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("ошибка сериализации: %w", err)
	}

	url := fmt.Sprintf("%s/api/%s/collections/%s/add", c.URL, c.APIVersion, c.Collection)
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("ошибка создания запроса: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("ошибка HTTP-запроса к ChromaDB: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ChromaDB вернул %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// Search — выполняет семантический поиск по векторному хранилищу.
// Возвращает n наиболее релевантных документов для заданного запроса.
func (c *ChromaStore) Search(query string, n int) ([]map[string]interface{}, error) {
	if c.URL == "" {
		return nil, fmt.Errorf("ChromaDB URL не задан")
	}
	if n <= 0 {
		n = 5
	}

	payload := map[string]interface{}{
		"query_texts": []string{query},
		"n_results":   n,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("ошибка сериализации: %w", err)
	}

	url := fmt.Sprintf("%s/api/%s/collections/%s/query", c.URL, c.APIVersion, c.Collection)
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ошибка создания запроса: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ошибка HTTP-запроса к ChromaDB: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ChromaDB вернул %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("ошибка декодирования ответа: %w", err)
	}

	rawIDs, _ := result["ids"].([]interface{})
	rawDocs, _ := result["documents"].([]interface{})
	rawDists, _ := result["distances"].([]interface{})
	rawMetas, _ := result["metadatas"].([]interface{})

	var results []map[string]interface{}

	idsArr := unwrapFirstArray(rawIDs)
	docsArr := unwrapFirstArray(rawDocs)
	distsArr := unwrapFirstArray(rawDists)
	metasArr := unwrapFirstArray(rawMetas)

	for i := range idsArr {
		item := map[string]interface{}{
			"id": idsArr[i],
		}
		if i < len(docsArr) {
			item["content"] = docsArr[i]
		}
		if i < len(distsArr) {
			if d, ok := distsArr[i].(float64); ok {
				item["score"] = 1.0 - d
			}
		}
		if i < len(metasArr) {
			item["metadata"] = metasArr[i]
		}
		results = append(results, item)
	}

	return results, nil
}

func unwrapFirstArray(raw []interface{}) []interface{} {
	if len(raw) > 0 {
		if inner, ok := raw[0].([]interface{}); ok {
			return inner
		}
	}
	return raw
}
