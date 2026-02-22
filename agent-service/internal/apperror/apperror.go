// Package apperror — типизированные ошибки приложения.
//
// Предоставляет структуру AppError для единообразной обработки ошибок
// во всех слоях приложения. Каждая ошибка содержит:
//   - Код ошибки (NOT_FOUND, BAD_REQUEST, INTERNAL и т.д.)
//   - Человекочитаемое сообщение
//   - Опциональную вложенную ошибку (для wrap-паттерна)
//
// Методы:
//   - HTTPStatus() — маппинг кода ошибки в HTTP-статус
//   - WriteJSON() — запись ошибки в HTTP-ответ в формате JSON
//   - Error() — реализация интерфейса error
//   - Unwrap() — поддержка errors.Is / errors.As
//
// Фабричные функции:
//   - New(code, message) — создать ошибку без вложенной
//   - Wrap(code, message, err) — обернуть существующую ошибку
//   - NotFound, BadRequest, Internal, Validation, Timeout — быстрые конструкторы
package apperror

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// AppError — структура типизированной ошибки приложения.
//
// Поля:
//   - Code: строковый код ошибки (NOT_FOUND, BAD_REQUEST, INTERNAL, VALIDATION, TIMEOUT и др.)
//   - Message: человекочитаемое описание ошибки
//   - Err: вложенная ошибка (nil, если ошибка создана без обёртки)
type AppError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Err     error  `json:"-"`
}

// Error — реализация интерфейса error.
// Форматирует ошибку как "[КОД] сообщение: вложенная_ошибка".
func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap — возвращает вложенную ошибку для поддержки errors.Is / errors.As.
func (e *AppError) Unwrap() error {
	return e.Err
}

// New — создаёт новую ошибку приложения без вложенной ошибки.
func New(code, message string) *AppError {
	return &AppError{Code: code, Message: message}
}

// Wrap — оборачивает существующую ошибку в AppError.
// Используется для добавления контекста к ошибкам из нижних слоёв.
func Wrap(code, message string, err error) *AppError {
	return &AppError{Code: code, Message: message, Err: err}
}

// HTTPStatus — возвращает HTTP-статус код, соответствующий коду ошибки.
//
// Маппинг:
//   - NOT_FOUND    → 404
//   - BAD_REQUEST  → 400
//   - VALIDATION   → 400
//   - UNAUTHORIZED → 401
//   - FORBIDDEN    → 403
//   - TIMEOUT      → 504
//   - CONFLICT     → 409
//   - RATE_LIMIT   → 429
//   - остальные    → 500
func (e *AppError) HTTPStatus() int {
	switch e.Code {
	case "NOT_FOUND":
		return http.StatusNotFound
	case "BAD_REQUEST", "VALIDATION":
		return http.StatusBadRequest
	case "UNAUTHORIZED":
		return http.StatusUnauthorized
	case "FORBIDDEN":
		return http.StatusForbidden
	case "TIMEOUT":
		return http.StatusGatewayTimeout
	case "CONFLICT":
		return http.StatusConflict
	case "RATE_LIMIT":
		return http.StatusTooManyRequests
	default:
		return http.StatusInternalServerError
	}
}

// WriteJSON — записывает ошибку в HTTP-ответ в формате JSON.
// Устанавливает Content-Type: application/json и соответствующий HTTP-статус.
func (e *AppError) WriteJSON(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(e.HTTPStatus())
	json.NewEncoder(w).Encode(map[string]string{
		"error":   e.Code,
		"message": e.Message,
	})
}

// NotFound — создаёт ошибку «не найдено» (HTTP 404).
func NotFound(message string) *AppError {
	return New("NOT_FOUND", message)
}

// BadRequest — создаёт ошибку «некорректный запрос» (HTTP 400).
func BadRequest(message string) *AppError {
	return New("BAD_REQUEST", message)
}

// Internal — создаёт внутреннюю ошибку сервера (HTTP 500) с обёрткой.
func Internal(message string, err error) *AppError {
	return Wrap("INTERNAL", message, err)
}

// Validation — создаёт ошибку валидации (HTTP 400).
func Validation(message string) *AppError {
	return New("VALIDATION", message)
}

// Timeout — создаёт ошибку таймаута (HTTP 504).
func Timeout(message string) *AppError {
	return New("TIMEOUT", message)
}
