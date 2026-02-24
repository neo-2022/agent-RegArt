#!/usr/bin/env python3
import os
import subprocess
import sys
import threading
import time
import socket
import logging
import logging.handlers
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
log_file = os.path.join(log_dir, "agent-core-ng.log")

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
    lock_socket.bind('\0agent-core-ng-tray')
except socket.error:
    print("Приложение уже запущено")
    sys.exit(1)

class AgentTray:
    def __init__(self):
        logger.info("Инициализация AgentTray")
        # Путь к иконке (предполагается, что она лежит рядом со скриптом)
        icon_path = os.path.join(os.path.dirname(__file__), "favicon.png")
        if not os.path.exists(icon_path):
            print(f"Иконка не найдена по пути: {icon_path}, используется стандартная")
            icon_name = "face-smile"
        else:
            icon_name = icon_path
            
        self.indicator = AppIndicator3.Indicator.new(
            "agent-core-ng",
            icon_name,
            AppIndicator3.IndicatorCategory.APPLICATION_STATUS
        )
        self.indicator.set_status(AppIndicator3.IndicatorStatus.ACTIVE)
        self.indicator.set_title("Agent Core NG")
        self.indicator.set_label("Проверка статуса...", "Status")
        self.menu = self.create_menu()
        self.indicator.set_menu(self.menu)
        self.update_status_thread()
        
        # Создаем прозрачное окно для обработки кликов
        self.window = Gtk.Window()
        self.window.set_default_size(1, 1)
        self.window.set_decorated(False)
        self.window.set_skip_taskbar_hint(True)
        self.window.set_skip_pager_hint(True)
        self.window.set_accept_focus(False)
        
        # Добавляем обработчик кликов
        self.window.connect("button-press-event", self.on_button_press)
        self.window.show_all()

    def create_menu(self):
        menu = Gtk.Menu()
        
        # Пункт "Открыть веб-интерфейс"
        item_open_ui = Gtk.MenuItem(label="Открыть веб-интерфейс")
        item_open_ui.connect("activate", self.open_web_interface)
        menu.append(item_open_ui)
        
        # Пункт "Перезапустить все сервисы"
        item_restart = Gtk.MenuItem(label="Перезапустить все сервисы")
        item_restart.connect("activate", self.restart_all)
        menu.append(item_restart)
        
        # Пункт "Показать статус сервисов"
        item_status = Gtk.MenuItem(label="Показать статус сервисов")
        item_status.connect("activate", self.show_status)
        menu.append(item_status)
        
        # Пункт "Открыть логи"
        item_logs = Gtk.MenuItem(label="Открыть логи")
        item_logs.connect("activate", self.open_logs)
        menu.append(item_logs)
        
        # Пункт "Открыть конфигурацию"
        item_config = Gtk.MenuItem(label="Открыть конфигурацию")
        item_config.connect("activate", self.open_config)
        menu.append(item_config)
        
        # Пункт "Проверить обновления"
        item_updates = Gtk.MenuItem(label="Проверить обновления")
        item_updates.connect("activate", self.check_updates)
        menu.append(item_updates)
        
        # Пункт "О программе"
        item_about = Gtk.MenuItem(label="О программе")
        item_about.connect("activate", self.show_about)
        menu.append(item_about)
        
        # Разделитель
        menu.append(Gtk.SeparatorMenuItem())
        
        # Пункт "Выход"
        item_quit = Gtk.MenuItem(label="Выход")
        item_quit.connect("activate", self.quit)
        menu.append(item_quit)
        
        return menu

    def on_button_press(self, widget, event):
        """Обработчик кликов по иконке"""
        if event.button == 3:  # Правая кнопка мыши
            self.menu.popup(None, None, None, None, event.button, event.time)
            return True
        return False

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
            # Если не получилось, пробуем другие браузеры
            browsers = ["firefox", "google-chrome"]
            success = False
            
            for browser in browsers:
                try:
                    subprocess.run([browser, web_url], check=True)
                    success = True
                    break
                except Exception:
                    continue
            
            if not success:
                logger.error("Не удалось открыть веб-интерфейс ни одним из браузеров")
                # Если ничего не помогло, показываем сообщение один раз
                dialog = Gtk.MessageDialog(
                    None,
                    Gtk.DialogFlags.MODAL,
                    Gtk.MessageType.INFO,
                    Gtk.ButtonsType.OK,
                    f"Не удалось открыть интерфейс. Пожалуйста, откройте вручную по адресу: {web_url}"
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
            None,
            Gtk.DialogFlags.MODAL,
            Gtk.MessageType.INFO,
            Gtk.ButtonsType.OK,
            "\n".join(status_lines)
        )
        dialog.set_title("Статус системы")
        dialog.run()
        dialog.destroy()

    def restart_all(self, _):
        """Перезапускает все сервисы"""
        logger.info("Запрошен перезапуск всех сервисов")
        dialog = Gtk.MessageDialog(
            None,
            Gtk.DialogFlags.MODAL,
            Gtk.MessageType.WARNING,
            Gtk.ButtonsType.YES_NO,
            "Вы уверены, что хотите перезапустить все сервисы?"
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
                time.sleep(2)
                
                logger.info("Сервисы успешно перезапущены")
                # Показываем успешное завершение
                dialog_success = Gtk.MessageDialog(
                    None,
                    Gtk.DialogFlags.MODAL,
                    Gtk.MessageType.INFO,
                    Gtk.ButtonsType.OK,
                    "Сервисы успешно перезапущены!"
                )
                dialog_success.run()
                dialog_success.destroy()
                
            except Exception as e:
                logger.error(f"Ошибка при перезапуске сервисов: {str(e)}")
                dialog_error = Gtk.MessageDialog(
                    None,
                    Gtk.DialogFlags.MODAL,
                    Gtk.MessageType.ERROR,
                    Gtk.ButtonsType.OK,
                    f"Ошибка перезапуска: {str(e)}"
                )
                dialog_error.run()
                dialog_error.destroy()

    def open_logs(self, _):
        """Открывает файл логов"""
        logger.info("Открытие файла логов")
        try:
            log_file = os.path.expanduser("~/.logs/agent-core-ng.log")
            subprocess.run(["xdg-open", log_file], check=True)
        except Exception as e:
            logger.error(f"Ошибка при открытии логов: {str(e)}")
            dialog = Gtk.MessageDialog(
                None,
                Gtk.DialogFlags.MODAL,
                Gtk.MessageType.ERROR,
                Gtk.ButtonsType.OK,
                f"Не удалось открыть файл логов: {str(e)}"
            )
            dialog.run()
            dialog.destroy()

    def open_config(self, _):
        """Открывает директорию с конфигурацией"""
        logger.info("Открытие директории с конфигурацией")
        try:
            config_dir = os.path.expanduser("~/.comate/config")
            subprocess.run(["xdg-open", config_dir], check=True)
        except Exception as e:
            logger.error(f"Ошибка при открытии конфигурации: {str(e)}")
            dialog = Gtk.MessageDialog(
                None,
                Gtk.DialogFlags.MODAL,
                Gtk.MessageType.ERROR,
                Gtk.ButtonsType.OK,
                f"Не удалось открыть конфигурацию: {str(e)}"
            )
            dialog.run()
            dialog.destroy()

    def check_updates(self, _):
        """Проверяет наличие обновлений"""
        logger.info("Проверка обновлений")
        try:
            # Простая проверка обновлений (можно расширить)
            dialog = Gtk.MessageDialog(
                None,
                Gtk.DialogFlags.MODAL,
                Gtk.MessageType.INFO,
                Gtk.ButtonsType.OK,
                "Проверка обновлений...\n\nТекущая версия: 1.0.0\nОбновлений не найдено."
            )
            dialog.run()
            dialog.destroy()
        except Exception as e:
            logger.error(f"Ошибка при проверке обновлений: {str(e)}")

    def show_about(self, _):
        """Показывает информацию о программе"""
        logger.info("Показ информации о программе")
        about_dialog = Gtk.AboutDialog()
        about_dialog.set_program_name("Agent Core NG Tray")
        about_dialog.set_version("1.0.0")
        about_dialog.set_copyright("© 2026 Agent Core NG")
        about_dialog.set_comments("Системный трей для управления Agent Core NG")
        about_dialog.set_website("https://github.com/neo-2022/agent-RegArt")
        about_dialog.set_website_label("GitHub репозиторий")
        about_dialog.run()
        about_dialog.destroy()

    def quit(self, _):
        """Завершает работу приложения"""
        logger.info("Завершение работы приложения")
        Gtk.main_quit()

    def check_service_status(self, service_name):
        """Проверяет статус сервиса"""
        try:
            result = subprocess.run(
                ["systemctl", "is-active", service_name],
                capture_output=True, text=True
            )
            return result.stdout.strip() == "active"
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
        except:
            return False

    def update_status_thread(self):
        """Обновление статуса в фоне с визуальными индикаторами"""
        def update():
            # Проверяем статус основных сервисов
            services_ok = all(self.check_service_status(srv) for srv in ["agent-tools", "agent-agent", "agent-gateway"])
            
            # Проверяем доступность веб-интерфейса
            web_ok = self.check_web_interface()
            
            # Определяем путь к иконке статуса
            base_dir = os.path.dirname(__file__)
            status_icon = "status-green.png" if (services_ok and web_ok) else "status-red.png"
            icon_path = os.path.join(base_dir, status_icon)
            
            # Если иконка статуса не существует, используем основную иконку
            if not os.path.exists(icon_path):
                icon_path = os.path.join(base_dir, "favicon.png") if os.path.exists(os.path.join(base_dir, "favicon.png")) else "face-smile"
            
            self.indicator.set_icon_full(icon_path, "Status")
            
            # Обновляем каждые 10 секунд
            GLib.timeout_add_seconds(10, update)
        
        # Первый запуск проверки
        GLib.timeout_add_seconds(1, update)

def main():
    if not HAVE_TRAY:
        logger.error("Не установлены библиотеки для системного трея")
        print("Ошибка: Не установлены библиотеки для системного трея")
        return
    
    # Запускаем трей-приложение
    indicator = AgentTray()
    Gtk.main()

if __name__ == "__main__":
    main()