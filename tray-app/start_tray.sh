#!/bin/bash
# Определяем тип графической сессии
if [ "$XDG_SESSION_TYPE" == "wayland" ]; then
    echo "Wayland session detected"
    export WAYLAND_DISPLAY="$DISPLAY"
    export QT_QPA_PLATFORM=wayland
    export CLUTTER_BACKEND=wayland
    export SDL_VIDEODRIVER=wayland
    export MOZ_ENABLE_WAYLAND=1
else
    echo "X11 session detected"
    export DISPLAY="${DISPLAY:-:0}"
    export XAUTHORITY="${XAUTHORITY:-$HOME/.Xauthority}"
fi

# Запускаем tray.py с учетом окружения
cd "$(dirname "$0")"
/usr/bin/python3 tray.py "$@"
