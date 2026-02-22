#!/usr/bin/env bash
# Скрипт безопасности: проверка кода на уязвимости и захардкоженные секреты.
#
# Выполняемые проверки:
# 1. gosec — статический анализ Go-кода на уязвимости (если установлен)
# 2. bandit — статический анализ Python-кода на уязвимости (если установлен)
# 3. Поиск захардкоженных секретов (пароли, токены, ключи API) в коде
# 4. Проверка наличия файлов зависимостей (go.sum)
#
# Код возврата равен количеству найденных ошибок (0 = всё чисто).
set -euo pipefail

echo "=== Сканирование безопасности ==="
ERRORS=0

# Go: gosec — статический анализатор безопасности для Go (если установлен)
if command -v gosec &>/dev/null; then
  echo "--- gosec: agent-service ---"
  gosec ./agent-service/... || ERRORS=$((ERRORS+1))
  echo "--- gosec: api-gateway ---"
  gosec ./api-gateway/... || ERRORS=$((ERRORS+1))
  echo "--- gosec: tools-service ---"
  gosec ./tools-service/... || ERRORS=$((ERRORS+1))
else
  echo "[ПРОПУСК] gosec не установлен (go install github.com/securego/gosec/v2/cmd/gosec@latest)"
fi

# Python: bandit — статический анализатор безопасности для Python (если установлен)
if command -v bandit &>/dev/null; then
  echo "--- bandit: memory-service ---"
  bandit -r memory-service/app/ -ll || ERRORS=$((ERRORS+1))
else
  echo "[ПРОПУСК] bandit не установлен (pip install bandit)"
fi

# Поиск захардкоженных секретов: пароли, токены, ключи API в коде
echo "--- Проверка захардкоженных секретов ---"
PATTERNS='(password|secret|api_key|token|credential)\s*[:=]\s*["\x27][^"\x27]{8,}'
if grep -rEi "$PATTERNS" --include="*.go" --include="*.py" --include="*.ts" --include="*.tsx" \
   --exclude-dir=node_modules --exclude-dir=.git --exclude-dir=vendor . 2>/dev/null; then
  echo "[ВНИМАНИЕ] Обнаружены потенциальные захардкоженные секреты (см. выше)"
  ERRORS=$((ERRORS+1))
else
  echo "[OK] Захардкоженные секреты не обнаружены"
fi

# Проверка файлов зависимостей: наличие go.sum для каждого Go-сервиса
echo "--- Файлы зависимостей ---"
for svc in agent-service api-gateway tools-service; do
  if [ -f "$svc/go.sum" ]; then
    echo "[OK] $svc/go.sum существует"
  else
    echo "[ВНИМАНИЕ] $svc/go.sum отсутствует"
  fi
done

echo "=== Сканирование завершено (ошибок: $ERRORS) ==="
exit $ERRORS
