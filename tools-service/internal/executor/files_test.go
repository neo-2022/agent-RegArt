package executor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidatePath_PathTraversal(t *testing.T) {
	_, err := validatePath("../../etc/shadow")
	if err == nil {
		t.Fatal("ожидалась ошибка для path traversal")
	}
}

func TestValidatePath_ForbiddenPath(t *testing.T) {
	_, err := validatePath("/etc/shadow")
	if err == nil {
		t.Fatal("ожидалась ошибка для /etc/shadow")
	}
}

func TestValidatePath_AllowedSystemFile(t *testing.T) {
	path, err := validatePath("/proc/cpuinfo")
	if err != nil {
		t.Fatalf("ожидался успех для /proc/cpuinfo, получена ошибка: %v", err)
	}
	if path != "/proc/cpuinfo" {
		t.Errorf("ожидался путь /proc/cpuinfo, получен %s", path)
	}
}

func TestReadFile_MaxSize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.txt")
	data := make([]byte, MaxFileSize+1)
	os.WriteFile(path, data, 0644)

	_, err := ReadFile(path)
	if err == nil {
		t.Fatal("ожидалась ошибка для файла превышающего MaxFileSize")
	}
}

func TestWriteFile_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	err := WriteFile(path, "тестовое содержимое")
	if err != nil {
		t.Fatalf("ожидался успех, получена ошибка: %v", err)
	}

	content, err := ReadFile(path)
	if err != nil {
		t.Fatalf("ошибка чтения файла: %v", err)
	}
	if content != "тестовое содержимое" {
		t.Errorf("ожидалось 'тестовое содержимое', получено %q", content)
	}
}

func TestListDirectory(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0644)

	names, err := ListDirectory(dir)
	if err != nil {
		t.Fatalf("ошибка ListDirectory: %v", err)
	}
	if len(names) != 2 {
		t.Errorf("ожидалось 2 файла, получено %d", len(names))
	}
}

func TestDeleteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "del.txt")
	os.WriteFile(path, []byte("delete me"), 0644)

	err := DeleteFile(path)
	if err != nil {
		t.Fatalf("ошибка DeleteFile: %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("файл должен быть удалён")
	}
}
