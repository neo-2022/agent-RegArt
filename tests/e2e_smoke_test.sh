#!/usr/bin/env bash
# Quality Gate — E2E smoke-тесты ключевых сценариев Admin.
# Запуск: bash tests/e2e_smoke_test.sh
# Требования: запущенные сервисы (agent-service:8083, tools-service:8082,
#              memory-service:8001, api-gateway:8080).
set -euo pipefail

GATEWAY="${GATEWAY_URL:-http://localhost:8080}"
AGENT="${AGENT_SERVICE_URL:-http://localhost:8083}"
TOOLS="${TOOLS_SERVICE_URL:-http://localhost:8082}"
MEMORY="${MEMORY_SERVICE_URL:-http://localhost:8001}"

PASS=0
FAIL=0
TOTAL=0

check() {
    local name="$1"
    local url="$2"
    local method="${3:-GET}"
    local body="${4:-}"
    local expect_code="${5:-200}"

    TOTAL=$((TOTAL + 1))
    local code
    if [ "$method" = "GET" ]; then
        code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 10 "$url" 2>/dev/null || echo "000")
    else
        code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 10 -X "$method" \
            -H "Content-Type: application/json" -d "$body" "$url" 2>/dev/null || echo "000")
    fi

    if [ "$code" = "$expect_code" ]; then
        echo "  [OK]   $name (HTTP $code)"
        PASS=$((PASS + 1))
    else
        echo "  [FAIL] $name (ожидался $expect_code, получен $code)"
        FAIL=$((FAIL + 1))
    fi
}

echo "=== Quality Gate: E2E Smoke-тесты ==="
echo ""

echo "--- 1. Health-проверки сервисов ---"
check "agent-service /health" "$AGENT/health"
check "tools-service /health" "$TOOLS/health"
check "memory-service /health" "$MEMORY/health"
check "api-gateway /health"   "$GATEWAY/health"

echo ""
echo "--- 2. Основные API-эндпоинты ---"
check "GET /agents"          "$AGENT/agents"
check "GET /models"          "$AGENT/models"
check "GET /providers"       "$AGENT/providers"
check "GET /workspaces"      "$AGENT/workspaces"
check "GET /prompts"         "$AGENT/prompts"

echo ""
echo "--- 3. Memory-service ---"
check "POST /facts"          "$MEMORY/facts" "POST" '{"text":"smoke test fact","workspace_id":1}'
check "GET /search"          "$MEMORY/search?q=smoke"
check "GET /learning-stats"  "$MEMORY/learning-stats?model=test"
check "GET /stats"           "$MEMORY/stats"

echo ""
echo "--- 4. Tools-service ---"
check "GET /sysinfo"         "$TOOLS/sysinfo"
check "GET /sysload"         "$TOOLS/sysload"

echo ""
echo "--- 5. Метрики и мониторинг ---"
check "GET /scenario-metrics"   "$AGENT/scenario-metrics"
check "POST /scenario-metrics"  "$AGENT/scenario-metrics" "POST" \
    '{"scenario":"smoke_test","latency_ms":100,"tool_call_count":1,"success":true}' "201"
check "GET /autoskill/patterns" "$AGENT/autoskill/patterns"
check "GET /autoskill/candidates" "$AGENT/autoskill/candidates"

echo ""
echo "--- 6. Gateway маршрутизация ---"
check "gateway -> /agents"   "$GATEWAY/agents"
check "gateway -> /models"   "$GATEWAY/models"
check "gateway -> /memory/health" "$GATEWAY/memory/health"
check "gateway -> /tools/sysinfo" "$GATEWAY/tools/sysinfo"

echo ""
echo "==================================="
echo "Итого: $TOTAL тестов | $PASS пройдено | $FAIL провалено"
echo "==================================="

if [ "$FAIL" -gt 0 ]; then
    echo "QUALITY GATE: НЕ ПРОЙДЕН"
    exit 1
else
    echo "QUALITY GATE: ПРОЙДЕН"
    exit 0
fi
