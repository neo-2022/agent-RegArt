// Пакет input — модуль ввода и управления окнами браузера.
//
// Реализует все возможности взаимодействия с GUI из документации:
// - Chrome DevTools Protocol: Input.dispatchKeyEvent, Input.dispatchMouseEvent,
//   Input.insertText, Input.setIgnoreInputEvents
// - Firefox WebExtensions: tabs API (create, remove, update, query, move, reload),
//   windows API (create, remove, update, getAll, getCurrent)
// - Linux X11 инструменты:
//   - xdotool: симуляция клавиатуры (key, type), мыши (mousemove, click, scroll),
//     управление окнами (activate, focus, minimize, maximize, close, resize, move)
//   - wmctrl: управление окнами (activate, close, move, resize, maximize, fullscreen,
//     shade, sticky, список окон)
//   - xclip/xsel: операции с буфером обмена (копирование, вставка, очистка)
//   - xdpyinfo: информация о дисплее (разрешение, глубина цвета)
//
// Все функции работают через системные команды (xdotool, wmctrl, xclip),
// что позволяет управлять любым GUI-приложением, не только браузером.
//
// Требования:
// - Установленные пакеты: xdotool, wmctrl, xclip (или xsel)
// - Работающий X11 или Xwayland сервер (переменная DISPLAY)
package input

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// InputResult — структура результата операции ввода/управления окном.
type InputResult struct {
	Success bool   `json:"success"`          // Успех операции
	Data    string `json:"data,omitempty"`    // Данные результата
	Error   string `json:"error,omitempty"`   // Описание ошибки (на русском)
	Action  string `json:"action,omitempty"`  // Выполненное действие
}

// ============================================================================
// 1. Клавиатурный ввод (xdotool — эквивалент CDP Input.dispatchKeyEvent)
// ============================================================================

// KeyPress — нажимает клавишу или комбинацию клавиш.
// Эквивалент Chrome DevTools Protocol Input.dispatchKeyEvent (keyDown + keyUp).
// Эквивалент Firefox WebExtensions API не имеет прямого аналога (только через content scripts).
//
// Параметры:
//   - keys: клавиша или комбинация (например: "Return", "ctrl+c", "alt+F4", "super+l")
//   - windowID: ID окна для фокуса (0 = текущее активное окно)
//
// Поддерживаемые модификаторы: ctrl, alt, shift, super (Win key)
// Поддерживаемые специальные клавиши: Return, Tab, Escape, BackSpace, Delete,
// Home, End, Page_Up, Page_Down, Up, Down, Left, Right, F1-F12, space
//
// Примеры:
//   - KeyPress("Return", 0) — нажать Enter
//   - KeyPress("ctrl+s", 0) — сохранить (Ctrl+S)
//   - KeyPress("ctrl+shift+t", 0) — восстановить вкладку
func KeyPress(keys string, windowID int) InputResult {
	args := []string{}
	if windowID > 0 {
		args = append(args, "--window", strconv.Itoa(windowID))
	}
	args = append(args, "key", "--clearmodifiers", keys)

	cmd := exec.Command("xdotool", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return InputResult{
			Success: false,
			Error:   fmt.Sprintf("Ошибка нажатия клавиши '%s': %v (%s)", keys, err, string(output)),
			Action:  "key_press",
		}
	}

	return InputResult{
		Success: true,
		Data:    fmt.Sprintf("Нажата клавиша: %s", keys),
		Action:  "key_press",
	}
}

// TypeText — вводит текст посимвольно, имитируя набор на клавиатуре.
// Эквивалент Chrome DevTools Protocol Input.insertText().
// Поддерживает Unicode (русский, китайский и т.д.) через xdotool type.
//
// Параметры:
//   - text: текст для ввода
//   - windowID: ID окна для фокуса (0 = текущее активное)
//   - delay: задержка между символами в миллисекундах (0 = без задержки)
//
// Примеры:
//   - TypeText("Привет мир", 0, 50) — набирает текст с задержкой 50мс
//   - TypeText("test@mail.ru", 0, 0) — быстрый ввод email
func TypeText(text string, windowID int, delay int) InputResult {
	args := []string{}
	if windowID > 0 {
		args = append(args, "--window", strconv.Itoa(windowID))
	}
	if delay > 0 {
		args = append(args, "--delay", strconv.Itoa(delay))
	}
	args = append(args, "type", "--clearmodifiers", text)

	cmd := exec.Command("xdotool", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return InputResult{
			Success: false,
			Error:   fmt.Sprintf("Ошибка ввода текста: %v (%s)", err, string(output)),
			Action:  "type_text",
		}
	}

	return InputResult{
		Success: true,
		Data:    fmt.Sprintf("Введён текст: %s", text),
		Action:  "type_text",
	}
}

// ============================================================================
// 2. Мышь (xdotool — эквивалент CDP Input.dispatchMouseEvent)
// ============================================================================

// MouseClick — кликает мышью в указанных координатах.
// Эквивалент Chrome DevTools Protocol Input.dispatchMouseEvent (mousePressed + mouseReleased).
//
// Параметры:
//   - x, y: координаты клика (в пикселях от верхнего левого угла экрана)
//   - button: кнопка мыши (1=левая, 2=средняя, 3=правая)
//   - clicks: количество кликов (1=одинарный, 2=двойной)
//
// Примеры:
//   - MouseClick(500, 300, 1, 1) — одинарный левый клик
//   - MouseClick(500, 300, 1, 2) — двойной клик
//   - MouseClick(500, 300, 3, 1) — правый клик (контекстное меню)
func MouseClick(x, y, button, clicks int) InputResult {
	// Сначала перемещаем курсор
	moveCmd := exec.Command("xdotool", "mousemove", strconv.Itoa(x), strconv.Itoa(y))
	if err := moveCmd.Run(); err != nil {
		return InputResult{
			Success: false,
			Error:   fmt.Sprintf("Ошибка перемещения мыши в (%d, %d): %v", x, y, err),
			Action:  "mouse_click",
		}
	}

	// Затем кликаем
	args := []string{
		"click",
		"--repeat", strconv.Itoa(clicks),
		strconv.Itoa(button),
	}
	clickCmd := exec.Command("xdotool", args...)
	output, err := clickCmd.CombinedOutput()
	if err != nil {
		return InputResult{
			Success: false,
			Error:   fmt.Sprintf("Ошибка клика мышью: %v (%s)", err, string(output)),
			Action:  "mouse_click",
		}
	}

	buttonName := map[int]string{1: "левая", 2: "средняя", 3: "правая"}[button]
	return InputResult{
		Success: true,
		Data:    fmt.Sprintf("Клик мышью: %s кнопка в (%d, %d), кликов: %d", buttonName, x, y, clicks),
		Action:  "mouse_click",
	}
}

// MouseMove — перемещает курсор мыши в указанные координаты.
// Эквивалент CDP Input.dispatchMouseEvent с type="mouseMoved".
//
// Параметры:
//   - x, y: целевые координаты
func MouseMove(x, y int) InputResult {
	cmd := exec.Command("xdotool", "mousemove", strconv.Itoa(x), strconv.Itoa(y))
	if err := cmd.Run(); err != nil {
		return InputResult{
			Success: false,
			Error:   fmt.Sprintf("Ошибка перемещения мыши: %v", err),
			Action:  "mouse_move",
		}
	}
	return InputResult{
		Success: true,
		Data:    fmt.Sprintf("Курсор перемещён в (%d, %d)", x, y),
		Action:  "mouse_move",
	}
}

// MouseScroll — прокручивает колесо мыши вверх или вниз.
// Эквивалент CDP Input.dispatchMouseEvent с type="mouseWheel".
//
// Параметры:
//   - direction: "up" (вверх) или "down" (вниз)
//   - clicks: количество шагов прокрутки (1 шаг ≈ 3 строки)
func MouseScroll(direction string, clicks int) InputResult {
	button := "5" // вниз
	if direction == "up" {
		button = "4" // вверх
	}

	cmd := exec.Command("xdotool", "click", "--repeat", strconv.Itoa(clicks), button)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return InputResult{
			Success: false,
			Error:   fmt.Sprintf("Ошибка прокрутки: %v (%s)", err, string(output)),
			Action:  "mouse_scroll",
		}
	}

	return InputResult{
		Success: true,
		Data:    fmt.Sprintf("Прокрутка %s на %d шагов", direction, clicks),
		Action:  "mouse_scroll",
	}
}

// MouseDrag — перетаскивает элемент из одной точки в другую (drag & drop).
// Эквивалент CDP: последовательность mousePressed + mouseMoved + mouseReleased.
//
// Параметры:
//   - fromX, fromY: начальные координаты
//   - toX, toY: конечные координаты
func MouseDrag(fromX, fromY, toX, toY int) InputResult {
	// xdotool mousemove + mousedown + mousemove + mouseup
	cmd := exec.Command("xdotool",
		"mousemove", strconv.Itoa(fromX), strconv.Itoa(fromY),
		"mousedown", "1",
		"mousemove", strconv.Itoa(toX), strconv.Itoa(toY),
		"mouseup", "1",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return InputResult{
			Success: false,
			Error:   fmt.Sprintf("Ошибка drag&drop: %v (%s)", err, string(output)),
			Action:  "mouse_drag",
		}
	}

	return InputResult{
		Success: true,
		Data:    fmt.Sprintf("Drag&Drop: (%d,%d) → (%d,%d)", fromX, fromY, toX, toY),
		Action:  "mouse_drag",
	}
}

// ============================================================================
// 3. Управление вкладками (xdotool hotkeys — эквивалент browser.tabs API)
// ============================================================================

// TabAction — выполняет действие с вкладками браузера через горячие клавиши.
// Эквивалент Firefox/Chrome WebExtensions: browser.tabs.create(), browser.tabs.remove(),
// browser.tabs.update(), browser.tabs.move().
//
// Поддерживаемые действия:
//   - "new" — новая вкладка (Ctrl+T) = browser.tabs.create({})
//   - "close" — закрыть текущую (Ctrl+W) = browser.tabs.remove(tabId)
//   - "next" — следующая вкладка (Ctrl+Tab)
//   - "prev" — предыдущая вкладка (Ctrl+Shift+Tab)
//   - "reopen" — восстановить закрытую (Ctrl+Shift+T)
//   - "goto" — перейти к вкладке N (Ctrl+1..9), param = номер 1-9
//   - "duplicate" — дублировать вкладку (нет стандартной горячей клавиши)
//   - "pin" — закрепить/открепить вкладку (нет стандартной горячей клавиши)
//   - "mute" — включить/выключить звук вкладки (Ctrl+M в Chrome)
//   - "reload" — перезагрузить (F5) = browser.tabs.reload(tabId)
//   - "hard_reload" — жёсткая перезагрузка (Ctrl+Shift+R)
//
// Параметры:
//   - action: действие (см. выше)
//   - param: дополнительный параметр (номер вкладки для "goto")
func TabAction(action, param string) InputResult {
	var keys string

	switch action {
	case "new":
		keys = "ctrl+t"
	case "close":
		keys = "ctrl+w"
	case "next":
		keys = "ctrl+Tab"
	case "prev":
		keys = "ctrl+shift+Tab"
	case "reopen":
		keys = "ctrl+shift+t"
	case "goto":
		if param == "" {
			return InputResult{Success: false, Error: "Укажите номер вкладки (1-9)", Action: "tab"}
		}
		n, err := strconv.Atoi(param)
		if err != nil || n < 1 || n > 9 {
			return InputResult{Success: false, Error: "Номер вкладки должен быть от 1 до 9", Action: "tab"}
		}
		keys = fmt.Sprintf("ctrl+%d", n)
	case "reload":
		keys = "F5"
	case "hard_reload":
		keys = "ctrl+shift+r"
	case "mute":
		keys = "ctrl+m"
	default:
		return InputResult{
			Success: false,
			Error:   fmt.Sprintf("Неизвестное действие с вкладкой: '%s'. Доступны: new, close, next, prev, reopen, goto, reload, hard_reload, mute", action),
			Action:  "tab",
		}
	}

	return KeyPress(keys, 0)
}

// ============================================================================
// 4. Управление окнами (wmctrl/xdotool — эквивалент browser.windows API)
// ============================================================================

// WindowAction — выполняет действие с окном приложения.
// Эквивалент Firefox/Chrome WebExtensions: browser.windows.create(), browser.windows.remove(),
// browser.windows.update(), browser.windows.getAll().
//
// Реализация через wmctrl и xdotool (Linux X11/Xwayland).
//
// Поддерживаемые действия:
//   - "list" — список всех открытых окон (wmctrl -l)
//   - "activate" — активировать окно (wmctrl -ia <id>) = browser.windows.update(id, {focused:true})
//   - "close" — закрыть окно (wmctrl -ic <id>) = browser.windows.remove(id)
//   - "minimize" — свернуть (xdotool windowminimize) = browser.windows.update(id, {state:"minimized"})
//   - "maximize" — развернуть (wmctrl -ir <id> -b add,maximized_vert,maximized_horz) = {state:"maximized"}
//   - "unmaximize" — восстановить из максимизации
//   - "fullscreen" — полноэкранный режим (wmctrl -ir <id> -b add,fullscreen) = {state:"fullscreen"}
//   - "unfullscreen" — выйти из полноэкранного
//   - "move" — переместить окно (wmctrl -ir <id> -e 0,x,y,w,h) = browser.windows.update(id, {left,top})
//   - "resize" — изменить размер окна
//   - "focus" — установить фокус (xdotool windowfocus)
//   - "raise" — поднять окно поверх других (xdotool windowraise)
//   - "sticky" — сделать окно видимым на всех рабочих столах
//
// Параметры:
//   - action: действие (см. выше)
//   - target: ID окна или имя окна (для activate/close/move/resize)
//   - params: дополнительные параметры (x,y,w,h для move/resize)
func WindowAction(action, target, params string) InputResult {
	switch action {
	case "list":
		cmd := exec.Command("wmctrl", "-l", "-p")
		output, err := cmd.CombinedOutput()
		if err != nil {
			// Пробуем xdotool если wmctrl не установлен
			cmd2 := exec.Command("xdotool", "search", "--name", "")
			output2, err2 := cmd2.CombinedOutput()
			if err2 != nil {
				return InputResult{
					Success: false,
					Error:   fmt.Sprintf("wmctrl и xdotool не смогли получить список окон: %v", err),
					Action:  "window_list",
				}
			}
			return InputResult{Success: true, Data: string(output2), Action: "window_list"}
		}
		return InputResult{Success: true, Data: string(output), Action: "window_list"}

	case "activate":
		if target == "" {
			return InputResult{Success: false, Error: "Укажите ID окна для активации", Action: "window_activate"}
		}
		cmd := exec.Command("wmctrl", "-ia", target)
		if output, err := cmd.CombinedOutput(); err != nil {
			return InputResult{Success: false, Error: fmt.Sprintf("Ошибка активации окна: %v (%s)", err, string(output)), Action: "window_activate"}
		}
		return InputResult{Success: true, Data: fmt.Sprintf("Окно %s активировано", target), Action: "window_activate"}

	case "close":
		if target == "" {
			return InputResult{Success: false, Error: "Укажите ID окна для закрытия", Action: "window_close"}
		}
		cmd := exec.Command("wmctrl", "-ic", target)
		if output, err := cmd.CombinedOutput(); err != nil {
			return InputResult{Success: false, Error: fmt.Sprintf("Ошибка закрытия окна: %v (%s)", err, string(output)), Action: "window_close"}
		}
		return InputResult{Success: true, Data: fmt.Sprintf("Окно %s закрыто", target), Action: "window_close"}

	case "minimize":
		wid := target
		if wid == "" {
			out, err := exec.Command("xdotool", "getactivewindow").Output()
			if err != nil {
				return InputResult{Success: false, Error: "Не удалось определить активное окно", Action: "window_minimize"}
			}
			wid = strings.TrimSpace(string(out))
		}
		cmd := exec.Command("xdotool", "windowminimize", wid)
		if output, err := cmd.CombinedOutput(); err != nil {
			return InputResult{Success: false, Error: fmt.Sprintf("Ошибка сворачивания: %v (%s)", err, string(output)), Action: "window_minimize"}
		}
		return InputResult{Success: true, Data: "Окно свёрнуто", Action: "window_minimize"}

	case "maximize":
		if target == "" {
			return InputResult{Success: false, Error: "Укажите ID окна для максимизации", Action: "window_maximize"}
		}
		cmd := exec.Command("wmctrl", "-ir", target, "-b", "add,maximized_vert,maximized_horz")
		if output, err := cmd.CombinedOutput(); err != nil {
			return InputResult{Success: false, Error: fmt.Sprintf("Ошибка максимизации: %v (%s)", err, string(output)), Action: "window_maximize"}
		}
		return InputResult{Success: true, Data: "Окно развёрнуто на весь экран", Action: "window_maximize"}

	case "unmaximize":
		if target == "" {
			return InputResult{Success: false, Error: "Укажите ID окна", Action: "window_unmaximize"}
		}
		cmd := exec.Command("wmctrl", "-ir", target, "-b", "remove,maximized_vert,maximized_horz")
		if output, err := cmd.CombinedOutput(); err != nil {
			return InputResult{Success: false, Error: fmt.Sprintf("Ошибка: %v (%s)", err, string(output)), Action: "window_unmaximize"}
		}
		return InputResult{Success: true, Data: "Максимизация снята", Action: "window_unmaximize"}

	case "fullscreen":
		if target == "" {
			return InputResult{Success: false, Error: "Укажите ID окна", Action: "window_fullscreen"}
		}
		cmd := exec.Command("wmctrl", "-ir", target, "-b", "add,fullscreen")
		if output, err := cmd.CombinedOutput(); err != nil {
			return InputResult{Success: false, Error: fmt.Sprintf("Ошибка: %v (%s)", err, string(output)), Action: "window_fullscreen"}
		}
		return InputResult{Success: true, Data: "Полноэкранный режим включён", Action: "window_fullscreen"}

	case "unfullscreen":
		if target == "" {
			return InputResult{Success: false, Error: "Укажите ID окна", Action: "window_unfullscreen"}
		}
		cmd := exec.Command("wmctrl", "-ir", target, "-b", "remove,fullscreen")
		if output, err := cmd.CombinedOutput(); err != nil {
			return InputResult{Success: false, Error: fmt.Sprintf("Ошибка: %v (%s)", err, string(output)), Action: "window_unfullscreen"}
		}
		return InputResult{Success: true, Data: "Полноэкранный режим выключен", Action: "window_unfullscreen"}

	case "move":
		if target == "" || params == "" {
			return InputResult{Success: false, Error: "Укажите ID окна и координаты (x,y,w,h)", Action: "window_move"}
		}
		cmd := exec.Command("wmctrl", "-ir", target, "-e", "0,"+params)
		if output, err := cmd.CombinedOutput(); err != nil {
			return InputResult{Success: false, Error: fmt.Sprintf("Ошибка перемещения: %v (%s)", err, string(output)), Action: "window_move"}
		}
		return InputResult{Success: true, Data: fmt.Sprintf("Окно перемещено: %s", params), Action: "window_move"}

	case "resize":
		if target == "" || params == "" {
			return InputResult{Success: false, Error: "Укажите ID окна и размеры (x,y,w,h)", Action: "window_resize"}
		}
		cmd := exec.Command("wmctrl", "-ir", target, "-e", "0,"+params)
		if output, err := cmd.CombinedOutput(); err != nil {
			return InputResult{Success: false, Error: fmt.Sprintf("Ошибка изменения размера: %v (%s)", err, string(output)), Action: "window_resize"}
		}
		return InputResult{Success: true, Data: fmt.Sprintf("Размер окна изменён: %s", params), Action: "window_resize"}

	case "focus":
		wid := target
		if wid == "" {
			return InputResult{Success: false, Error: "Укажите ID окна", Action: "window_focus"}
		}
		cmd := exec.Command("xdotool", "windowfocus", wid)
		if output, err := cmd.CombinedOutput(); err != nil {
			return InputResult{Success: false, Error: fmt.Sprintf("Ошибка фокуса: %v (%s)", err, string(output)), Action: "window_focus"}
		}
		return InputResult{Success: true, Data: "Фокус установлен", Action: "window_focus"}

	case "raise":
		wid := target
		if wid == "" {
			return InputResult{Success: false, Error: "Укажите ID окна", Action: "window_raise"}
		}
		cmd := exec.Command("xdotool", "windowraise", wid)
		if output, err := cmd.CombinedOutput(); err != nil {
			return InputResult{Success: false, Error: fmt.Sprintf("Ошибка: %v (%s)", err, string(output)), Action: "window_raise"}
		}
		return InputResult{Success: true, Data: "Окно поднято поверх других", Action: "window_raise"}

	case "sticky":
		if target == "" {
			return InputResult{Success: false, Error: "Укажите ID окна", Action: "window_sticky"}
		}
		cmd := exec.Command("wmctrl", "-ir", target, "-b", "add,sticky")
		if output, err := cmd.CombinedOutput(); err != nil {
			return InputResult{Success: false, Error: fmt.Sprintf("Ошибка: %v (%s)", err, string(output)), Action: "window_sticky"}
		}
		return InputResult{Success: true, Data: "Окно закреплено на всех рабочих столах", Action: "window_sticky"}

	default:
		return InputResult{
			Success: false,
			Error:   fmt.Sprintf("Неизвестное действие с окном: '%s'. Доступны: list, activate, close, minimize, maximize, unmaximize, fullscreen, unfullscreen, move, resize, focus, raise, sticky", action),
			Action:  "window",
		}
	}
}

// ============================================================================
// 5. Буфер обмена (xclip/xsel — аналог Clipboard API)
// ============================================================================

// ClipboardAction — операции с системным буфером обмена.
// Эквивалент Web Clipboard API: navigator.clipboard.writeText(), navigator.clipboard.readText()
// Эквивалент Firefox WebExtensions: clipboard.setImageData()
//
// Реализация через xclip (приоритет) или xsel (fallback).
//
// Поддерживаемые действия:
//   - "copy" — копировать текст в буфер обмена
//   - "paste" — получить текст из буфера обмена
//   - "clear" — очистить буфер обмена
//
// Параметры:
//   - action: действие (copy, paste, clear)
//   - text: текст для копирования (только для "copy")
func ClipboardAction(action, text string) InputResult {
	clipTool := "xclip"
	if _, err := exec.LookPath("xclip"); err != nil {
		if _, err := exec.LookPath("xsel"); err != nil {
			return InputResult{
				Success: false,
				Error:   "Не найден xclip или xsel. Установите: sudo apt install xclip",
				Action:  "clipboard",
			}
		}
		clipTool = "xsel"
	}

	switch action {
	case "copy":
		if text == "" {
			return InputResult{Success: false, Error: "Текст для копирования не может быть пустым", Action: "clipboard_copy"}
		}
		var cmd *exec.Cmd
		if clipTool == "xclip" {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		}
		cmd.Stdin = strings.NewReader(text)
		if output, err := cmd.CombinedOutput(); err != nil {
			return InputResult{Success: false, Error: fmt.Sprintf("Ошибка копирования: %v (%s)", err, string(output)), Action: "clipboard_copy"}
		}
		return InputResult{Success: true, Data: "Текст скопирован в буфер обмена", Action: "clipboard_copy"}

	case "paste":
		var cmd *exec.Cmd
		if clipTool == "xclip" {
			cmd = exec.Command("xclip", "-selection", "clipboard", "-o")
		} else {
			cmd = exec.Command("xsel", "--clipboard", "--output")
		}
		output, err := cmd.Output()
		if err != nil {
			return InputResult{Success: false, Error: fmt.Sprintf("Ошибка получения из буфера: %v", err), Action: "clipboard_paste"}
		}
		return InputResult{Success: true, Data: string(output), Action: "clipboard_paste"}

	case "clear":
		var cmd *exec.Cmd
		if clipTool == "xclip" {
			cmd = exec.Command("xclip", "-selection", "clipboard")
			cmd.Stdin = strings.NewReader("")
		} else {
			cmd = exec.Command("xsel", "--clipboard", "--clear")
		}
		if err := cmd.Run(); err != nil {
			return InputResult{Success: false, Error: fmt.Sprintf("Ошибка очистки буфера: %v", err), Action: "clipboard_clear"}
		}
		return InputResult{Success: true, Data: "Буфер обмена очищен", Action: "clipboard_clear"}

	default:
		return InputResult{
			Success: false,
			Error:   fmt.Sprintf("Неизвестное действие с буфером: '%s'. Доступны: copy, paste, clear", action),
			Action:  "clipboard",
		}
	}
}

// ============================================================================
// 6. Масштабирование (Zoom), DevTools, поиск текста
// ============================================================================

// ZoomAction — управление масштабом страницы через горячие клавиши.
// Эквивалент Chrome DevTools Protocol: Emulation.setPageScaleFactor()
// Эквивалент Firefox WebExtensions: tabs.setZoom(), tabs.getZoom()
//
// Параметры:
//   - action: "in" (увеличить, Ctrl++), "out" (уменьшить, Ctrl+-), "reset" (сбросить, Ctrl+0)
func ZoomAction(action string) InputResult {
	var keys string
	switch action {
	case "in":
		keys = "ctrl+plus"
	case "out":
		keys = "ctrl+minus"
	case "reset":
		keys = "ctrl+0"
	default:
		return InputResult{Success: false, Error: fmt.Sprintf("Неизвестное действие масштаба: '%s'. Доступны: in, out, reset", action), Action: "zoom"}
	}
	return KeyPress(keys, 0)
}

// ToggleDevTools — открывает/закрывает DevTools (инструменты разработчика).
// Эквивалент Chrome DevTools Protocol: подключение через WebSocket.
// Горячая клавиша: F12 (работает во всех Chromium-based браузерах и Firefox).
func ToggleDevTools() InputResult {
	return KeyPress("F12", 0)
}

// FindText — открывает поиск текста на странице и вводит запрос.
// Эквивалент Chrome DevTools Protocol: DOM.performSearch(query)
// Эквивалент Firefox WebExtensions: find.find(query)
//
// Параметры:
//   - text: текст для поиска
func FindText(text string) InputResult {
	// Открываем поиск (Ctrl+F)
	result := KeyPress("ctrl+f", 0)
	if !result.Success {
		return result
	}
	// Вводим текст поиска
	return TypeText(text, 0, 0)
}

// ============================================================================
// 7. Получение активного окна и позиции курсора
// ============================================================================

// GetActiveWindow — получает информацию о текущем активном окне.
// Использует xdotool getactivewindow + getwindowname + getwindowgeometry.
func GetActiveWindow() InputResult {
	widCmd := exec.Command("xdotool", "getactivewindow")
	widOutput, err := widCmd.Output()
	if err != nil {
		return InputResult{Success: false, Error: fmt.Sprintf("Ошибка получения активного окна: %v", err), Action: "get_active_window"}
	}
	wid := strings.TrimSpace(string(widOutput))

	nameCmd := exec.Command("xdotool", "getwindowname", wid)
	nameOutput, _ := nameCmd.Output()

	geoCmd := exec.Command("xdotool", "getwindowgeometry", wid)
	geoOutput, _ := geoCmd.Output()

	info := fmt.Sprintf("ID: %s\nИмя: %s\n%s", wid, strings.TrimSpace(string(nameOutput)), strings.TrimSpace(string(geoOutput)))
	return InputResult{Success: true, Data: info, Action: "get_active_window"}
}

// GetMouseLocation — получает текущие координаты курсора мыши.
// Использует xdotool getmouselocation.
func GetMouseLocation() InputResult {
	cmd := exec.Command("xdotool", "getmouselocation")
	output, err := cmd.Output()
	if err != nil {
		return InputResult{Success: false, Error: fmt.Sprintf("Ошибка: %v", err), Action: "get_mouse_location"}
	}
	return InputResult{Success: true, Data: strings.TrimSpace(string(output)), Action: "get_mouse_location"}
}

// GetScreenResolution — получает разрешение экрана через xdpyinfo.
func GetScreenResolution() InputResult {
	cmd := exec.Command("xdpyinfo")
	output, err := cmd.Output()
	if err != nil {
		return InputResult{Success: false, Error: fmt.Sprintf("Ошибка: %v", err), Action: "screen_resolution"}
	}
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "dimensions:") {
			return InputResult{Success: true, Data: strings.TrimSpace(line), Action: "screen_resolution"}
		}
	}
	return InputResult{Success: true, Data: "Разрешение не определено", Action: "screen_resolution"}
}
