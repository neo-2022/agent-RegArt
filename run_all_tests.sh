#!/bin/bash
# Скрипт для запуска всех unit-тестов проекта

set -e

PROJECT_ROOT="/home/art/agent-RegArt"
FAILED=0
PASSED=0

# Цвета для вывода
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}================================${NC}"
echo -e "${BLUE}Unit Tests Execution${NC}"
echo -e "${BLUE}================================${NC}\n"

# ===== GO TESTS =====
echo -e "${YELLOW}[1/4] Running tools-service/executor tests...${NC}"
if cd "$PROJECT_ROOT/tools-service" && go test ./internal/executor -v -timeout 30s; then
    echo -e "${GREEN}✓ executor tests PASSED${NC}\n"
    ((PASSED++))
else
    echo -e "${RED}✗ executor tests FAILED${NC}\n"
    ((FAILED++))
fi

echo -e "${YELLOW}[2/4] Running agent-service/llm tests...${NC}"
if cd "$PROJECT_ROOT/agent-service" && go test ./internal/llm -v -timeout 30s; then
    echo -e "${GREEN}✓ llm/registry tests PASSED${NC}\n"
    ((PASSED++))
else
    echo -e "${RED}✗ llm/registry tests FAILED${NC}\n"
    ((FAILED++))
fi

# ===== PYTHON TESTS =====
echo -e "${YELLOW}[3/4] Running memory-service/ranking tests...${NC}"
if cd "$PROJECT_ROOT/memory-service"; then
    if [ ! -d venv ]; then
        echo -e "${YELLOW}  Creating virtual environment...${NC}"
        python3 -m venv venv
    fi

    if source venv/bin/activate && \
       pip install -q -r requirements.txt pytest && \
       pytest tests/test_ranking.py -v --tb=short; then
        echo -e "${GREEN}✓ ranking tests PASSED${NC}\n"
        ((PASSED++))
    else
        echo -e "${RED}✗ ranking tests FAILED${NC}\n"
        ((FAILED++))
    fi
else
    echo -e "${RED}✗ Could not access memory-service${NC}\n"
    ((FAILED++))
fi

echo -e "${YELLOW}[4/4] Running memory-service/memory tests...${NC}"
if cd "$PROJECT_ROOT/memory-service"; then
    if source venv/bin/activate && \
       pytest tests/test_memory.py -v --tb=short; then
        echo -e "${GREEN}✓ memory/soft-delete tests PASSED${NC}\n"
        ((PASSED++))
    else
        echo -e "${RED}✗ memory/soft-delete tests FAILED${NC}\n"
        ((FAILED++))
    fi
else
    echo -e "${RED}✗ Could not access memory-service${NC}\n"
    ((FAILED++))
fi

# ===== SUMMARY =====
echo -e "${BLUE}================================${NC}"
echo -e "${BLUE}Test Summary${NC}"
echo -e "${BLUE}================================${NC}"
echo -e "Passed: ${GREEN}${PASSED}/4${NC}"
echo -e "Failed: ${RED}${FAILED}/4${NC}"

if [ $FAILED -eq 0 ]; then
    echo -e "\n${GREEN}All tests passed!${NC}"
    exit 0
else
    echo -e "\n${RED}Some tests failed!${NC}"
    exit 1
fi
