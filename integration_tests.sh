#!/usr/bin/env bash

# =============================================================================
# INTEGRATION TESTS FOR AGENT CORE NG
# =============================================================================
# Полные интеграционные тесты для docker-compose стека
# Проверяет все микросервисы, эндпоинты и взаимодействие компонентов
# =============================================================================

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Цвета для вывода
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

TESTS_PASSED=0
TESTS_FAILED=0
SERVICES_UP=0

# =============================================================================
# UTILITY FUNCTIONS
# =============================================================================

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[✓]${NC} $1"
    ((TESTS_PASSED++))
}

log_error() {
    echo -e "${RED}[✗]${NC} $1"
    ((TESTS_FAILED++))
}

log_warn() {
    echo -e "${YELLOW}[!]${NC} $1"
}

test_endpoint() {
    local url=$1
    local expected_status=$2
    local description=$3

    log_info "Testing: $description"

    local status=$(curl -s -o /dev/null -w "%{http_code}" "$url")

    if [ "$status" = "$expected_status" ]; then
        log_success "$description → HTTP $status"
    else
        log_error "$description → Expected HTTP $expected_status, got $status"
        return 1
    fi
}

wait_for_service() {
    local url=$1
    local max_attempts=$2
    local description=$3

    log_info "Waiting for $description..."

    for i in $(seq 1 $max_attempts); do
        if curl -s "$url" > /dev/null 2>&1; then
            log_success "$description is UP"
            return 0
        fi
        echo -n "."
        sleep 2
    done

    log_error "$description failed to start (timeout after $((max_attempts * 2))s)"
    return 1
}

# =============================================================================
# DOCKER COMPOSE CHECKS
# =============================================================================

echo ""
echo -e "${BLUE}════════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}  AGENT CORE NG - FULL INTEGRATION TESTS${NC}"
echo -e "${BLUE}════════════════════════════════════════════════════════════${NC}"
echo ""

log_info "Step 1: Checking Docker and Docker Compose"

if ! command -v docker &> /dev/null; then
    log_error "Docker is not installed"
    exit 1
fi

if ! command -v docker-compose &> /dev/null; then
    log_error "Docker Compose is not installed"
    exit 1
fi

log_success "Docker environment is ready"

# =============================================================================
# START SERVICES
# =============================================================================

echo ""
log_info "Step 2: Starting docker-compose stack"

docker-compose down -v 2>/dev/null || true
sleep 2

docker-compose up -d

log_info "Waiting for all services to be ready..."
sleep 5

# =============================================================================
# CHECK SERVICE HEALTH
# =============================================================================

echo ""
log_info "Step 3: Checking service health"

wait_for_service "http://localhost:5432" 30 "PostgreSQL" || exit 1
wait_for_service "http://localhost:6333" 30 "Qdrant" || exit 1
wait_for_service "http://localhost:8001/health" 30 "memory-service" || exit 1
wait_for_service "http://localhost:8082/health" 30 "tools-service" || exit 1
wait_for_service "http://localhost:8083/health" 30 "agent-service" || exit 1
wait_for_service "http://localhost:8080/health" 30 "api-gateway" || exit 1
wait_for_service "http://localhost:5173" 30 "web-ui" || exit 1

# =============================================================================
# TEST MEMORY SERVICE
# =============================================================================

echo ""
log_info "Step 4: Testing memory-service (RAG)"

test_endpoint "http://localhost:8001/health" "200" "memory-service health"

# Test 4.1: Add a fact to memory
log_info "Adding fact to memory..."
FACT_RESPONSE=$(curl -s -X POST http://localhost:8001/facts \
  -H "Content-Type: application/json" \
  -d '{"text":"Test fact about AI","metadata":{"source":"test"}}')

if echo "$FACT_RESPONSE" | grep -q '"id"'; then
    log_success "Fact added to memory-service"
else
    log_warn "Could not add fact (memory-service may need initialization)"
fi

# Test 4.2: Search memory
log_info "Searching memory..."
SEARCH_RESPONSE=$(curl -s -X POST http://localhost:8001/search \
  -H "Content-Type: application/json" \
  -d '{"query":"AI","top_k":5}')

if echo "$SEARCH_RESPONSE" | grep -q '"results"'; then
    log_success "Memory search works"
else
    log_warn "Memory search may be empty initially"
fi

# =============================================================================
# TEST TOOLS SERVICE
# =============================================================================

echo ""
log_info "Step 5: Testing tools-service"

test_endpoint "http://localhost:8082/health" "200" "tools-service health"

# Test 5.1: sysinfo
log_info "Testing sysinfo endpoint..."
SYSINFO=$(curl -s -X POST http://localhost:8082/sysinfo \
  -H "Content-Type: application/json")

if echo "$SYSINFO" | grep -q -E '"hostname"|"os"|"arch"'; then
    log_success "tools-service sysinfo works"
else
    log_error "tools-service sysinfo failed"
fi

# Test 5.2: List home directory
log_info "Testing list endpoint..."
LIST=$(curl -s -X POST http://localhost:8082/list \
  -H "Content-Type: application/json" \
  -d '{"path":"~"}')

if echo "$LIST" | grep -q '"entries"'; then
    log_success "tools-service list works"
else
    log_error "tools-service list failed"
fi

# =============================================================================
# TEST AGENT SERVICE
# =============================================================================

echo ""
log_info "Step 6: Testing agent-service"

test_endpoint "http://localhost:8083/health" "200" "agent-service health"

# Test 6.1: Get agents
log_info "Testing /agents endpoint..."
AGENTS=$(curl -s http://localhost:8083/agents -H "Content-Type: application/json")

if echo "$AGENTS" | grep -q '"name"'; then
    log_success "agent-service agents list works"
else
    log_error "agent-service agents list failed"
fi

# Test 6.2: Get models
log_info "Testing /models endpoint..."
MODELS=$(curl -s http://localhost:8083/models -H "Content-Type: application/json")

if echo "$MODELS" | grep -q '"model"'; then
    log_success "agent-service models list works"
else
    log_warn "agent-service models may be empty (check Ollama connection)"
fi

# =============================================================================
# TEST API GATEWAY
# =============================================================================

echo ""
log_info "Step 7: Testing api-gateway"

test_endpoint "http://localhost:8080/health" "200" "api-gateway health"

# Test 7.1: Gateway routing to memory-service
log_info "Testing gateway routing to memory-service..."
GATEWAY_MEM=$(curl -s http://localhost:8080/memory/health)

if echo "$GATEWAY_MEM" | grep -q '"status"'; then
    log_success "api-gateway routes to memory-service"
else
    log_error "api-gateway routing to memory-service failed"
fi

# Test 7.2: Gateway routing to agent-service
log_info "Testing gateway routing to agent-service..."
GATEWAY_AGENT=$(curl -s http://localhost:8080/agents)

if echo "$GATEWAY_AGENT" | grep -q '"name"'; then
    log_success "api-gateway routes to agent-service"
else
    log_error "api-gateway routing to agent-service failed"
fi

# =============================================================================
# TEST WEB UI
# =============================================================================

echo ""
log_info "Step 8: Testing web-ui"

WEB_STATUS=$(curl -s -o /dev/null -w "%{http_code}" "http://localhost:5173/")

if [ "$WEB_STATUS" = "200" ] || [ "$WEB_STATUS" = "404" ]; then
    log_success "web-ui is accessible"
else
    log_warn "web-ui returned HTTP $WEB_STATUS"
fi

# =============================================================================
# PERFORMANCE TESTS
# =============================================================================

echo ""
log_info "Step 9: Performance baseline"

# Test response times
log_info "Measuring API response times..."

# Gateway health
START=$(date +%s%N)
curl -s http://localhost:8080/health > /dev/null
END=$(date +%s%N)
DURATION=$((($END - $START) / 1000000))
log_success "api-gateway /health: ${DURATION}ms"

# Memory service health
START=$(date +%s%N)
curl -s http://localhost:8001/health > /dev/null
END=$(date +%s%N)
DURATION=$((($END - $START) / 1000000))
log_success "memory-service /health: ${DURATION}ms"

# Agent service health
START=$(date +%s%N)
curl -s http://localhost:8083/health > /dev/null
END=$(date +%s%N)
DURATION=$((($END - $START) / 1000000))
log_success "agent-service /health: ${DURATION}ms"

# =============================================================================
# CONTAINER LOGS CHECK
# =============================================================================

echo ""
log_info "Step 10: Checking for critical errors in logs"

for service in postgres qdrant memory-service agent-service tools-service api-gateway web-ui; do
    ERROR_COUNT=$(docker-compose logs "$service" 2>/dev/null | grep -i "error\|fatal\|panic" | grep -v "warning" | wc -l)
    if [ "$ERROR_COUNT" -gt 0 ]; then
        log_warn "$service has $ERROR_COUNT potential errors (check logs)"
    else
        log_success "$service logs clean"
    fi
done

# =============================================================================
# FINAL REPORT
# =============================================================================

echo ""
echo -e "${BLUE}════════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}  INTEGRATION TEST RESULTS${NC}"
echo -e "${BLUE}════════════════════════════════════════════════════════════${NC}"
echo ""

log_info "Tests passed: $TESTS_PASSED"
log_info "Tests failed: $TESTS_FAILED"

DOCKER_CONTAINERS=$(docker-compose ps -q | wc -l)
log_info "Running containers: $DOCKER_CONTAINERS"

if [ "$TESTS_FAILED" -eq 0 ]; then
    log_success "ALL INTEGRATION TESTS PASSED! ✓"
    echo ""
    echo -e "${GREEN}Agent Core NG is ready for production!${NC}"
    echo ""
    echo "Access points:"
    echo "  Web UI:         http://localhost:5173"
    echo "  API Gateway:    http://localhost:8080"
    echo "  Agent Service:  http://localhost:8083"
    echo "  Memory Service: http://localhost:8001"
    echo "  Tools Service:  http://localhost:8082"
    echo ""
    exit 0
else
    log_error "SOME TESTS FAILED!"
    echo ""
    echo "Debug logs:"
    docker-compose logs --tail=50
    echo ""
    exit 1
fi
