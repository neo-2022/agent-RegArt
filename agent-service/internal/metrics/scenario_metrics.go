package metrics

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"sync"
	"time"
)

// ScenarioRecord — запись одного выполнения сценария.
type ScenarioRecord struct {
	Scenario      string  `json:"scenario"`
	LatencyMs     float64 `json:"latency_ms"`
	ToolCallCount int     `json:"tool_call_count"`
	Success       bool    `json:"success"`
	ErrorMsg      string  `json:"error_msg,omitempty"`
	Timestamp     int64   `json:"timestamp"`
}

// ScenarioStats — агрегированная статистика по сценарию.
type ScenarioStats struct {
	Scenario     string  `json:"scenario"`
	TotalRuns    int     `json:"total_runs"`
	SuccessCount int     `json:"success_count"`
	ErrorCount   int     `json:"error_count"`
	SuccessRate  float64 `json:"success_rate"`
	ErrorRate    float64 `json:"error_rate"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
	P50LatencyMs float64 `json:"p50_latency_ms"`
	P95LatencyMs float64 `json:"p95_latency_ms"`
	P99LatencyMs float64 `json:"p99_latency_ms"`
	MaxLatencyMs float64 `json:"max_latency_ms"`
	AvgToolCalls float64 `json:"avg_tool_calls"`
	LastRunAt    int64   `json:"last_run_at"`
}

// ScenarioCollector — сборщик метрик качества по сценариям.
type ScenarioCollector struct {
	mu      sync.RWMutex
	records map[string][]ScenarioRecord
	maxKeep int
}

var (
	globalCollector     *ScenarioCollector
	globalCollectorOnce sync.Once
)

// GetScenarioCollector — глобальный сборщик метрик.
func GetScenarioCollector() *ScenarioCollector {
	globalCollectorOnce.Do(func() {
		globalCollector = NewScenarioCollector(10000)
	})
	return globalCollector
}

// NewScenarioCollector — создаёт сборщик с лимитом записей на сценарий.
func NewScenarioCollector(maxKeep int) *ScenarioCollector {
	if maxKeep <= 0 {
		maxKeep = 10000
	}
	return &ScenarioCollector{
		records: make(map[string][]ScenarioRecord),
		maxKeep: maxKeep,
	}
}

// Record — регистрация результата выполнения сценария.
func (c *ScenarioCollector) Record(scenario string, latencyMs float64, toolCallCount int, success bool, errMsg string) {
	rec := ScenarioRecord{
		Scenario:      scenario,
		LatencyMs:     latencyMs,
		ToolCallCount: toolCallCount,
		Success:       success,
		ErrorMsg:      errMsg,
		Timestamp:     time.Now().Unix(),
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	recs := c.records[scenario]
	recs = append(recs, rec)
	if len(recs) > c.maxKeep {
		recs = recs[len(recs)-c.maxKeep:]
	}
	c.records[scenario] = recs
}

// Stats — агрегированная статистика по конкретному сценарию.
func (c *ScenarioCollector) Stats(scenario string) *ScenarioStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	recs, ok := c.records[scenario]
	if !ok || len(recs) == 0 {
		return nil
	}
	return computeStats(scenario, recs)
}

// AllStats — статистика по всем сценариям.
func (c *ScenarioCollector) AllStats() []ScenarioStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]ScenarioStats, 0, len(c.records))
	for scenario, recs := range c.records {
		if len(recs) > 0 {
			result = append(result, *computeStats(scenario, recs))
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Scenario < result[j].Scenario
	})
	return result
}

// Reset — сброс метрик для указанного сценария (или всех, если пустая строка).
func (c *ScenarioCollector) Reset(scenario string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if scenario == "" {
		c.records = make(map[string][]ScenarioRecord)
		slog.Info("[METRICS] Все метрики сценариев сброшены")
	} else {
		delete(c.records, scenario)
		slog.Info("[METRICS] Метрики сброшены", slog.String("сценарий", scenario))
	}
}

func computeStats(scenario string, recs []ScenarioRecord) *ScenarioStats {
	st := &ScenarioStats{Scenario: scenario, TotalRuns: len(recs)}

	latencies := make([]float64, 0, len(recs))
	var totalLatency, totalTools float64

	for _, r := range recs {
		if r.Success {
			st.SuccessCount++
		} else {
			st.ErrorCount++
		}
		latencies = append(latencies, r.LatencyMs)
		totalLatency += r.LatencyMs
		totalTools += float64(r.ToolCallCount)
		if r.Timestamp > st.LastRunAt {
			st.LastRunAt = r.Timestamp
		}
		if r.LatencyMs > st.MaxLatencyMs {
			st.MaxLatencyMs = r.LatencyMs
		}
	}

	n := float64(st.TotalRuns)
	st.SuccessRate = float64(st.SuccessCount) / n
	st.ErrorRate = float64(st.ErrorCount) / n
	st.AvgLatencyMs = totalLatency / n
	st.AvgToolCalls = totalTools / n

	sort.Float64s(latencies)
	st.P50LatencyMs = percentile(latencies, 0.50)
	st.P95LatencyMs = percentile(latencies, 0.95)
	st.P99LatencyMs = percentile(latencies, 0.99)

	return st
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := p * float64(len(sorted)-1)
	lower := int(idx)
	upper := lower + 1
	if upper >= len(sorted) {
		return sorted[len(sorted)-1]
	}
	frac := idx - float64(lower)
	return sorted[lower]*(1-frac) + sorted[upper]*frac
}

// ScenarioMetricsHandler — HTTP-обработчик для /scenario-metrics.
// GET  — возвращает статистику по всем сценариям (или ?scenario=X для одного).
// POST — записывает новую метрику.
func ScenarioMetricsHandler(w http.ResponseWriter, r *http.Request) {
	collector := GetScenarioCollector()

	switch r.Method {
	case http.MethodGet:
		scenario := r.URL.Query().Get("scenario")
		w.Header().Set("Content-Type", "application/json")
		if scenario != "" {
			st := collector.Stats(scenario)
			if st == nil {
				http.Error(w, fmt.Sprintf(`{"error":"сценарий %q не найден"}`, scenario), http.StatusNotFound)
				return
			}
			json.NewEncoder(w).Encode(st)
		} else {
			json.NewEncoder(w).Encode(collector.AllStats())
		}

	case http.MethodPost:
		var rec ScenarioRecord
		if err := json.NewDecoder(r.Body).Decode(&rec); err != nil {
			http.Error(w, `{"error":"неверный формат запроса"}`, http.StatusBadRequest)
			return
		}
		collector.Record(rec.Scenario, rec.LatencyMs, rec.ToolCallCount, rec.Success, rec.ErrorMsg)
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintf(w, `{"status":"recorded","scenario":%q}`, rec.Scenario)

	default:
		http.Error(w, `{"error":"метод не поддерживается"}`, http.StatusMethodNotAllowed)
	}
}
