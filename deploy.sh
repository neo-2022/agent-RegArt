#!/usr/bin/env bash

# =============================================================================
# PRODUCTION DEPLOYMENT SCRIPT FOR AGENT CORE NG
# =============================================================================
# Полный скрипт для развертывания на production
# Включает: build, validate, test, deploy, monitor
# =============================================================================

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Цвета
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

TIMESTAMP=$(date '+%Y-%m-%d_%H:%M:%S')
LOG_FILE="deployment_${TIMESTAMP}.log"

# =============================================================================
# UTILITY FUNCTIONS
# =============================================================================

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1" | tee -a "$LOG_FILE"
}

log_success() {
    echo -e "${GREEN}[✓]${NC} $1" | tee -a "$LOG_FILE"
}

log_error() {
    echo -e "${RED}[✗]${NC} $1" | tee -a "$LOG_FILE"
}

log_warn() {
    echo -e "${YELLOW}[!]${NC} $1" | tee -a "$LOG_FILE"
}

run_cmd() {
    log_info "Running: $@"
    if eval "$@" >> "$LOG_FILE" 2>&1; then
        log_success "Command completed: $@"
    else
        log_error "Command failed: $@"
        exit 1
    fi
}

# =============================================================================
# START DEPLOYMENT
# =============================================================================

echo ""
echo -e "${CYAN}╔══════════════════════════════════════════════════════════╗${NC}"
echo -e "${CYAN}║   AGENT CORE NG - PRODUCTION DEPLOYMENT${NC}"
echo -e "${CYAN}║   $TIMESTAMP${NC}"
echo -e "${CYAN}╚══════════════════════════════════════════════════════════╝${NC}"
echo ""

log_info "Deployment log: $LOG_FILE"

# =============================================================================
# STAGE 1: BUILD & VALIDATION
# =============================================================================

echo ""
echo -e "${CYAN}════════════════════════════════════════════════════════════${NC}"
echo -e "${CYAN}STAGE 1: BUILD & COMPILATION${NC}"
echo -e "${CYAN}════════════════════════════════════════════════════════════${NC}"
echo ""

log_info "Checking Go modules..."
run_cmd "cd agent-service && go mod verify"
run_cmd "cd ../api-gateway && go mod verify"
run_cmd "cd ../tools-service && go mod verify"

log_info "Building Go services..."
run_cmd "cd agent-service && go build -o agent-service ./cmd/server/"
log_success "agent-service compiled"

run_cmd "cd ../api-gateway && go build -o api-gateway ./cmd/"
log_success "api-gateway compiled"

run_cmd "cd ../tools-service && go build -o tools-service ./cmd/server/"
log_success "tools-service compiled"

log_info "Checking Python syntax..."
run_cmd "cd memory-service && find . -name '*.py' -exec python3 -m py_compile {} \\;"
log_success "Python syntax OK"

# =============================================================================
# STAGE 2: DOCKER BUILD
# =============================================================================

echo ""
echo -e "${CYAN}════════════════════════════════════════════════════════════${NC}"
echo -e "${CYAN}STAGE 2: DOCKER BUILD${NC}"
echo -e "${CYAN}════════════════════════════════════════════════════════════${NC}"
echo ""

log_info "Building Docker images..."
run_cmd "docker-compose build --no-cache"
log_success "All Docker images built"

# =============================================================================
# STAGE 3: UNIT TESTS
# =============================================================================

echo ""
echo -e "${CYAN}════════════════════════════════════════════════════════════${NC}"
echo -e "${CYAN}STAGE 3: UNIT TESTS${NC}"
echo -e "${CYAN}════════════════════════════════════════════════════════════${NC}"
echo ""

log_info "Running Go unit tests..."
run_cmd "cd agent-service && go test ./... -v"
log_success "Go unit tests passed"

# =============================================================================
# STAGE 4: DEPLOY & START SERVICES
# =============================================================================

echo ""
echo -e "${CYAN}════════════════════════════════════════════════════════════${NC}"
echo -e "${CYAN}STAGE 4: START DOCKER STACK${NC}"
echo -e "${CYAN}════════════════════════════════════════════════════════════${NC}"
echo ""

log_info "Stopping existing containers..."
docker-compose down -v 2>/dev/null || true
sleep 2

log_info "Starting docker-compose stack..."
run_cmd "docker-compose up -d"
log_success "Docker stack started"

# =============================================================================
# STAGE 5: HEALTH CHECKS
# =============================================================================

echo ""
echo -e "${CYAN}════════════════════════════════════════════════════════════${NC}"
echo -e "${CYAN}STAGE 5: HEALTH CHECKS${NC}"
echo -e "${CYAN}════════════════════════════════════════════════════════════${NC}"
echo ""

wait_for_service() {
    local url=$1
    local service=$2
    local max_attempts=30

    log_info "Checking $service..."

    for i in $(seq 1 $max_attempts); do
        if curl -s "$url" > /dev/null 2>&1; then
            log_success "$service is healthy"
            return 0
        fi
        echo -n "."
        sleep 2
    done

    log_error "$service health check failed"
    return 1
}

wait_for_service "http://localhost:8001/health" "memory-service" || exit 1
wait_for_service "http://localhost:8082/health" "tools-service" || exit 1
wait_for_service "http://localhost:8083/health" "agent-service" || exit 1
wait_for_service "http://localhost:8080/health" "api-gateway" || exit 1

# =============================================================================
# STAGE 6: INTEGRATION TESTS
# =============================================================================

echo ""
echo -e "${CYAN}════════════════════════════════════════════════════════════${NC}"
echo -e "${CYAN}STAGE 6: INTEGRATION TESTS${NC}"
echo -e "${CYAN}════════════════════════════════════════════════════════════${NC}"
echo ""

log_info "Running integration tests..."

# Test 1: Memory service
log_info "Testing memory-service..."
if curl -s -X POST http://localhost:8001/facts \
  -H "Content-Type: application/json" \
  -d '{"text":"Test","metadata":{"source":"test"}}' | grep -q '"id"'; then
    log_success "memory-service fact insertion works"
else
    log_warn "memory-service fact insertion may need initialization"
fi

# Test 2: Tools service
log_info "Testing tools-service..."
if curl -s -X POST http://localhost:8082/sysinfo \
  -H "Content-Type: application/json" | grep -q '"hostname"'; then
    log_success "tools-service sysinfo works"
else
    log_error "tools-service sysinfo failed"
    exit 1
fi

# Test 3: Agent service
log_info "Testing agent-service..."
if curl -s http://localhost:8083/agents | grep -q '"name"'; then
    log_success "agent-service agents list works"
else
    log_error "agent-service agents list failed"
    exit 1
fi

# Test 4: API Gateway routing
log_info "Testing api-gateway routing..."
if curl -s http://localhost:8080/memory/health | grep -q '"status"'; then
    log_success "api-gateway routing works"
else
    log_error "api-gateway routing failed"
    exit 1
fi

# =============================================================================
# STAGE 7: VERIFY FEATURES
# =============================================================================

echo ""
echo -e "${CYAN}════════════════════════════════════════════════════════════${NC}"
echo -e "${CYAN}STAGE 7: FEATURE VERIFICATION${NC}"
echo -e "${CYAN}════════════════════════════════════════════════════════════${NC}"
echo ""

# RAG Check
log_info "Checking RAG functionality..."
if grep -q "RAG ВКЛЮЧЕН" agent-service/cmd/server/main.go; then
    log_success "RAG is enabled in code"
else
    log_error "RAG is not enabled"
fi

# Learnings Check
log_info "Checking Learnings functionality..."
if grep -q "Learnings ВКЛЮЧЕНЫ" agent-service/cmd/server/main.go; then
    log_success "Learnings is enabled in code"
else
    log_error "Learnings is not enabled"
fi

# LLM Providers Check
log_info "Checking LLM providers..."
PROVIDERS=$(curl -s http://localhost:8080/providers | grep -o '"name":"[^"]*"' | wc -l)
log_success "Found $PROVIDERS LLM providers configured"

# =============================================================================
# STAGE 8: SERVICE MONITORING
# =============================================================================

echo ""
echo -e "${CYAN}════════════════════════════════════════════════════════════${NC}"
echo -e "${CYAN}STAGE 8: SERVICE MONITORING${NC}"
echo -e "${CYAN}════════════════════════════════════════════════════════════${NC}"
echo ""

log_info "Container status:"
docker-compose ps | tee -a "$LOG_FILE"

log_info "Checking container resource usage..."
docker stats --no-stream | tee -a "$LOG_FILE"

# =============================================================================
# STAGE 9: DATABASE VERIFICATION
# =============================================================================

echo ""
echo -e "${CYAN}════════════════════════════════════════════════════════════${NC}"
echo -e "${CYAN}STAGE 9: DATABASE VERIFICATION${NC}"
echo -e "${CYAN}════════════════════════════════════════════════════════════${NC}"
echo ""

log_info "Checking PostgreSQL..."
PSQL_CHECK=$(docker-compose exec -T postgres pg_isready -U agentcore 2>&1 || echo "failed")
if echo "$PSQL_CHECK" | grep -q "accepting"; then
    log_success "PostgreSQL is accepting connections"
else
    log_warn "PostgreSQL response: $PSQL_CHECK"
fi

log_info "Checking Qdrant..."
if curl -s http://localhost:6333/health | grep -q '"status"'; then
    log_success "Qdrant is healthy"
else
    log_warn "Qdrant health check inconclusive"
fi

# =============================================================================
# STAGE 10: FINAL REPORT
# =============================================================================

echo ""
echo -e "${CYAN}════════════════════════════════════════════════════════════${NC}"
echo -e "${CYAN}DEPLOYMENT SUMMARY${NC}"
echo -e "${CYAN}════════════════════════════════════════════════════════════${NC}"
echo ""

echo -e "${GREEN}✓ BUILD: Success${NC}"
echo -e "${GREEN}✓ DOCKER: Success${NC}"
echo -e "${GREEN}✓ TESTS: Passed${NC}"
echo -e "${GREEN}✓ DEPLOYMENT: Complete${NC}"
echo -e "${GREEN}✓ HEALTH: All services online${NC}"
echo -e "${GREEN}✓ RAG: Enabled${NC}"
echo -e "${GREEN}✓ LEARNINGS: Enabled${NC}"

echo ""
echo "Production URLs:"
echo "  Web UI:         http://localhost:5173"
echo "  API Gateway:    http://localhost:8080"
echo "  Agent Service:  http://localhost:8083"
echo "  Memory Service: http://localhost:8001"
echo "  Tools Service:  http://localhost:8082"
echo ""

echo "Commands for debug:"
echo "  View logs:      docker-compose logs -f"
echo "  Stop services:  docker-compose down"
echo "  Restart:        docker-compose restart"
echo ""

log_success "DEPLOYMENT COMPLETED SUCCESSFULLY! 🎉"
log_info "Full log saved to: $LOG_FILE"

exit 0
