#!/usr/bin/env bash
set -euo pipefail

echo "=== Security Scan ==="
ERRORS=0

# Go: gosec (if installed)
if command -v gosec &>/dev/null; then
  echo "--- gosec: agent-service ---"
  gosec ./agent-service/... || ERRORS=$((ERRORS+1))
  echo "--- gosec: api-gateway ---"
  gosec ./api-gateway/... || ERRORS=$((ERRORS+1))
  echo "--- gosec: tools-service ---"
  gosec ./tools-service/... || ERRORS=$((ERRORS+1))
else
  echo "[SKIP] gosec not installed (go install github.com/securego/gosec/v2/cmd/gosec@latest)"
fi

# Python: bandit (if installed)
if command -v bandit &>/dev/null; then
  echo "--- bandit: memory-service ---"
  bandit -r memory-service/app/ -ll || ERRORS=$((ERRORS+1))
else
  echo "[SKIP] bandit not installed (pip install bandit)"
fi

# Secrets scan: check for hardcoded secrets
echo "--- Hardcoded secrets check ---"
PATTERNS='(password|secret|api_key|token|credential)\s*[:=]\s*["\x27][^"\x27]{8,}'
if grep -rEi "$PATTERNS" --include="*.go" --include="*.py" --include="*.ts" --include="*.tsx" \
   --exclude-dir=node_modules --exclude-dir=.git --exclude-dir=vendor . 2>/dev/null; then
  echo "[WARN] Potential hardcoded secrets found above"
  ERRORS=$((ERRORS+1))
else
  echo "[OK] No hardcoded secrets detected"
fi

# Dependency check: go.sum exists
echo "--- Dependency files ---"
for svc in agent-service api-gateway tools-service; do
  if [ -f "$svc/go.sum" ]; then
    echo "[OK] $svc/go.sum exists"
  else
    echo "[WARN] $svc/go.sum missing"
  fi
done

echo "=== Scan complete (errors: $ERRORS) ==="
exit $ERRORS
