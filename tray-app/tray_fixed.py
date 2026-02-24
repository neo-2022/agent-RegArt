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
        self.indicator.set_menu(self.create_menu())
        self.update_status_thread()
        self.indicator.connect('button-press-event', self.on_button_press)

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
            self.create_menu().popup(None, None, None, None, event.button, event.time)
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