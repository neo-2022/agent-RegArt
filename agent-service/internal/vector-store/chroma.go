// Package vector_store — клиент для работы с ChromaDB (векторная база данных).
//
// Предоставляет обёртку над ChromaDB API для хранения и поиска
// документов с использованием векторных эмбеддингов.
// Используется RAG-системой agent-service для семантического поиска.
package vector_store

// ChromaStore — клиент для взаимодействия с ChromaDB.
// Хранит URL подключения к экземпляру ChromaDB.
type ChromaStore struct {
	URL string // URL-адрес ChromaDB (например, http://localhost:8000)
}

// NewChromaStore — создаёт новый клиент ChromaDB с указанным URL подключения.
func NewChromaStore(url string) *ChromaStore {
	return &ChromaStore{URL: url}
}

// AddDocuments — добавляет документы в векторное хранилище ChromaDB.
// Каждый документ представлен как map с полями: id, content, embedding, metadata.
// ЗАДАЧА: реализовать HTTP-запрос к ChromaDB API для добавления документов.
func (c *ChromaStore) AddDocuments(docs []map[string]interface{}) error {
	// ЗАДАЧА: реализовать добавление документов в ChromaDB
	return nil
}

// Search — выполняет семантический поиск по векторному хранилищу.
// Возвращает n наиболее релевантных документов для заданного запроса.
// ЗАДАЧА: реализовать HTTP-запрос к ChromaDB API для поиска.
func (c *ChromaStore) Search(query string, n int) ([]map[string]interface{}, error) {
	// ЗАДАЧА: реализовать семантический поиск в ChromaDB
	return nil, nil
}
