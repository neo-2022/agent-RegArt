#!/usr/bin/env python3
import os
import subprocess
import sys
import threading
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

class AgentTray:
    def __init__(self):
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
        self.indicator.set_menu(self.create_menu())
        self.update_status_thread()

    def create_menu(self):
        menu = Gtk.Menu()

        # Пункт "Статус"
        item_status = Gtk.MenuItem(label="Показать статус")
        item_status.connect("activate", self.show_status)
        menu.append(item_status)

        # Пункт "Перезапустить все сервисы"
        item_restart = Gtk.MenuItem(label="Перезапустить все")
        item_restart.connect("activate", self.restart_all)
        menu.append(item_restart)

        # Разделитель
        menu.append(Gtk.SeparatorMenuItem())

        # Пункт "Выход"
        item_quit = Gtk.MenuItem(label="Выход")
        item_quit.connect("activate", self.quit)
        menu.append(item_quit)

        menu.show_all()
        return menu

    def show_status(self, _):
        """Показывает статус всех сервисов через уведомление"""
        services = ["agent-tools", "agent-agent", "agent-gateway"]
        status_lines = []
        for srv in services:
            result = subprocess.run(
                ["systemctl", "is-active", srv],
                capture_output=True, text=True
            )
            status = result.stdout.strip() or "inactive"
            status_lines.append(f"{srv}: {status}")

        # Проверка Docker-контейнера
        docker_result = subprocess.run(
            ["docker", "ps", "--filter", "name=agent-memory", "--format", "{{.Status}}"],
            capture_output=True, text=True
        )
        docker_status = docker_result.stdout.strip() or "not running"
        status_lines.append(f"agent-memory: {docker_status}")

        message = "\n".join(status_lines)
        subprocess.run(["notify-send", "Agent Core NG", message])

    def restart_all(self, _):
        """Перезапускает все сервисы (требует sudo)"""
        # Для Docker-контейнера
        subprocess.run(["docker", "restart", "agent-memory"], capture_output=True)

        # Для systemd-сервисов
        for srv in ["agent-tools", "agent-agent", "agent-gateway"]:
            subprocess.run(["sudo", "systemctl", "restart", srv], capture_output=True)

        self.show_status(None)  # показать обновлённый статус

    def quit(self, _):
        Gtk.main_quit()

    def update_status_thread(self):
        """Периодически обновляет иконку (заглушка)"""
        GLib.timeout_add_seconds(60, self.update_status_thread)

def main():
    if not HAVE_TRAY:
        return
    Gtk.init([])
    app = AgentTray()
    Gtk.main()

if __name__ == "__main__":
    main()
