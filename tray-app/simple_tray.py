#!/usr/bin/env python3
import os
import subprocess
import sys
try:
    import gi
    gi.require_version('Gtk', '3.0')
    gi.require_version('AppIndicator3', '0.1')
    from gi.repository import Gtk, AppIndicator3, GLib
    HAVE_TRAY = True
except ImportError:
    print("Не установлены библиотеки для трея. Установите: sudo apt install gir1.2-appindicator3-0.1 python3-gi")
    sys.exit(1)

class SimpleTray:
    def __init__(self):
        # Путь к иконке
        icon_path = os.path.join(os.path.dirname(__file__), "favicon.png")
        if not os.path.exists(icon_path):
            icon_name = "face-smile"
        else:
            icon_name = icon_path
            
        self.indicator = AppIndicator3.Indicator.new(
            "agent-core-ng-simple",
            icon_name,
            AppIndicator3.IndicatorCategory.APPLICATION_STATUS
        )
        self.indicator.set_status(AppIndicator3.IndicatorStatus.ACTIVE)
        self.indicator.set_title("Agent Core NG")
        self.indicator.set_label("Simple", "Status")
        
        # Создаем меню
        menu = Gtk.Menu()
        item = Gtk.MenuItem(label="Тест")
        item.connect("activate", self.test)
        menu.append(item)
        
        item_quit = Gtk.MenuItem(label="Выход")
        item_quit.connect("activate", self.quit)
        menu.append(item_quit)
        
        menu.show_all()
        self.indicator.set_menu(menu)
        
    def test(self, _):
        print("Тест работает")
        
    def quit(self, _):
        Gtk.main_quit()

def main():
    if not HAVE_TRAY:
        print("Ошибка: Не установлены библиотеки для системного трея")
        return
    
    indicator = SimpleTray()
    Gtk.main()

if __name__ == "__main__":
    main()