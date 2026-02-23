package skills

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DialogPattern — паттерн успешного диалога (intent → цепочка tool-вызовов).
type DialogPattern struct {
	Intent    string   `json:"intent"`
	ToolChain []string `json:"tool_chain"`
	HitCount  int      `json:"hit_count"`
	LastSeen  int64    `json:"last_seen"`
	AvgMs     float64  `json:"avg_ms"`
}

// CandidateSkill — кандидат на автогенерацию YAML-навыка.
type CandidateSkill struct {
	Pattern   DialogPattern `json:"pattern"`
	Status    string        `json:"status"`
	Version   int           `json:"version"`
	CreatedAt int64         `json:"created_at"`
	YAMLPath  string        `json:"yaml_path,omitempty"`
}

// AutoSkillPipeline — конвейер автоматической генерации навыков.
//
// Поток: dialog → learning → candidate (N повторов) → YAML + smoke → publish.
type AutoSkillPipeline struct {
	mu              sync.RWMutex
	patterns        map[string]*DialogPattern
	candidates      map[string]*CandidateSkill
	publishedSkills map[string][]CandidateSkill
	skillsDir       string
	threshold       int
	stateFile       string
}

// NewAutoSkillPipeline — создаёт конвейер автогенерации навыков.
// skillsDir — директория для публикации YAML, threshold — количество повторов для промоушена.
func NewAutoSkillPipeline(skillsDir string, threshold int) *AutoSkillPipeline {
	if threshold <= 0 {
		threshold = 3
	}
	p := &AutoSkillPipeline{
		patterns:        make(map[string]*DialogPattern),
		candidates:      make(map[string]*CandidateSkill),
		publishedSkills: make(map[string][]CandidateSkill),
		skillsDir:       skillsDir,
		threshold:       threshold,
		stateFile:       filepath.Join(skillsDir, ".autoskill_state.json"),
	}
	p.loadState()
	return p
}

func patternKey(intent string, chain []string) string {
	return intent + "|" + strings.Join(chain, "→")
}

// RecordSuccess — регистрация успешного диалога (intent + цепочка инструментов + время).
func (p *AutoSkillPipeline) RecordSuccess(intent string, toolChain []string, durationMs float64) {
	if intent == "" || len(toolChain) == 0 {
		return
	}
	key := patternKey(intent, toolChain)

	p.mu.Lock()
	defer p.mu.Unlock()

	pat, exists := p.patterns[key]
	if !exists {
		pat = &DialogPattern{
			Intent:    intent,
			ToolChain: toolChain,
		}
		p.patterns[key] = pat
	}
	pat.HitCount++
	pat.LastSeen = time.Now().Unix()
	pat.AvgMs = (pat.AvgMs*float64(pat.HitCount-1) + durationMs) / float64(pat.HitCount)

	if pat.HitCount >= p.threshold {
		if _, already := p.candidates[key]; !already {
			p.candidates[key] = &CandidateSkill{
				Pattern:   *pat,
				Status:    "candidate",
				Version:   1,
				CreatedAt: time.Now().Unix(),
			}
			slog.Info("[AUTO-SKILL] Новый кандидат на навык",
				slog.String("intent", intent),
				slog.Int("повторов", pat.HitCount))
		}
	}

	p.saveState()
}

// PromoteCandidates — генерация YAML + smoke-тест для всех кандидатов со статусом "candidate".
func (p *AutoSkillPipeline) PromoteCandidates() []string {
	p.mu.Lock()
	defer p.mu.Unlock()

	var promoted []string
	for key, cand := range p.candidates {
		if cand.Status != "candidate" {
			continue
		}
		yamlPath, err := p.generateYAML(cand)
		if err != nil {
			slog.Error("[AUTO-SKILL] Ошибка генерации YAML",
				slog.String("intent", cand.Pattern.Intent),
				slog.String("ошибка", err.Error()))
			continue
		}
		if err := p.smokeTest(yamlPath); err != nil {
			slog.Warn("[AUTO-SKILL] Smoke-тест не пройден, навык отложен",
				slog.String("intent", cand.Pattern.Intent),
				slog.String("ошибка", err.Error()))
			cand.Status = "smoke_failed"
			continue
		}
		cand.Status = "published"
		cand.YAMLPath = yamlPath
		p.publishedSkills[key] = append(p.publishedSkills[key], *cand)
		promoted = append(promoted, cand.Pattern.Intent)
		slog.Info("[AUTO-SKILL] Навык опубликован",
			slog.String("intent", cand.Pattern.Intent),
			slog.String("файл", yamlPath),
			slog.Int("версия", cand.Version))
	}
	p.saveState()
	return promoted
}

// Rollback — откат навыка к предыдущей версии или полное удаление.
func (p *AutoSkillPipeline) Rollback(intent string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	for key, cand := range p.candidates {
		if cand.Pattern.Intent != intent {
			continue
		}
		if cand.YAMLPath != "" {
			if err := os.Remove(cand.YAMLPath); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("ошибка удаления %s: %w", cand.YAMLPath, err)
			}
			slog.Info("[AUTO-SKILL] Навык удалён (rollback)",
				slog.String("intent", intent),
				slog.String("файл", cand.YAMLPath))
		}
		history := p.publishedSkills[key]
		if len(history) > 1 {
			prev := history[len(history)-2]
			p.candidates[key] = &prev
			p.publishedSkills[key] = history[:len(history)-1]
			slog.Info("[AUTO-SKILL] Откат к предыдущей версии",
				slog.String("intent", intent),
				slog.Int("версия", prev.Version))
		} else {
			delete(p.candidates, key)
		}
		p.saveState()
		return nil
	}
	return fmt.Errorf("навык для intent=%q не найден", intent)
}

// ListCandidates — список текущих кандидатов.
func (p *AutoSkillPipeline) ListCandidates() []CandidateSkill {
	p.mu.RLock()
	defer p.mu.RUnlock()
	result := make([]CandidateSkill, 0, len(p.candidates))
	for _, c := range p.candidates {
		result = append(result, *c)
	}
	return result
}

// ListPatterns — список всех наблюдаемых паттернов.
func (p *AutoSkillPipeline) ListPatterns() []DialogPattern {
	p.mu.RLock()
	defer p.mu.RUnlock()
	result := make([]DialogPattern, 0, len(p.patterns))
	for _, pat := range p.patterns {
		result = append(result, *pat)
	}
	return result
}

// generateYAML — генерация YAML-файла навыка из кандидата.
func (p *AutoSkillPipeline) generateYAML(cand *CandidateSkill) (string, error) {
	safeName := strings.ReplaceAll(strings.ToLower(cand.Pattern.Intent), " ", "_")
	safeName = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			return r
		}
		return '_'
	}, safeName)
	fileName := fmt.Sprintf("auto_%s_v%d.yaml", safeName, cand.Version)
	filePath := filepath.Join(p.skillsDir, fileName)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Автоматически сгенерированный навык (v%d)\n", cand.Version))
	sb.WriteString(fmt.Sprintf("# Intent: %s\n", cand.Pattern.Intent))
	sb.WriteString(fmt.Sprintf("# Повторений: %d, Средняя задержка: %.0f мс\n\n", cand.Pattern.HitCount, cand.Pattern.AvgMs))
	sb.WriteString(fmt.Sprintf("name: auto_%s\n", safeName))
	sb.WriteString(fmt.Sprintf("description: \"Автонавык: %s (цепочка: %s)\"\n", cand.Pattern.Intent, strings.Join(cand.Pattern.ToolChain, " → ")))
	sb.WriteString(fmt.Sprintf("version: \"%d.0\"\n", cand.Version))
	sb.WriteString("author: \"auto-skill-pipeline\"\n\n")
	sb.WriteString("parameters:\n")
	sb.WriteString("  - name: input\n")
	sb.WriteString("    type: string\n")
	sb.WriteString("    required: false\n")
	sb.WriteString("    description: \"Входные данные для навыка\"\n\n")

	if len(cand.Pattern.ToolChain) > 0 {
		endpoint := "http://localhost:8082/execute"
		sb.WriteString(fmt.Sprintf("endpoint: \"%s\"\n", endpoint))
	}
	sb.WriteString("method: POST\n\n")

	sb.WriteString("template: |\n")
	sb.WriteString("  {\n")
	sb.WriteString(fmt.Sprintf("    \"command\": \"%s\",\n", strings.Join(cand.Pattern.ToolChain, " && ")))
	sb.WriteString("    \"input\": \"{{input}}\"\n")
	sb.WriteString("  }\n\n")

	sb.WriteString("tags:\n")
	sb.WriteString("  - auto-generated\n")
	sb.WriteString(fmt.Sprintf("  - %s\n\n", safeName))

	sb.WriteString("agents:\n")
	sb.WriteString("  - admin\n")

	if err := os.WriteFile(filePath, []byte(sb.String()), 0644); err != nil {
		return "", fmt.Errorf("ошибка записи YAML %s: %w", filePath, err)
	}
	return filePath, nil
}

// smokeTest — проверка сгенерированного YAML: читаемость, обязательные поля.
func (p *AutoSkillPipeline) smokeTest(yamlPath string) error {
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return fmt.Errorf("не удалось прочитать файл: %w", err)
	}
	content := string(data)
	required := []string{"name:", "description:", "endpoint:", "method:", "agents:"}
	for _, field := range required {
		if !strings.Contains(content, field) {
			return fmt.Errorf("отсутствует обязательное поле: %s", field)
		}
	}
	if len(data) < 50 {
		return fmt.Errorf("файл слишком маленький (%d байт)", len(data))
	}
	slog.Info("[AUTO-SKILL] Smoke-тест пройден", slog.String("файл", yamlPath))
	return nil
}

// saveState — сохранение состояния конвейера на диск.
func (p *AutoSkillPipeline) saveState() {
	state := map[string]interface{}{
		"patterns":   p.patterns,
		"candidates": p.candidates,
		"published":  p.publishedSkills,
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		slog.Error("[AUTO-SKILL] Ошибка сериализации состояния", slog.String("ошибка", err.Error()))
		return
	}
	if err := os.MkdirAll(filepath.Dir(p.stateFile), 0755); err != nil {
		return
	}
	if err := os.WriteFile(p.stateFile, data, 0644); err != nil {
		slog.Error("[AUTO-SKILL] Ошибка сохранения состояния", slog.String("ошибка", err.Error()))
	}
}

// loadState — восстановление состояния конвейера с диска.
func (p *AutoSkillPipeline) loadState() {
	data, err := os.ReadFile(p.stateFile)
	if err != nil {
		return
	}
	var state struct {
		Patterns   map[string]*DialogPattern   `json:"patterns"`
		Candidates map[string]*CandidateSkill  `json:"candidates"`
		Published  map[string][]CandidateSkill `json:"published"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		slog.Warn("[AUTO-SKILL] Ошибка загрузки состояния", slog.String("ошибка", err.Error()))
		return
	}
	if state.Patterns != nil {
		p.patterns = state.Patterns
	}
	if state.Candidates != nil {
		p.candidates = state.Candidates
	}
	if state.Published != nil {
		p.publishedSkills = state.Published
	}
	slog.Info("[AUTO-SKILL] Состояние восстановлено",
		slog.Int("паттернов", len(p.patterns)),
		slog.Int("кандидатов", len(p.candidates)))
}
