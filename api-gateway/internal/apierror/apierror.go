package apierror

import (
	"encoding/json"
	"net/http"
)

type Response struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Hint      string `json:"hint,omitempty"`
	RequestID string `json:"request_id,omitempty"`
	Retryable bool   `json:"retryable"`
}

func Write(w http.ResponseWriter, status int, resp Response) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}

func BadGateway(w http.ResponseWriter, requestID, message, hint string) {
	Write(w, http.StatusBadGateway, Response{
		Code:      "BAD_GATEWAY",
		Message:   message,
		Hint:      hint,
		RequestID: requestID,
		Retryable: true,
	})
}

func ServiceUnavailable(w http.ResponseWriter, requestID, message, hint string) {
	Write(w, http.StatusServiceUnavailable, Response{
		Code:      "SERVICE_UNAVAILABLE",
		Message:   message,
		Hint:      hint,
		RequestID: requestID,
		Retryable: true,
	})
}

func TooManyRequests(w http.ResponseWriter, requestID, message, hint string) {
	Write(w, http.StatusTooManyRequests, Response{
		Code:      "RATE_LIMITED",
		Message:   message,
		Hint:      hint,
		RequestID: requestID,
		Retryable: true,
	})
}

func InternalError(w http.ResponseWriter, requestID, message string) {
	Write(w, http.StatusInternalServerError, Response{
		Code:      "INTERNAL_ERROR",
		Message:   message,
		RequestID: requestID,
		Retryable: true,
	})
}

func NotFound(w http.ResponseWriter, requestID, message string) {
	Write(w, http.StatusNotFound, Response{
		Code:      "NOT_FOUND",
		Message:   message,
		RequestID: requestID,
		Retryable: false,
	})
}

func MethodNotAllowed(w http.ResponseWriter, requestID string) {
	Write(w, http.StatusMethodNotAllowed, Response{
		Code:      "METHOD_NOT_ALLOWED",
		Message:   "Метод не поддерживается",
		RequestID: requestID,
		Retryable: false,
	})
}

func BadRequest(w http.ResponseWriter, requestID, message string) {
	Write(w, http.StatusBadRequest, Response{
		Code:      "BAD_REQUEST",
		Message:   message,
		RequestID: requestID,
		Retryable: false,
	})
}

func PayloadTooLarge(w http.ResponseWriter, requestID, message string) {
	Write(w, http.StatusRequestEntityTooLarge, Response{
		Code:      "PAYLOAD_TOO_LARGE",
		Message:   message,
		RequestID: requestID,
		Retryable: false,
	})
}
