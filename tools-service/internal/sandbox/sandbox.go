// Package sandbox — изолированное выполнение команд в Docker-контейнерах.
//
// Обеспечивает безопасное выполнение пользовательских команд с ограничениями
// по памяти, CPU, количеству процессов и сети. Использует Docker-in-Docker.
package sandbox

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Config — конфигурация песочницы (sandbox).
// Определяет образ Docker, ограничения ресурсов и точки монтирования.
type Config struct {
	Enabled        bool          // Включена ли песочница (из переменной SANDBOX_ENABLED)
	Image          string        // Docker-образ для контейнера (по умолчанию ubuntu:22.04)
	MemoryLimit    string        // Лимит оперативной памяти (по умолчанию 256m)
	CPULimit       string        // Лимит CPU (по умолчанию 0.5 ядра)
	NetworkDisable bool          // Отключить сетевой доступ в контейнере
	Timeout        time.Duration // Таймаут выполнения команды (по умолчанию 30 секунд)
	MountReadOnly  []string      // Точки монтирования только для чтения
	MountReadWrite []string      // Точки монтирования для чтения и записи
}

// DefaultConfig — создаёт конфигурацию по умолчанию из переменных окружения.
// Если переменная не задана, используется значение по умолчанию.
func DefaultConfig() Config {
	return Config{
		Enabled:        os.Getenv("SANDBOX_ENABLED") == "true",
		Image:          getEnv("SANDBOX_IMAGE", "ubuntu:22.04"),
		MemoryLimit:    getEnv("SANDBOX_MEMORY_LIMIT", "256m"),
		CPULimit:       getEnv("SANDBOX_CPU_LIMIT", "0.5"),
		NetworkDisable: os.Getenv("SANDBOX_NETWORK_DISABLE") == "true",
		Timeout:        30 * time.Second,
	}
}

// Result — результат выполнения команды в песочнице.
type Result struct {
	Stdout     string `json:"stdout"`          // Стандартный вывод команды
	Stderr     string `json:"stderr"`          // Стандартный поток ошибок
	ReturnCode int    `json:"returncode"`      // Код возврата процесса
	Error      string `json:"error,omitempty"` // Ошибка выполнения (если есть)
	Sandboxed  bool   `json:"sandboxed"`       // Была ли команда выполнена в песочнице
}

// Execute — выполнить команду в изолированном Docker-контейнере.
//
// Создаёт контейнер с ограничениями ресурсов (память, CPU, PID-лимит),
// монтирует файловую систему только для чтения (кроме /tmp),
// применяет политику безопасности no-new-privileges,
// и опционально отключает сеть.
//
// Если песочница не включена (Enabled=false), возвращает ошибку без выполнения.
// Если команда превышает таймаут, процесс принудительно завершается.
func Execute(cfg Config, command string) Result {
	if !cfg.Enabled {
		return Result{Error: "песочница не включена", Sandboxed: false}
	}

	// Сборка аргументов docker run с ограничениями безопасности
	args := []string{
		"run", "--rm",
		"--memory", cfg.MemoryLimit,
		"--cpus", cfg.CPULimit,
		"--pids-limit", "100",
		"--read-only",
		"--tmpfs", "/tmp:rw,noexec,nosuid,size=64m",
		"--security-opt", "no-new-privileges",
	}

	// Отключение сети, если задано в конфигурации
	if cfg.NetworkDisable {
		args = append(args, "--network", "none")
	}

	// Добавление точек монтирования
	for _, m := range cfg.MountReadOnly {
		args = append(args, "-v", m+":ro")
	}
	for _, m := range cfg.MountReadWrite {
		args = append(args, "-v", m+":rw")
	}

	args = append(args, cfg.Image, "bash", "-c", command)

	log.Printf("[ПЕСОЧНИЦА] выполняем: docker %s", strings.Join(args, " "))

	cmd := exec.Command("docker", args...)

	// Таймер принудительного завершения при превышении таймаута
	timer := time.AfterFunc(cfg.Timeout, func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
	})
	defer timer.Stop()

	stdout, err := cmd.Output()
	stderr := ""
	exitCode := 0

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr = string(exitErr.Stderr)
			exitCode = exitErr.ExitCode()
		} else {
			return Result{
				Error:     fmt.Sprintf("ошибка выполнения в песочнице: %v", err),
				Sandboxed: true,
			}
		}
	}

	return Result{
		Stdout:     string(stdout),
		Stderr:     stderr,
		ReturnCode: exitCode,
		Sandboxed:  true,
	}
}

// IsAvailable — проверяет, доступен ли Docker для выполнения команд.
// Возвращает true, если docker info выполняется успешно.
func IsAvailable() bool {
	cmd := exec.Command("docker", "info")
	return cmd.Run() == nil
}

// getEnv — возвращает значение переменной окружения или fallback, если не задана.
func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
