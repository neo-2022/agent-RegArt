package metrics

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestScenarioCollector_Record(t *testing.T) {
	c := NewScenarioCollector(100)
	c.Record("chat", 150, 3, true, "")
	c.Record("chat", 250, 5, false, "timeout")

	st := c.Stats("chat")
	if st == nil {
		t.Fatal("статистика не найдена")
	}
	if st.TotalRuns != 2 {
		t.Fatalf("ожидалось 2 запуска, получено %d", st.TotalRuns)
	}
	if st.SuccessCount != 1 {
		t.Fatalf("ожидался 1 успех, получено %d", st.SuccessCount)
	}
	if st.ErrorCount != 1 {
		t.Fatalf("ожидалась 1 ошибка, получено %d", st.ErrorCount)
	}
	if st.SuccessRate < 0.49 || st.SuccessRate > 0.51 {
		t.Fatalf("ожидался success_rate 0.5, получено %.2f", st.SuccessRate)
	}
}

func TestScenarioCollector_Latency(t *testing.T) {
	c := NewScenarioCollector(100)
	c.Record("perf", 100, 1, true, "")
	c.Record("perf", 200, 2, true, "")
	c.Record("perf", 300, 3, true, "")

	st := c.Stats("perf")
	if st.AvgLatencyMs < 199 || st.AvgLatencyMs > 201 {
		t.Fatalf("ожидалась средняя 200, получено %.1f", st.AvgLatencyMs)
	}
	if st.P50LatencyMs < 199 || st.P50LatencyMs > 201 {
		t.Fatalf("ожидался p50 ~200, получено %.1f", st.P50LatencyMs)
	}
	if st.MaxLatencyMs != 300 {
		t.Fatalf("ожидался max 300, получено %.1f", st.MaxLatencyMs)
	}
}

func TestScenarioCollector_AvgToolCalls(t *testing.T) {
	c := NewScenarioCollector(100)
	c.Record("tools", 100, 2, true, "")
	c.Record("tools", 100, 4, true, "")

	st := c.Stats("tools")
	if st.AvgToolCalls < 2.9 || st.AvgToolCalls > 3.1 {
		t.Fatalf("ожидалось avg_tool_calls 3, получено %.1f", st.AvgToolCalls)
	}
}

func TestScenarioCollector_AllStats(t *testing.T) {
	c := NewScenarioCollector(100)
	c.Record("a", 100, 1, true, "")
	c.Record("b", 200, 2, false, "err")

	all := c.AllStats()
	if len(all) != 2 {
		t.Fatalf("ожидалось 2 сценария, получено %d", len(all))
	}
}

func TestScenarioCollector_Reset(t *testing.T) {
	c := NewScenarioCollector(100)
	c.Record("x", 100, 1, true, "")
	c.Reset("x")
	if st := c.Stats("x"); st != nil {
		t.Fatal("после сброса статистика должна быть nil")
	}
}

func TestScenarioCollector_MaxKeep(t *testing.T) {
	c := NewScenarioCollector(5)
	for i := 0; i < 10; i++ {
		c.Record("overflow", float64(i*100), 1, true, "")
	}
	c.mu.RLock()
	n := len(c.records["overflow"])
	c.mu.RUnlock()
	if n != 5 {
		t.Fatalf("ожидалось 5 записей (maxKeep), получено %d", n)
	}
}

func TestScenarioCollector_NotFound(t *testing.T) {
	c := NewScenarioCollector(100)
	if st := c.Stats("nonexistent"); st != nil {
		t.Fatal("ожидался nil для несуществующего сценария")
	}
}

func TestScenarioMetricsHandler_POST(t *testing.T) {
	body := `{"scenario":"test_handler","latency_ms":150,"tool_call_count":2,"success":true}`
	req := httptest.NewRequest(http.MethodPost, "/scenario-metrics", strings.NewReader(body))
	w := httptest.NewRecorder()
	ScenarioMetricsHandler(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("ожидался 201, получен %d", w.Code)
	}
}

func TestScenarioMetricsHandler_GET(t *testing.T) {
	collector := GetScenarioCollector()
	collector.Record("handler_test", 100, 1, true, "")

	req := httptest.NewRequest(http.MethodGet, "/scenario-metrics?scenario=handler_test", nil)
	w := httptest.NewRecorder()
	ScenarioMetricsHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("ожидался 200, получен %d", w.Code)
	}

	var st ScenarioStats
	if err := json.NewDecoder(w.Body).Decode(&st); err != nil {
		t.Fatal(err)
	}
	if st.Scenario != "handler_test" {
		t.Fatalf("ожидался сценарий handler_test, получен %s", st.Scenario)
	}
}
