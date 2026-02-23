package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRecordSuccess_BelowThreshold(t *testing.T) {
	dir := t.TempDir()
	p := NewAutoSkillPipeline(dir, 3)
	p.RecordSuccess("sysinfo", []string{"sysinfo"}, 100)
	p.RecordSuccess("sysinfo", []string{"sysinfo"}, 120)

	patterns := p.ListPatterns()
	if len(patterns) != 1 {
		t.Fatalf("ожидался 1 паттерн, получено %d", len(patterns))
	}
	if patterns[0].HitCount != 2 {
		t.Fatalf("ожидалось 2 попадания, получено %d", patterns[0].HitCount)
	}
	candidates := p.ListCandidates()
	if len(candidates) != 0 {
		t.Fatalf("не должно быть кандидатов при %d повторах (порог=3)", patterns[0].HitCount)
	}
}

func TestRecordSuccess_PromoteToCandidate(t *testing.T) {
	dir := t.TempDir()
	p := NewAutoSkillPipeline(dir, 3)
	for i := 0; i < 3; i++ {
		p.RecordSuccess("open_browser", []string{"findapp", "launchapp"}, 200)
	}

	candidates := p.ListCandidates()
	if len(candidates) != 1 {
		t.Fatalf("ожидался 1 кандидат, получено %d", len(candidates))
	}
	if candidates[0].Status != "candidate" {
		t.Fatalf("ожидался статус candidate, получен %s", candidates[0].Status)
	}
}

func TestPromoteCandidates_GeneratesYAML(t *testing.T) {
	dir := t.TempDir()
	p := NewAutoSkillPipeline(dir, 2)
	p.RecordSuccess("check_disk", []string{"execute"}, 150)
	p.RecordSuccess("check_disk", []string{"execute"}, 160)

	promoted := p.PromoteCandidates()
	if len(promoted) != 1 {
		t.Fatalf("ожидался 1 промоушен, получено %d", len(promoted))
	}

	files, _ := filepath.Glob(filepath.Join(dir, "auto_*.yaml"))
	if len(files) != 1 {
		t.Fatalf("ожидался 1 YAML-файл, найдено %d", len(files))
	}

	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, field := range []string{"name:", "description:", "endpoint:", "method:", "agents:"} {
		if !contains(content, field) {
			t.Errorf("отсутствует поле %s в YAML", field)
		}
	}
}

func TestRollback(t *testing.T) {
	dir := t.TempDir()
	p := NewAutoSkillPipeline(dir, 2)
	p.RecordSuccess("test_rollback", []string{"execute"}, 100)
	p.RecordSuccess("test_rollback", []string{"execute"}, 100)
	p.PromoteCandidates()

	files, _ := filepath.Glob(filepath.Join(dir, "auto_*.yaml"))
	if len(files) != 1 {
		t.Fatalf("ожидался 1 файл до rollback, найдено %d", len(files))
	}

	if err := p.Rollback("test_rollback"); err != nil {
		t.Fatal(err)
	}

	files, _ = filepath.Glob(filepath.Join(dir, "auto_*.yaml"))
	if len(files) != 0 {
		t.Fatalf("после rollback файлов быть не должно, найдено %d", len(files))
	}
}

func TestAvgLatency(t *testing.T) {
	dir := t.TempDir()
	p := NewAutoSkillPipeline(dir, 5)
	p.RecordSuccess("avg_test", []string{"a"}, 100)
	p.RecordSuccess("avg_test", []string{"a"}, 200)

	patterns := p.ListPatterns()
	if len(patterns) != 1 {
		t.Fatal("ожидался 1 паттерн")
	}
	if patterns[0].AvgMs < 149 || patterns[0].AvgMs > 151 {
		t.Fatalf("ожидалась средняя ~150, получено %.1f", patterns[0].AvgMs)
	}
}

func TestEmptyInputIgnored(t *testing.T) {
	dir := t.TempDir()
	p := NewAutoSkillPipeline(dir, 2)
	p.RecordSuccess("", []string{"a"}, 100)
	p.RecordSuccess("x", nil, 100)
	p.RecordSuccess("x", []string{}, 100)

	if len(p.ListPatterns()) != 0 {
		t.Fatal("пустые записи не должны сохраняться")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
