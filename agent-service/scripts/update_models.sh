#!/bin/bash
# Скрипт для автоматического обновления списка моделей в базе данных
# Находит все файлы моделей в /models и обновляет их в базе данных

MODELS_DIR="agent-service/models"
DB_PATH="agentcore.db"

# Проверяем существование базы данных
if [ ! -f "$DB_PATH" ]; then
    echo "Ошибка: База данных $DB_PATH не найдена!" >&2
    exit 1
fi

# Создаем временный файл для SQL-запросов
TMP_SQL=$(mktemp)

# Генерируем SQL для добавления/обновления моделей
find "$MODELS_DIR" -type f \( -name "*.bin" -o -name "*.gguf" \) | while read -r model_path; do
    model_name=$(basename "$model_path")
    provider="local"
    version=$(date -r "$model_path" "+%Y.%m.%d")
    
    cat >> "$TMP_SQL" << SQL
INSERT INTO models (name, provider, version)
VALUES ('$model_name', '$provider', '$version')
ON CONFLICT(name) DO UPDATE SET
    provider = excluded.provider,
    version = excluded.version,
    updated_at = CURRENT_TIMESTAMP;
SQL
done

# Выполняем SQL-запросы
if [ -s "$TMP_SQL" ]; then
    sqlite3 "$DB_PATH" < "$TMP_SQL"
    echo "Модели успешно обновлены в базе данных"
else
    echo "Не найдено моделей для обновления в $MODELS_DIR"
fi

rm -f "$TMP_SQL"
