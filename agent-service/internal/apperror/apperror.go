package apperror

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type AppError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Err     error  `json:"-"`
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

func (e *AppError) Unwrap() error {
	return e.Err
}

func New(code, message string) *AppError {
	return &AppError{Code: code, Message: message}
}

func Wrap(code, message string, err error) *AppError {
	return &AppError{Code: code, Message: message, Err: err}
}

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

func (e *AppError) WriteJSON(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(e.HTTPStatus())
	json.NewEncoder(w).Encode(map[string]string{
		"error":   e.Code,
		"message": e.Message,
	})
}

func NotFound(message string) *AppError {
	return New("NOT_FOUND", message)
}

func BadRequest(message string) *AppError {
	return New("BAD_REQUEST", message)
}

func Internal(message string, err error) *AppError {
	return Wrap("INTERNAL", message, err)
}

func Validation(message string) *AppError {
	return New("VALIDATION", message)
}

func Timeout(message string) *AppError {
	return New("TIMEOUT", message)
}
