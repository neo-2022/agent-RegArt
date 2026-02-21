FROM python:3.12-slim

WORKDIR /app

# Установка системных зависимостей, необходимых для некоторых библиотек (pypdf, chromadb)
RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc \
    g++ \
    make \
    && rm -rf /var/lib/apt/lists/*

# Копирование requirements и установка зависимостей
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Копирование кода приложения
COPY app/ ./app/

# Создание директорий для данных
RUN mkdir -p /app/data/chroma /app/data/temp

# Порт, на котором работает приложение
EXPOSE 8001

# Запуск сервера
CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8001"]
