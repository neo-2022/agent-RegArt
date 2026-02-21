package embeddings

import (
	"crypto/sha256"
	"encoding/binary"
	"math"
	"math/rand"
)

// EmbeddingModel интерфейс для вычисления эмбеддингов
type EmbeddingModel interface {
	Compute(text string) ([]float64, error)
	Dimension() int
}

// LocalEmbeddingModel локальная модель эмбеддингов на CPU
type LocalEmbeddingModel struct {
	dimension int
}

// NewLocalEmbeddingModel создаёт новую локальную модель
func NewLocalEmbeddingModel() *LocalEmbeddingModel {
	return &LocalEmbeddingModel{
		dimension: 384, // Стандартный размер для many models
	}
}

// Dimension возвращает размерность эмбеддинга
func (m *LocalEmbeddingModel) Dimension() int {
	return m.dimension
}

// Compute вычисляет эмбеддинг для текста
// Использует хеширование для детерминированности
func (m *LocalEmbeddingModel) Compute(text string) ([]float64, error) {
	// Используем SHA256 хеш текста для seed
	hash := sha256.Sum256([]byte(text))
	seed := binary.BigEndian.Uint64(hash[:8])

	rng := rand.New(rand.NewSource(int64(seed)))

	vec := make([]float64, m.dimension)

	// Генерируем псевдослучайный вектор с нормальным распределением
	// Используем Box-Muller transform
	for i := 0; i < m.dimension; i++ {
		u1 := rng.Float64()
		u2 := rng.Float64()

		// Избегаем log(0)
		if u1 < 1e-10 {
			u1 = 1e-10
		}

		// Box-Muller transform для нормального распределения
		z := math.Sqrt(-2.0*math.Log(u1)) * math.Cos(2.0*math.Pi*u2)
		vec[i] = z
	}

	// Нормализуем вектор
	norm := 0.0
	for _, v := range vec {
		norm += v * v
	}
	norm = math.Sqrt(norm)

	if norm > 0 {
		for i := range vec {
			vec[i] /= norm
		}
	}

	return vec, nil
}

// ComputeSimple вычисляет простой эмбеддинг (быстрее, но менее качественно)
func (m *LocalEmbeddingModel) ComputeSimple(text string) ([]float64, error) {
	hash := sha256.Sum256([]byte(text))
	seed := binary.BigEndian.Uint64(hash[:8])

	rng := rand.New(rand.NewSource(int64(seed)))

	vec := make([]float64, m.dimension)
	for i := range vec {
		vec[i] = rng.Float64()*2 - 1 // [-1, 1]
	}

	// Нормализуем
	norm := 0.0
	for _, v := range vec {
		norm += v * v
	}
	norm = math.Sqrt(norm)

	if norm > 0 {
		for i := range vec {
			vec[i] /= norm
		}
	}

	return vec, nil
}

// SentenceTransformerEmbedding эмуляция SentenceTransformer для совместимости
type SentenceTransformerEmbedding struct {
	*LocalEmbeddingModel
}

// NewSentenceTransformerEmbedding создаёт эмуляцию SentenceTransformer
func NewSentenceTransformerEmbedding(modelName string) *SentenceTransformerEmbedding {
	return &SentenceTransformerEmbedding{
		LocalEmbeddingModel: NewLocalEmbeddingModel(),
	}
}
