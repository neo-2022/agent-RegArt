-- Создание таблицы для хранения документов в RAG
CREATE TABLE IF NOT EXISTS rag_docs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    doc_id TEXT UNIQUE NOT NULL,
    title TEXT NOT NULL,
    content TEXT NOT NULL,
    source TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Создание индексов отдельными командами (правильный синтаксис для SQLite)
CREATE INDEX IF NOT EXISTS idx_rag_doc_id ON rag_docs(doc_id);
CREATE INDEX IF NOT EXISTS idx_rag_source ON rag_docs(source);
CREATE INDEX IF NOT EXISTS idx_rag_created_at ON rag_docs(created_at);
