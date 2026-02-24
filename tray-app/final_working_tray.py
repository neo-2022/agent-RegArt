#!/usr/bin/env python3
import os
import subprocess
import sys
import socket
import logging
import time
try:
    import gi
    gi.require_version('Gtk', '3.0')
    gi.require_version('AppIndicator3', '0.1')
    from gi.repository import Gtk, AppIndicator3, GLib
    HAVE_TRAY = True
except ImportError:
    print("Не установлены библиотеки для трея. Установите: sudo apt install gir1.2-appindicator3-0.1 python3-gi")
    HAVE_TRAY = False
    sys.exit(1)

# Настройка логирования
log_dir = os.path.expanduser("~/.logs")
os.makedirs(log_dir, exist_ok=True)
log_file = os.path.join(log_dir, "agent-core-ng-final.log")

logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s',
    handlers=[
        logging.FileHandler(log_file),
        logging.StreamHandler()
    ]
)
logger = logging.getLogger("AgentTray")

# Проверка на уже запущенный экземпляр
try:
    lock_socket = socket.socket(socket.AF_UNIX, socket.SOCK_DGRAM)
    lock_socket.bind('\0agent-core-ng-tray-final')
except socket.error:
    print("Приложение уже запущено")
    sys.exit(1)

class FinalTray:
    def __init__(self):
        logger.info("Инициализация FinalTray")
        # Путь к иконке
        self.base_dir = os.path.dirname(__file__)
        self.icon_path = os.path.join(self.base_dir, "favicon_64x64.png")
        logger.info(f"Путь к иконке: {self.icon_path}")
        logger.info(f"Иконка существует: {os.path.exists(self.icon_path)}")
        if not os.path.exists(self.icon_path):
            logger.warning("Иконка не найдена, используем стандартную")
            self.icon_name = "face-smile"
        else:
            logger.info("Используем пользовательскую иконку")
            self.icon_name = self.icon_path
            
        self.indicator = AppIndicator3.Indicator.new(
            "agent-core-ng-final",
            self.icon_name,
            AppIndicator3.IndicatorCategory.APPLICATION_STATUS
        )
        self.indicator.set_status(AppIndicator3.IndicatorStatus.ACTIVE)
        self.indicator.set_title("Agent Core NG")
        self.indicator.set_label("Проверка статуса...", "Status")
        
        # Создаем меню
        menu = Gtk.Menu()
        
        # Пункт "Показать статус"
        item_status = Gtk.MenuItem(label="Показать статус")
        item_status.connect("activate", self.show_status)
        menu.append(item_status)
        
        # Пункт "Открыть веб-интерфейс"
        item_open_ui = Gtk.MenuItem(label="Открыть веб-интерфейс")
        item_open_ui.connect("activate", self.open_web_interface)
        menu.append(item_open_ui)
        
        # Пункт "Открыть логи"
        item_logs = Gtk.MenuItem(label="Открыть логи")
        item_logs.connect("activate", self.open_logs)
        menu.append(item_logs)
        
        # Пункт "Перезапустить все сервисы"
        item_restart = Gtk.MenuItem(label="Перезапустить все сервисы")
        item_restart.connect("activate", self.restart_all)
        menu.append(item_restart)
        
        # Пункт "Выход"
        item_quit = Gtk.MenuItem(label="Выход")
        item_quit.connect("activate", self.quit)
        menu.append(item_quit)
        
        menu.show_all()
        self.indicator.set_menu(menu)
        
        # Запускаем обновление статуса
        self.update_status_thread()
        
    def open_web_interface(self, _):
        """Открывает веб-интерфейс в браузере"""
        logger.info("Попытка открыть веб-интерфейс")
        try:
            # Получаем порт из скрипта
            port_result = subprocess.run(
                ["/home/art/agent-RegArt-1/tray-app/get_vite_port.sh"],
                capture_output=True, text=True, timeout=5
            )
            port = port_result.stdout.strip()
            web_url = f"http://localhost:{port}"
            
            # Пробуем открыть через xdg-open
            subprocess.run(["xdg-open", web_url], check=True)
        except Exception as e:
            logger.error(f"Ошибка открытия веб-интерфейса: {str(e)}")
            dialog = Gtk.MessageDialog(
                parent=None,
                modal=True,
                message_type=Gtk.MessageType.ERROR,
                buttons=Gtk.ButtonsType.OK,
                text=f"Не удалось открыть интерфейс: {str(e)}"
            )
            dialog.run()
            dialog.destroy()

    def show_status(self, _):
        """Показывает статус всех сервисов через уведомление"""
        services = ["agent-tools", "agent-agent", "agent-gateway"]
        status_lines = []
        
        # Проверяем статус сервисов
        for srv in services:
            status = "активен" if self.check_service_status(srv) else "не активен"
            status_lines.append(f"{srv}: {status} ({'🟢' if self.check_service_status(srv) else '🔴'})")
            
        # Проверяем веб-интерфейс
        web_accessible = self.check_web_interface()
        web_status = "доступен" if web_accessible else "недоступен"
        status_lines.append(f"Веб-интерфейс: {web_status} ({'🟢' if web_accessible else '🔴'})")
            
        # Добавляем информацию о ChromaDB
        chroma_result = subprocess.run(
            ["docker", "ps", "-q", "-f", "name=agent-chroma"],
            capture_output=True, text=True
        )
        chroma_status = "запущен" if chroma_result.stdout.strip() else "не запущен"
        status_lines.append(f"ChromaDB: {chroma_status}")
        
        # Добавляем информацию о файлах
        try:
            find_result = subprocess.run(
                ["find", "agent-service/uploads", "-type", "f", "-name", "*.md"],
                capture_output=True, text=True
            ).stdout.strip()
            file_count = len(find_result.split('\n')) if find_result else 0
            status_lines.append(f"Файлов в RAG: {file_count}")
        except:
            status_lines.append("Файлов в RAG: не определено")
            
        # Создаем диалог с информацией
        dialog = Gtk.MessageDialog(
            parent=None,
            modal=True,
            message_type=Gtk.MessageType.INFO,
            buttons=Gtk.ButtonsType.OK,
            text="\n".join(status_lines)
        )
        dialog.set_title("Статус системы")
        dialog.run()
        dialog.destroy()

    def check_service_status(self, service_name):
        """Проверяет статус сервиса через health-чек"""
        port_mapping = {
            "agent-tools": 8082,
            "agent-agent": 8083, 
            "agent-gateway": 8080
        }
        
        if service_name not in port_mapping:
            return False
            
        try:
            port = port_mapping[service_name]
            result = subprocess.run(
                ["curl", "-s", f"http://localhost:{port}/health"],
                capture_output=True, 
                text=True,
                timeout=3
            )
            health_data = result.stdout.strip()
            return '"status":"ok"' in health_data
        except:
            return False

    def check_web_interface(self):
        """Проверяет доступность веб-интерфейса"""
        try:
            # Получаем порт из скрипта
            port_result = subprocess.run(
                ["/home/art/agent-RegArt-1/tray-app/get_vite_port.sh"],
                capture_output=True, text=True, timeout=5
            )
            port = port_result.stdout.strip()
            web_url = f"http://localhost:{port}"
            
            # Проверяем доступность
            result = subprocess.run(
                ["curl", "-s", "-o", "/dev/null", "-w", "%{http_code}", web_url],
                capture_output=True, text=True, timeout=5
            )
            return result.stdout.strip() == "200"
        except Exception as e:
            logger.error(f"Ошибка проверки веб-интерфейса: {str(e)}")
            return False

    def update_tray_icon(self, icon_path):
        """Обновляет иконку в трее"""
        self.indicator.set_icon_full(icon_path, "Status")

    def update_status_thread(self):
        """Обновление статуса в фоне с визуальными индикаторами"""
        def update():
            # Проверяем статус основных сервисов
            services_ok = all(self.check_service_status(srv) for srv in ["agent-tools", "agent-agent", "agent-gateway"])
            
            # Проверяем доступность веб-интерфейса
            web_ok = self.check_web_interface()
            
            # Определяем цвет статуса (используем только favicon.png)
            if services_ok and web_ok:
                logger.info("Все сервисы активны")
            else:
                logger.warning("Обнаружены проблемы с сервисами")
            
            icon_path = self.icon_path if os.path.exists(self.icon_path) else "face-smile"
            self.update_tray_icon(icon_path)
            status_text = "Готов" if (services_ok and web_ok) else "Ошибка"
            self.indicator.set_label(status_text, "Status")
            
            # Обновляем каждые 10 секунд
            return True  # Необходимо для GLib.timeout_add
            
        # Первый запуск проверки
        GLib.timeout_add_seconds(1, update)
        # Последующие обновления каждые 10 секунд
        GLib.timeout_add_seconds(10, update)

    def open_logs(self, _):
        """Открывает файл логов"""
        logger.info("Открытие файла логов")
        try:
            log_file = os.path.expanduser("~/.logs/agent-core-ng-final.log")
            subprocess.run(["xdg-open", log_file], check=True)
        except Exception as e:
            logger.error(f"Ошибка при открытии логов: {str(e)}")
            dialog = Gtk.MessageDialog(
                parent=None,
                modal=True,
                message_type=Gtk.MessageType.ERROR,
                buttons=Gtk.ButtonsType.OK,
                text=f"Не удалось открыть файл логов: {str(e)}"
            )
            dialog.run()
            dialog.destroy()

    def restart_all(self, _):
        """Перезапускает все сервисы"""
        logger.info("Запрошен перезапуск всех сервисов")
        dialog = Gtk.MessageDialog(
            parent=None,
            modal=True,
            message_type=Gtk.MessageType.WARNING,
            buttons=Gtk.ButtonsType.YES_NO,
            text="Вы уверены, что хотите перезапустить все сервисы?"
        )
        response = dialog.run()
        dialog.destroy()
        
        if response == Gtk.ResponseType.YES:
            try:
                # Перезапуск сервисов
                subprocess.run(["sudo", "systemctl", "restart", "agent-tools"], check=True)
                subprocess.run(["sudo", "systemctl", "restart", "agent-agent"], check=True)
                subprocess.run(["sudo", "systemctl", "restart", "agent-gateway"], check=True)
                
                # Ждем немного
                import time
                time.sleep(2)
                
                logger.info("Сервисы успешно перезапущены")
                # Показываем успешное завершение
                dialog_success = Gtk.MessageDialog(
                    parent=None,
                    modal=True,
                    message_type=Gtk.MessageType.INFO,
                    buttons=Gtk.ButtonsType.OK,
                    text="Сервисы успешно перезапущены!"
                )
                dialog_success.run()
                dialog_success.destroy()
                
            except Exception as e:
                logger.error(f"Ошибка при перезапуске сервисов: {str(e)}")
                dialog_error = Gtk.MessageDialog(
                    parent=None,
                    modal=True,
                    message_type=Gtk.MessageType.ERROR,
                    buttons=Gtk.ButtonsType.OK,
                    text=f"Ошибка перезапуска: {str(e)}"
                )
                dialog_error.run()
                dialog_error.destroy()

    def quit(self, _):
        """Завершает работу приложения"""
        logger.info("Завершение работы приложения")
        Gtk.main_quit()

def main():
    if not HAVE_TRAY:
        logger.error("Не установлены библиотеки для системного трея")
        print("Ошибка: Не установлены библиотеки для системного трея")
        return
    
    # Запускаем трей-приложение
    indicator = FinalTray()
    Gtk.main()

if __name__ == "__main__":
    main()