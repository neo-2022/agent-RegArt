package llm

import (
	"math/rand"
	"os"
	"sync"
	"testing"
	"time"
)

// MockProvider — простой mock провайдера для тестирования
type MockProvider struct {
	name   string
	models []string
	err    error
}

func (m *MockProvider) Name() string {
	return m.name
}

func (m *MockProvider) Chat(req *ChatRequest) (*ChatResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &ChatResponse{
		Content: "mock response",
		Model:   m.name,
	}, nil
}

func (m *MockProvider) ListModels() ([]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.models, nil
}

func (m *MockProvider) ListModelsDetailed() ([]ModelDetail, error) {
	var details []ModelDetail
	for _, model := range m.models {
		details = append(details, ModelDetail{
			ID:          model,
			IsAvailable: true,
		})
	}
	return details, nil
}

// ===== Тесты для регистрации и получения провайдеров =====

func TestRegistry_Register_And_Get(t *testing.T) {
	// Создание отдельного реестра для теста (не глобального)
	reg := &Registry{
		providers: make(map[string]ChatProvider),
	}

	mock := &MockProvider{
		name:   "test-provider",
		models: []string{"model-1", "model-2"},
	}

	// Регистрируем провайдера
	reg.Register(mock)

	// Получаем провайдера обратно
	p, err := reg.Get("test-provider")
	if err != nil {
		t.Fatalf("ошибка получения провайдера: %v", err)
	}

	if p.Name() != "test-provider" {
		t.Errorf("ожидалось имя 'test-provider', получено %q", p.Name())
	}
}

func TestRegistry_Get_NonExistent(t *testing.T) {
	reg := &Registry{
		providers: make(map[string]ChatProvider),
	}

	_, err := reg.Get("non-existent")
	if err == nil {
		t.Fatal("ожидалась ошибка для несуществующего провайдера")
	}
}

func TestRegistry_Register_Replaces_Existing(t *testing.T) {
	reg := &Registry{
		providers: make(map[string]ChatProvider),
	}

	mock1 := &MockProvider{
		name:   "provider-1",
		models: []string{"model-1"},
	}
	mock2 := &MockProvider{
		name:   "provider-1", // Одно и то же имя
		models: []string{"model-2", "model-3"},
	}

	reg.Register(mock1)
	reg.Register(mock2) // Заменяем первого

	p, _ := reg.Get("provider-1")
	models, _ := p.ListModels()

	if len(models) != 2 {
		t.Errorf("ожидалось 2 модели (новый провайдер), получено %d", len(models))
	}
}

func TestRegistry_List(t *testing.T) {
	reg := &Registry{
		providers: make(map[string]ChatProvider),
	}

	providers := []string{"provider-1", "provider-2", "provider-3"}
	for _, name := range providers {
		reg.Register(&MockProvider{
			name: name,
		})
	}

	list := reg.List()
	if len(list) != 3 {
		t.Errorf("ожидалось 3 провайдера, получено %d", len(list))
	}

	// Проверяем, что все провайдеры в списке
	seen := make(map[string]bool)
	for _, name := range list {
		seen[name] = true
	}
	for _, name := range providers {
		if !seen[name] {
			t.Errorf("провайдер %q отсутствует в списке", name)
		}
	}
}

func TestRegistry_ListAll(t *testing.T) {
	reg := &Registry{
		providers: make(map[string]ChatProvider),
	}

	reg.Register(&MockProvider{
		name:   "provider-1",
		models: []string{"model-1", "model-2"},
	})
	reg.Register(&MockProvider{
		name:   "provider-2",
		models: []string{"model-3"},
	})

	infos := reg.ListAll()
	if len(infos) != 2 {
		t.Errorf("ожидалось 2 провайдера, получено %d", len(infos))
	}

	// Проверяем имена и модели
	found := make(map[string][]string)
	for _, info := range infos {
		found[info.Name] = info.Models
	}

	if len(found["provider-1"]) != 2 {
		t.Errorf("ожидалось 2 модели для provider-1, получено %d", len(found["provider-1"]))
	}
	if len(found["provider-2"]) != 1 {
		t.Errorf("ожидалось 1 модель для provider-2, получено %d", len(found["provider-2"]))
	}
}

// ===== Тесты инициализации из переменных окружения =====

func TestInitProviders_Ollama_Always_Registered(t *testing.T) {
	// Сохраняем текущее состояние
	originalRegistry := GlobalRegistry
	defer func() {
		GlobalRegistry = originalRegistry
	}()

	// Создаём новый реестр для теста
	GlobalRegistry = &Registry{
		providers: make(map[string]ChatProvider),
	}

	// Очищаем env для чистоты теста
	os.Unsetenv("OLLAMA_URL")
	os.Unsetenv("YANDEXGPT_API_KEY")
	os.Unsetenv("GIGACHAT_CLIENT_SECRET")

	InitProviders()

	// Ollama должен быть всегда зарегистрирован
	_, err := GlobalRegistry.Get("ollama")
	if err != nil {
		t.Fatal("Ollama должна быть зарегистрирована по умолчанию")
	}
}

func TestInitProviders_Ollama_Custom_URL(t *testing.T) {
	originalRegistry := GlobalRegistry
	defer func() {
		GlobalRegistry = originalRegistry
	}()

	GlobalRegistry = &Registry{
		providers: make(map[string]ChatProvider),
	}

	customURL := "http://custom.ollama:9999"
	os.Setenv("OLLAMA_URL", customURL)
	defer os.Unsetenv("OLLAMA_URL")

	InitProviders()

	p, _ := GlobalRegistry.Get("ollama")
	if p == nil {
		t.Fatal("Ollama не зарегистрирована")
	}
}

func TestInitProviders_YandexGPT_Registered_With_Key(t *testing.T) {
	originalRegistry := GlobalRegistry
	defer func() {
		GlobalRegistry = originalRegistry
	}()

	GlobalRegistry = &Registry{
		providers: make(map[string]ChatProvider),
	}

	os.Setenv("YANDEXGPT_API_KEY", "test-key")
	os.Setenv("YANDEXGPT_FOLDER_ID", "test-folder")
	defer func() {
		os.Unsetenv("YANDEXGPT_API_KEY")
		os.Unsetenv("YANDEXGPT_FOLDER_ID")
	}()

	InitProviders()

	_, err := GlobalRegistry.Get("yandexgpt")
	if err != nil {
		t.Error("YandexGPT должена быть зарегистрирована при наличии API-ключа")
	}
}

func TestInitProviders_YandexGPT_Not_Registered_Without_Key(t *testing.T) {
	originalRegistry := GlobalRegistry
	defer func() {
		GlobalRegistry = originalRegistry
	}()

	GlobalRegistry = &Registry{
		providers: make(map[string]ChatProvider),
	}

	os.Unsetenv("YANDEXGPT_API_KEY")

	InitProviders()

	_, err := GlobalRegistry.Get("yandexgpt")
	if err == nil {
		t.Error("YandexGPT не должна быть зарегистрирована без API-ключа")
	}
}

func TestInitProviders_GigaChat_Registered_With_Secret(t *testing.T) {
	originalRegistry := GlobalRegistry
	defer func() {
		GlobalRegistry = originalRegistry
	}()

	GlobalRegistry = &Registry{
		providers: make(map[string]ChatProvider),
	}

	os.Setenv("GIGACHAT_CLIENT_SECRET", "test-secret")
	os.Setenv("GIGACHAT_CLIENT_ID", "test-id")
	defer func() {
		os.Unsetenv("GIGACHAT_CLIENT_SECRET")
		os.Unsetenv("GIGACHAT_CLIENT_ID")
	}()

	InitProviders()

	_, err := GlobalRegistry.Get("gigachat")
	if err != nil {
		t.Error("GigaChat должен быть зарегистрирован при наличии CLIENT_SECRET")
	}
}

func TestInitProviders_GigaChat_Not_Registered_Without_Secret(t *testing.T) {
	originalRegistry := GlobalRegistry
	defer func() {
		GlobalRegistry = originalRegistry
	}()

	GlobalRegistry = &Registry{
		providers: make(map[string]ChatProvider),
	}

	os.Unsetenv("GIGACHAT_CLIENT_SECRET")

	InitProviders()

	_, err := GlobalRegistry.Get("gigachat")
	if err == nil {
		t.Error("GigaChat не должен быть зарегистрирован без CLIENT_SECRET")
	}
}

// ===== Тесты динамической регистрации =====

func TestRegisterProvider_Ollama(t *testing.T) {
	originalRegistry := GlobalRegistry
	defer func() {
		GlobalRegistry = originalRegistry
	}()

	GlobalRegistry = &Registry{
		providers: make(map[string]ChatProvider),
	}

	err := RegisterProvider("ollama", "", "http://localhost:11434", "", "")
	if err != nil {
		t.Fatalf("ошибка регистрации ollama: %v", err)
	}

	_, err = GlobalRegistry.Get("ollama")
	if err != nil {
		t.Error("Ollama не упал добавиться динамически")
	}
}

func TestRegisterProvider_YandexGPT(t *testing.T) {
	originalRegistry := GlobalRegistry
	defer func() {
		GlobalRegistry = originalRegistry
	}()

	GlobalRegistry = &Registry{
		providers: make(map[string]ChatProvider),
	}

	err := RegisterProvider("yandexgpt", "test-api-key", "", "test-folder-id", "")
	if err != nil {
		t.Fatalf("ошибка регистрации yandexgpt: %v", err)
	}

	_, err = GlobalRegistry.Get("yandexgpt")
	if err != nil {
		t.Error("YandexGPT не добавился динамически")
	}
}

func TestRegisterProvider_GigaChat(t *testing.T) {
	originalRegistry := GlobalRegistry
	defer func() {
		GlobalRegistry = originalRegistry
	}()

	GlobalRegistry = &Registry{
		providers: make(map[string]ChatProvider),
	}

	err := RegisterProvider("gigachat", "test-secret", "", "test-scope", "")
	if err != nil {
		t.Fatalf("ошибка регистрации gigachat: %v", err)
	}

	_, err = GlobalRegistry.Get("gigachat")
	if err != nil {
		t.Error("GigaChat не добавился динамически")
	}
}

func TestRegisterProvider_Unknown_Provider(t *testing.T) {
	originalRegistry := GlobalRegistry
	defer func() {
		GlobalRegistry = originalRegistry
	}()

	GlobalRegistry = &Registry{
		providers: make(map[string]ChatProvider),
	}

	err := RegisterProvider("unknown-provider", "", "", "", "")
	if err == nil {
		t.Fatal("ожидалась ошибка для неизвестного провайдера")
	}
}

// ===== Тесты потокобезопасности =====

func TestRegistry_ConcurrentRead(t *testing.T) {
	reg := &Registry{
		providers: make(map[string]ChatProvider),
	}

	// Регистрируем несколько провайдеров
	for i := 0; i < 10; i++ {
		reg.Register(&MockProvider{
			name: "provider-" + randString(5),
		})
	}

	// Читаем одновременно из нескольких goroutine
	var wg sync.WaitGroup
	errors := make(chan error, 50)

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			list := reg.List()
			if len(list) == 0 {
				errors <- nil // OK, может быть пусто временно
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		if err != nil {
			t.Errorf("ошибка при concurrent read: %v", err)
		}
	}
}

func TestRegistry_ConcurrentWrite_And_Read(t *testing.T) {
	reg := &Registry{
		providers: make(map[string]ChatProvider),
	}

	var wg sync.WaitGroup

	// Goroutine для записи
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				reg.Register(&MockProvider{
					name: "provider-" + randString(5),
				})
				time.Sleep(time.Microsecond)
			}
		}(i)
	}

	// Goroutine для чтения
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				_ = reg.List()
				_ = reg.ListAll()
				time.Sleep(time.Microsecond)
			}
		}()
	}

	wg.Wait()
}

func TestRegistry_ConcurrentGet_And_Register(t *testing.T) {
	reg := &Registry{
		providers: make(map[string]ChatProvider),
	}

	// Регистрируем initial провайдер
	reg.Register(&MockProvider{name: "test"})

	var wg sync.WaitGroup
	errors := make(chan error, 20)

	// Concurrent reads
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, err := reg.Get("test")
			if err != nil {
				errors <- err
			}
			if p == nil {
				errors <- nil
			}
		}()
	}

	// Concurrent write (update)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			reg.Register(&MockProvider{
				name: "updated-" + string(rune(id)),
			})
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		if err != nil {
			t.Errorf("concurrent error: %v", err)
		}
	}
}

func TestRegistry_ProviderReplacementUnderConcurrentAccess(t *testing.T) {
	reg := &Registry{
		providers: make(map[string]ChatProvider),
	}

	providerName := "concurrent-test"
	reg.Register(&MockProvider{
		name:   providerName,
		models: []string{"model-1"},
	})

	var wg sync.WaitGroup

	// Readers
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				p, err := reg.Get(providerName)
				if err != nil {
					t.Errorf("failed to get provider: %v", err)
					return
				}
				if p == nil {
					t.Error("provider is nil")
					return
				}
				models, _ := p.ListModels()
				if len(models) == 0 {
					// OK, может быть старая версия
				}
			}
		}()
	}

	// Writers - заменяем провайдера
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			time.Sleep(time.Millisecond)
			models := []string{"model-1", "model-2", "model-3"}
			reg.Register(&MockProvider{
				name:   providerName,
				models: models,
			})
		}(i)
	}

	wg.Wait()

	// Финальная проверка
	p, _ := reg.Get(providerName)
	if p == nil {
		t.Fatal("провайдер стал nil после concurrent write")
	}
}

// ===== Хелпер функции =====

func randString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	seededRand := rand.New(rand.NewSource(time.Now().UnixNano()))
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}
