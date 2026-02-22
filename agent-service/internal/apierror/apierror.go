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

func BadRequest(w http.ResponseWriter, requestID, message, hint string) {
	Write(w, http.StatusBadRequest, Response{
		Code:      "BAD_REQUEST",
		Message:   message,
		Hint:      hint,
		RequestID: requestID,
		Retryable: false,
	})
}

func InternalError(w http.ResponseWriter, requestID, message, hint string) {
	Write(w, http.StatusInternalServerError, Response{
		Code:      "INTERNAL_ERROR",
		Message:   message,
		Hint:      hint,
		RequestID: requestID,
		Retryable: true,
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

func NotFound(w http.ResponseWriter, requestID, message string) {
	Write(w, http.StatusNotFound, Response{
		Code:      "NOT_FOUND",
		Message:   message,
		RequestID: requestID,
		Retryable: false,
	})
}

func LLMError(w http.ResponseWriter, requestID, message, hint string) {
	Write(w, http.StatusBadGateway, Response{
		Code:      "LLM_ERROR",
		Message:   message,
		Hint:      hint,
		RequestID: requestID,
		Retryable: true,
	})
}

func ToolError(w http.ResponseWriter, requestID, message, hint string) {
	Write(w, http.StatusBadGateway, Response{
		Code:      "TOOL_ERROR",
		Message:   message,
		Hint:      hint,
		RequestID: requestID,
		Retryable: true,
	})
}
