package vector_store

// ChromaStore обеспечивает взаимодействие с Chroma векторной базой данных
type ChromaStore struct {
	URL string
}

// NewChromaStore создаёт новое подключение к Chroma
func NewChromaStore(url string) *ChromaStore {
	return &ChromaStore{URL: url}
}

// AddDocuments добавляет документы в векторное хранилище
func (c *ChromaStore) AddDocuments(docs []map[string]interface{}) error {
	// TODO: реализовать добавление документов в Chroma
	return nil
}

// Search выполняет семантический поиск
func (c *ChromaStore) Search(query string, n int) ([]map[string]interface{}, error) {
	// TODO: реализовать поиск
	return nil, nil
}
