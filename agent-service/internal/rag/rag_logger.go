package rag

import (
	"log"
	"time"
)

// Logger - структура для логирования операций
type Logger struct {
	Info  *log.Logger
	Error *log.Logger
	Debug *log.Logger
}

// NewLogger создает новый экземпляр логгера
func NewLogger() *Logger {
	return &Logger{
		Info:  log.New(log.Writer(), "INFO: ", log.Ldate|log.Ltime|log.Lshortfile),
		Error: log.New(log.Writer(), "ERROR: ", log.Ldate|log.Ltime|log.Lshortfile),
		Debug: log.New(log.Writer(), "DEBUG: ", log.Ldate|log.Ltime|log.Lshortfile),
	}
}

// LogInfo записывает информационное сообщение
func (l *Logger) LogInfo(message string, args ...interface{}) {
	l.Info.Printf(message, args...)
}

// LogError записывает сообщение об ошибке
func (l *Logger) LogError(message string, args ...interface{}) {
	l.Error.Printf(message, args...)
}

// LogDebug записывает отладочное сообщение
func (l *Logger) LogDebug(message string, args ...interface{}) {
	l.Debug.Printf(message, args...)
}

// LogOperation логирует выполнение операции
func (l *Logger) LogOperation(operation string, success bool, duration time.Duration) {
	status := "SUCCESS"
	if !success {
		status = "FAILED"
	}
	l.Info.Printf("OPERATION: %s | STATUS: %s | DURATION: %v", operation, status, duration)
}
