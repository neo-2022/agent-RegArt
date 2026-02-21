package rag

import "time"

// RagDoc представляет документ в RAG системе
type RagDoc struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"created_at"`
	Embedding []float64 `json:"embedding,omitempty"`
}

// DBRetriever обеспечивает работу с базой данных документов
type DBRetriever struct {
	// TODO: добавить подключение к БД
}

// NewDBRetriever создаёт новый экземпляр DBRetriever
func NewDBRetriever() *DBRetriever {
	return &DBRetriever{}
}

// EnsureTable создаёт таблицу rag_docs если её нет
func (d *DBRetriever) EnsureTable() error {
	// TODO: реализовать миграцию таблицы
	return nil
}

// SeedFromLocalCorpus загружает документы из локальных источников
func (d *DBRetriever) SeedFromLocalCorpus(paths []string) error {
	// TODO: реализовать загрузку из prompts/, uploads/
	return nil
}

// Search выполняет поиск документов по запросу
func (d *DBRetriever) Search(query string, topK int) ([]RagDoc, error) {
	// TODO: реализовать гибридный поиск (lexical + semantic)
	return nil, nil
}

// Engine формирует контекст для LLM из найденных документов
type Engine struct {
	Retriever *DBRetriever
}

// NewEngine создаёт новый экземпляр Engine
func NewEngine(r *DBRetriever) *Engine {
	return &Engine{Retriever: r}
}

// BuildContext формирует контекст для промпта из найденных документов
func (e *Engine) BuildContext(query string, docs []RagDoc) string {
	ctx := "Context:\n"
	for i, d := range docs {
		ctx += "[" + string(rune('A'+i)) + "] " + d.Title + "\n" + d.Content + "\n"
	}
	ctx += "\nQuestion: " + query
	return ctx
}
