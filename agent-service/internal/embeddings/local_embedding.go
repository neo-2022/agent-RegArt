package embeddings

import "math/rand"

// LocalEmbeddingModel локальная модель эмбеддингов на CPU
type LocalEmbeddingModel struct{}

// NewLocalEmbeddingModel создаёт новую локальную модель
func NewLocalEmbeddingModel() *LocalEmbeddingModel {
	return &LocalEmbeddingModel{}
}

// Compute вычисляет эмбеддинг для текста (заглушка для тестирования)
func (m *LocalEmbeddingModel) Compute(text string) ([]float64, error) {
	vec := make([]float64, 128)
	for i := range vec {
		vec[i] = rand.Float64()
	}
	return vec, nil
}
