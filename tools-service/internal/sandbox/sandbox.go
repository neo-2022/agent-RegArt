package sandbox

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

type Config struct {
	Enabled        bool
	Image          string
	MemoryLimit    string
	CPULimit       string
	NetworkDisable bool
	Timeout        time.Duration
	MountReadOnly  []string
	MountReadWrite []string
}

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

type Result struct {
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	ReturnCode int    `json:"returncode"`
	Error      string `json:"error,omitempty"`
	Sandboxed  bool   `json:"sandboxed"`
}

func Execute(cfg Config, command string) Result {
	if !cfg.Enabled {
		return Result{Error: "sandbox is not enabled", Sandboxed: false}
	}

	args := []string{
		"run", "--rm",
		"--memory", cfg.MemoryLimit,
		"--cpus", cfg.CPULimit,
		"--pids-limit", "100",
		"--read-only",
		"--tmpfs", "/tmp:rw,noexec,nosuid,size=64m",
		"--security-opt", "no-new-privileges",
	}

	if cfg.NetworkDisable {
		args = append(args, "--network", "none")
	}

	for _, m := range cfg.MountReadOnly {
		args = append(args, "-v", m+":ro")
	}
	for _, m := range cfg.MountReadWrite {
		args = append(args, "-v", m+":rw")
	}

	args = append(args, cfg.Image, "bash", "-c", command)

	log.Printf("[SANDBOX] executing: docker %s", strings.Join(args, " "))

	cmd := exec.Command("docker", args...)

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
				Error:     fmt.Sprintf("sandbox execution failed: %v", err),
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

func IsAvailable() bool {
	cmd := exec.Command("docker", "info")
	return cmd.Run() == nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
