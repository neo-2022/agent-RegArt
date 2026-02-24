#!/bin/bash
# Проверяем, запущен ли сервер на порту 3000
if ss -tulpn | grep -q ":3000 "; then
    echo "3000"
else
    # Если нет, пробуем найти запущенный процесс serve
    PORT=$(pgrep -f "serve.*dist.*3000" | head -1 | xargs -I {} ps -o args= {} | sed -n 's/.*-l \([0-9]*\).*/\1/p')
    if [ -n "$PORT" ]; then
        echo "$PORT"
    else
        echo "3000"
    fi
fi
