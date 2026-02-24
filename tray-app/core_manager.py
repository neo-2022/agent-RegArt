"""
Центральный менеджер системы для управления процессами, логированием и мониторингом
"""

import os
import subprocess
import threading
import time
from pathlib import Path
from typing import Dict, List, Optional

class CoreManager:
    def __init__(self):
        self.processes = {
            'web_server': {'name': 'Web Server', 'command': ['python3', 'web_server.py']},
            'agent_core': {'name': 'Agent Core', 'command': ['python3', 'agent_core.py']},
            'rag_service': {'name': 'RAG Service', 'command': ['python3', 'rag_service.py']},
            'memory_service': {'name': 'Memory Service', 'command': ['python3', 'memory_service.py']}
        }
        self.log_file = Path('/home/art/agent-RegArt-1/logs/core_manager.log')
        self.log_file.parent.mkdir(exist_ok=True)
        self.status = {}
        self.monitoring_thread = None
        self.is_running = False

    def log(self, message: str, level: str = 'INFO'):
        """Запись в лог с временной меткой"""
        timestamp = time.strftime("%Y-%m-%d %H:%M:%S")
        log_entry = f"[{timestamp}] {level}: {message}\n"
        with open(self.log_file, 'a') as f:
            f.write(log_entry)

    def start_process(self, process_name: str) -> bool:
        """Запуск процесса"""
        if process_name not in self.processes:
            self.log(f"Процесс {process_name} не найден", "ERROR")
            return False

        proc_info = self.processes[process_name]
        try:
            # Запуск процесса в фоне с перенаправлением вывода
            process = subprocess.Popen(
                proc_info['command'],
                stdout=subprocess.PIPE,
                stderr=subprocess.STDOUT,
                universal_newlines=True,
                cwd='/home/art/agent-RegArt-1'
            )
            
            # Сохранение процесса
            self.status[process_name] = {
                'pid': process.pid,
                'status': 'running',
                'process': process
            }
            
            self.log(f"Процесс {proc_info['name']} (PID: {process.pid}) запущен")
            return True
        except Exception as e:
            self.log(f"Ошибка при запуске {proc_info['name']}: {str(e)}", "ERROR")
            return False

    def stop_process(self, process_name: str) -> bool:
        """Остановка процесса"""
        if process_name not in self.status:
            self.log(f"Процесс {process_name} не запущен", "WARNING")
            return False

        proc_info = self.status[process_name]
        try:
            proc_info['process'].terminate()
            proc_info['status'] = 'stopped'
            self.log(f"Процесс {proc_info['name']} остановлен (PID: {proc_info['pid']})")
            return True
        except Exception as e:
            self.log(f"Ошибка при остановке {proc_info['name']}: {str(e)}", "ERROR")
            return False

    def check_status(self) -> Dict[str, str]:
        """Проверка статуса всех процессов"""
        status_report = {}
        for name, info in self.status.items():
            if info['process'].poll() is None:
                status_report[name] = 'running'
            else:
                status_report[name] = 'stopped'
        return status_report

    def start_monitoring(self):
        """Запуск мониторинга процессов"""
        self.is_running = True
        self.monitoring_thread = threading.Thread(target=self._monitor_loop, daemon=True)
        self.monitoring_thread.start()

    def _monitor_loop(self):
        """Цикл мониторинга"""
        while self.is_running:
            # Проверяем статус всех процессов каждые 5 секунд
            current_status = self.check_status()
            for name, status in current_status.items():
                if status == 'stopped' and self.status[name]['status'] == 'running':
                    self.log(f"Процесс {name} завершился аварийно, попытка перезапуска...")
                    # Попытка перезапуска
                    self.start_process(name)
            time.sleep(5)

    def stop(self):
        """Остановка менеджера"""
        self.is_running = False
        # Останавливаем все процессы
        for name in list(self.status.keys()):
            self.stop_process(name)
        self.log("Менеджер остановлен")

    def get_log_content(self) -> str:
        """Получение содержимого лога"""
        if self.log_file.exists():
            return self.log_file.read_text(encoding='utf-8')
        return "Лог файл не существует"