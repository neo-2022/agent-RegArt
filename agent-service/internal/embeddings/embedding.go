package embeddings

// EmbeddingModel интерфейс для вычисления эмбеддингов
type EmbeddingModel interface {
	Compute(text string) ([]float64, error)
}
