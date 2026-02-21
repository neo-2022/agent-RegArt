package executor

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// SystemInfo возвращает базовую информацию о системе.
type SystemInfo struct {
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	Hostname string `json:"hostname"`
	HomeDir  string `json:"home_dir"`
	User     string `json:"user"`
}

func GetSystemInfo() SystemInfo {
	hostname, _ := os.Hostname()
	homeDir, _ := os.UserHomeDir()
	user := os.Getenv("USER")
	if user == "" {
		user = os.Getenv("LOGNAME")
	}
	return SystemInfo{
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		Hostname: hostname,
		HomeDir:  homeDir,
		User:     user,
	}
}

// GetCPUInfo возвращает информацию о процессоре (упрощённо).
func GetCPUInfo() (string, error) {
	return ReadFile("/proc/cpuinfo")
}

// GetMemInfo возвращает информацию о памяти.
func GetMemInfo() (string, error) {
	return ReadFile("/proc/meminfo")
}

// GetCPUTemperature возвращает температуру процессора (через sensors).
func GetCPUTemperature() (string, error) {
	cmd := exec.Command("sensors", "-j")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// GetSystemLoad возвращает информацию о загрузке (load average, память, диски).
func GetSystemLoad() (map[string]interface{}, error) {
	// load average
	uptime := exec.Command("uptime")
	uptimeOut, _ := uptime.Output()
	load := strings.TrimSpace(string(uptimeOut))

	// память
	free := exec.Command("free", "-h")
	freeOut, _ := free.Output()
	mem := strings.TrimSpace(string(freeOut))

	// диски
	df := exec.Command("df", "-h")
	dfOut, _ := df.Output()
	disk := strings.TrimSpace(string(dfOut))

	return map[string]interface{}{
		"load":   load,
		"memory": mem,
		"disk":   disk,
	}, nil
}
