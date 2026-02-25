package executor

import (
	"os"
	"path/filepath"
	"testing"
)

// ===== Тесты валидации пути =====

func TestValidatePath_PathTraversal(t *testing.T) {
	_, err := validatePath("../../etc/shadow")
	if err == nil {
		t.Fatal("ожидалась ошибка для path traversal")
	}
}

func TestValidatePath_PathTraversal_Simple(t *testing.T) {
	_, err := validatePath("../etc/passwd")
	if err == nil {
		t.Fatal("ожидалась ошибка для simple path traversal")
	}
}

func TestValidatePath_PathTraversal_Embedded(t *testing.T) {
	_, err := validatePath("/home/user/../../../etc/passwd")
	if err == nil {
		t.Fatal("ожидалась ошибка для embedded path traversal")
	}
}

func TestValidatePath_ForbiddenPath(t *testing.T) {
	_, err := validatePath("/etc/shadow")
	if err == nil {
		t.Fatal("ожидалась ошибка для /etc/shadow")
	}
}

func TestValidatePath_ForbiddenPath_Etc_Passwd(t *testing.T) {
	_, err := validatePath("/etc/passwd")
	if err == nil {
		t.Fatal("ожидалась ошибка для /etc/passwd")
	}
}

func TestValidatePath_ForbiddenPath_Sudoers(t *testing.T) {
	_, err := validatePath("/etc/sudoers")
	if err == nil {
		t.Fatal("ожидалась ошибка для /etc/sudoers")
	}
}

func TestValidatePath_ForbiddenPath_Proc(t *testing.T) {
	_, err := validatePath("/proc/sys/kernel/panic")
	if err == nil {
		t.Fatal("ожидалась ошибка для /proc/sys/kernel/panic")
	}
}

func TestValidatePath_ForbiddenPath_Sys(t *testing.T) {
	_, err := validatePath("/sys/kernel/debug")
	if err == nil {
		t.Fatal("ожидалась ошибка для /sys/...")
	}
}

func TestValidatePath_ForbiddenPath_Dev(t *testing.T) {
	_, err := validatePath("/dev/sda")
	if err == nil {
		t.Fatal("ожидалась ошибка для /dev/sda")
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

func TestValidatePath_AllowedSystemFile_Meminfo(t *testing.T) {
	path, err := validatePath("/proc/meminfo")
	if err != nil {
		t.Fatalf("ожидался успех для /proc/meminfo, получена ошибка: %v", err)
	}
	if path != "/proc/meminfo" {
		t.Errorf("ожидался путь /proc/meminfo, получен %s", path)
	}
}

func TestValidatePath_Regular_File(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.txt")

	path, err := validatePath(testFile)
	if err != nil {
		t.Fatalf("ожидался успех для обычного файла: %v", err)
	}
	if path != testFile {
		t.Errorf("ожидалась корректная нормализация пути")
	}
}

// ===== Тесты резолва домашней директории =====

func TestValidatePath_HomePath_Tilde(t *testing.T) {
	path, err := validatePath("~/test.txt")
	if err != nil {
		t.Fatalf("ошибка валидации ~/test.txt: %v", err)
	}

	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, "test.txt")

	if path != expected {
		t.Errorf("ожидалось %s, получено %s", expected, path)
	}
}

func TestValidatePath_HomePath_TildeSlash(t *testing.T) {
	path, err := validatePath("~/subdir/file.txt")
	if err != nil {
		t.Fatalf("ошибка валидации ~/subdir/file.txt: %v", err)
	}

	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, "subdir/file.txt")

	if path != expected {
		t.Errorf("ожидалось %s, получено %s", expected, path)
	}
}

// ===== Тесты чтения файла =====

func TestReadFile_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	content := "тестовое содержимое"
	os.WriteFile(path, []byte(content), 0644)

	result, err := ReadFile(path)
	if err != nil {
		t.Fatalf("ошибка чтения файла: %v", err)
	}
	if result != content {
		t.Errorf("ожидалось %q, получено %q", content, result)
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

func TestReadFile_At_MaxSize_Boundary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "max.txt")
	data := make([]byte, MaxFileSize)
	os.WriteFile(path, data, 0644)

	_, err := ReadFile(path)
	if err != nil {
		t.Fatalf("файл размером в MaxFileSize должен читаться успешно: %v", err)
	}
}

func TestReadFile_PathTraversal_Blocked(t *testing.T) {
	_, err := ReadFile("../../etc/passwd")
	if err == nil {
		t.Fatal("ожидалась ошибка для path traversal при read")
	}
}

func TestReadFile_ForbiddenPath_Blocked(t *testing.T) {
	_, err := ReadFile("/etc/shadow")
	if err == nil {
		t.Fatal("ожидалась ошибка для /etc/shadow при read")
	}
}

func TestReadFile_NonExistent(t *testing.T) {
	_, err := ReadFile("/nonexistent/file.txt")
	if err == nil {
		t.Fatal("ожидалась ошибка для несуществующего файла")
	}
}

// ===== Тесты записи файла =====

func TestWriteFile_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	content := "тестовое содержимое"

	err := WriteFile(path, content)
	if err != nil {
		t.Fatalf("ошибка записи файла: %v", err)
	}

	// Проверяем содержимое
	result, _ := ReadFile(path)
	if result != content {
		t.Errorf("ожидалось %q, получено %q", content, result)
	}
}

func TestWriteFile_Creates_Parent_Directories(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir1", "subdir2", "test.txt")

	err := WriteFile(path, "content")
	if err != nil {
		t.Fatalf("ошибка при создании родительских директорий: %v", err)
	}

	// Проверяем что файл создан
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("файл не был создан")
	}
}

func TestWriteFile_Content_Too_Large(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.txt")
	largecontent := make([]byte, MaxFileSize+1)

	err := WriteFile(path, string(largecontent))
	if err == nil {
		t.Fatal("ожидалась ошибка для содержимого превышающего MaxFileSize")
	}
}

func TestWriteFile_PathTraversal_Blocked(t *testing.T) {
	err := WriteFile("../../etc/passwd", "malicious")
	if err == nil {
		t.Fatal("ожидалась ошибка для path traversal при write")
	}
}

func TestWriteFile_ForbiddenPath_Blocked(t *testing.T) {
	err := WriteFile("/etc/shadow", "malicious")
	if err == nil {
		t.Fatal("ожидалась ошибка для /etc/shadow при write")
	}
}

// ===== Тесты листинга директории =====

func TestListDirectory(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0644)
	os.Mkdir(filepath.Join(dir, "subdir"), 0755)

	names, err := ListDirectory(dir)
	if err != nil {
		t.Fatalf("ошибка ListDirectory: %v", err)
	}
	if len(names) != 3 {
		t.Errorf("ожидалось 3 элемента, получено %d", len(names))
	}
}

func TestListDirectory_Empty(t *testing.T) {
	dir := t.TempDir()

	names, err := ListDirectory(dir)
	if err != nil {
		t.Fatalf("ошибка ListDirectory для пустой директории: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("ожидалось 0 элементов, получено %d", len(names))
	}
}

func TestListDirectory_PathTraversal_Blocked(t *testing.T) {
	_, err := ListDirectory("../../etc")
	if err == nil {
		t.Fatal("ожидалась ошибка для path traversal при list")
	}
}

func TestListDirectory_ForbiddenPath_Blocked(t *testing.T) {
	_, err := ListDirectory("/etc")
	if err == nil {
		t.Fatal("ожидалась ошибка для /etc при list")
	}
}

// ===== Тесты удаления файла =====

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

func TestDeleteFile_NonExistent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.txt")

	err := DeleteFile(path)
	if err == nil {
		t.Fatal("ожидалась ошибка при удалении несуществующего файла")
	}
}

func TestDeleteFile_PathTraversal_Blocked(t *testing.T) {
	err := DeleteFile("../../etc/passwd")
	if err == nil {
		t.Fatal("ожидалась ошибка для path traversal при delete")
	}
}

func TestDeleteFile_ForbiddenPath_Blocked(t *testing.T) {
	err := DeleteFile("/etc/shadow")
	if err == nil {
		t.Fatal("ожидалась ошибка для /etc/shadow при delete")
	}
}

// ===== Тесты граничных случаев =====

func TestValidatePath_Empty_String(t *testing.T) {
	path, _ := validatePath("")
	home, _ := os.UserHomeDir()
	if path != home {
		t.Errorf("пустая строка должна резолваться в домашну директорию")
	}
}

func TestValidatePath_Whitespace_Only(t *testing.T) {
	path, _ := validatePath("   ")
	home, _ := os.UserHomeDir()
	if path != home {
		t.Error("пробелы должны быть обработаны как пустая строка")
	}
}

func TestValidatePath_Tilde_Only(t *testing.T) {
	path, _ := validatePath("~")
	home, _ := os.UserHomeDir()
	if path != home {
		t.Errorf("~ должен резолваться в домашню директорию")
	}
}

func TestValidatePath_Tilde_With_Slash(t *testing.T) {
	path, _ := validatePath("~/")
	home, _ := os.UserHomeDir()
	if path != home {
		t.Errorf("~/ должный резолваться в домашню директорию")
	}
}

// ===== Интеграционные тесты =====

func TestWriteAndReadFile_Integration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "integration.txt")
	originalContent := "Это интеграционный тест с русским текстом"

	// Пишем
	err := WriteFile(path, originalContent)
	if err != nil {
		t.Fatalf("ошибка при write: %v", err)
	}

	// Читаем
	readContent, err := ReadFile(path)
	if err != nil {
		t.Fatalf("ошибка при read: %v", err)
	}

	if readContent != originalContent {
		t.Errorf("содержимое не совпадает: %q != %q", originalContent, readContent)
	}

	// Удаляем
	err = DeleteFile(path)
	if err != nil {
		t.Fatalf("ошибка при delete: %v", err)
	}

	// Проверяем что удалено
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("файл должен быть удаён")
	}
}

func TestMultipleOperations_DifferentFiles(t *testing.T) {
	dir := t.TempDir()

	// Создаём несколько файлов
	files := map[string]string{
		"file1.txt": "content 1",
		"file2.txt": "content 2",
		"file3.txt": "content 3",
	}

	for name, content := range files {
		path := filepath.Join(dir, name)
		err := WriteFile(path, content)
		if err != nil {
			t.Fatalf("ошибка при write %s: %v", name, err)
		}
	}

	// Листим директорию
	list, err := ListDirectory(dir)
	if err != nil {
		t.Fatalf("ошибка при list: %v", err)
	}

	if len(list) != len(files) {
		t.Errorf("ожидалось %d файлов, получено %d", len(files), len(list))
	}

	// Проверяем содержимое каждого файла
	for name, originalContent := range files {
		path := filepath.Join(dir, name)
		readContent, err := ReadFile(path)
		if err != nil {
			t.Errorf("ошибка чтения %s: %v", name, err)
			continue
		}
		if readContent != originalContent {
			t.Errorf("содержимое %s не совпадает", name)
		}
	}
}
