import unittest
import sqlite3
from pathlib import Path

class TestRAGTable(unittest.TestCase):
    def setUp(self):
        # Подключаемся к базе данных
        self.db_path = Path("agentcore.db")
        self.conn = sqlite3.connect(self.db_path)
        self.cursor = self.conn.cursor()
    
    def tearDown(self):
        # Закрываем соединение
        self.conn.close()
    
    def test_table_exists(self):
        """Проверяем, что таблица rag_docs существует"""
        self.cursor.execute("SELECT name FROM sqlite_master WHERE type='table' AND name='rag_docs'")
        result = self.cursor.fetchone()
        self.assertIsNotNone(result, "Таблица rag_docs не найдена в базе данных")
    
    def test_table_structure(self):
        """Проверяем структуру таблицы"""
        self.cursor.execute("PRAGMA table_info(rag_docs)")
        columns = self.cursor.fetchall()
        
        expected_columns = [
            (0, 'id', 'INTEGER', 0, None, 1),
            (1, 'doc_id', 'TEXT', 1, None, 0),
            (2, 'title', 'TEXT', 1, None, 0),
            (3, 'content', 'TEXT', 1, None, 0),
            (4, 'source', 'TEXT', 1, None, 0),
            (5, 'created_at', 'DATETIME', 0, 'CURRENT_TIMESTAMP', 0),
            (6, 'updated_at', 'DATETIME', 0, 'CURRENT_TIMESTAMP', 0)
        ]
        
        self.assertEqual(len(columns), len(expected_columns))
        for i, col in enumerate(columns):
            self.assertEqual(col, expected_columns[i], f"Столбец {i} не соответствует ожидаемому")
    
    def test_index_creation(self):
        """Проверяем создание индексов"""
        # Проверяем существование индексов
        indices = [
            "idx_rag_doc_id",
            "idx_rag_source",
            "idx_rag_created_at"
        ]
        
        for idx_name in indices:
            self.cursor.execute(f"SELECT name FROM sqlite_master WHERE type='index' AND name='{idx_name}'")
            result = self.cursor.fetchone()
            self.assertIsNotNone(result, f"Индекс {idx_name} не создан")

if __name__ == '__main__':
    unittest.main()
