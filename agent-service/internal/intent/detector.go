package intent

import (
	"regexp"
	"strings"
)

// Типы интентов
const (
	IntentNone           = ""
	IntentRememberFact   = "REMEMBER_FACT"
	IntentAddSynonym     = "ADD_SYNONYM"
	IntentAddToAutostart = "ADD_TO_AUTOSTART"
	IntentOpenApp        = "OPEN_APP"
	IntentOpenFolder     = "OPEN_FOLDER"
	IntentHardwareInfo   = "HARDWARE_INFO"
)

// Params содержит параметры, извлечённые из интента
type Params map[string]string

// DetectIntent анализирует сообщение пользователя и возвращает тип интента и параметры
func DetectIntent(msg string) (string, Params) {
	msgLower := strings.ToLower(msg)
	msgLower = strings.TrimSpace(msgLower)

	// Запомни факт: "запомни: ..." или "сохрани: ..."
	if match := regexp.MustCompile(`^(?:запомни|сохрани|запиши)\s*:\s*(.+)`).FindStringSubmatch(msgLower); match != nil {
		return IntentRememberFact, Params{"fact": strings.TrimSpace(match[1])}
	}

	// Информация о железе – теперь более гибкое, учитывает возможные предшествующие слова
	if regexp.MustCompile(`(характеристик[иа]|информаци[юя]|что за|какие|все)\s+(железо|пк|компьютер|систем[еы]|оборудование)`).MatchString(msgLower) {
		return IntentHardwareInfo, nil
	}

	// Добавить в автозагрузку
	if match := regexp.MustCompile(`(?:добавь|помести|положи)\s+(?:приложение|программу)?\s*(.+)\s+(?:в|во|на)\s+(?:автозагрузк[уа]|автозапуск)`).FindStringSubmatch(msgLower); match != nil {
		return IntentAddToAutostart, Params{"app": strings.TrimSpace(match[1])}
	}

	// Добавить синоним
	if match := regexp.MustCompile(`^(?:добавь\s+)?синоним\s+([^\s]+)\s+([^\s]+)`).FindStringSubmatch(msgLower); match != nil {
		return IntentAddSynonym, Params{"wrong": match[1], "right": match[2]}
	}

	// Открыть папку — проверяем ПЕРЕД открытием приложения,
	// иначе regex приложения перехватит "открой папку" как app="папку"
	if regexp.MustCompile(`(?:открой|открыть)\s+(автозапуск|автозагрузк[ау])`).MatchString(msgLower) {
		return IntentOpenFolder, Params{"folder": "autostart"}
	}
	if regexp.MustCompile(`(?:открой|открыть)\s+(?:папку|директорию|каталог)\s*(?:загрузки|downloads|загрузок)`).MatchString(msgLower) {
		return IntentOpenFolder, Params{"folder": "downloads"}
	}
	if regexp.MustCompile(`(?:открой|открыть)\s+(?:домашнюю|home|личную)\s*(?:папку|директорию)?`).MatchString(msgLower) {
		return IntentOpenFolder, Params{"folder": "home"}
	}
	if regexp.MustCompile(`(?:открой|открыть)\s+(?:корневую|корень|root)\s*(?:папку|директорию)?`).MatchString(msgLower) {
		return IntentOpenFolder, Params{"folder": "root"}
	}
	if regexp.MustCompile(`(?:открой|открыть)\s+папку`).MatchString(msgLower) {
		return IntentOpenFolder, Params{"folder": "unspecified"}
	}

	// Открыть приложение — после папок, чтобы "открой папку" не срабатывало как приложение
	if match := regexp.MustCompile(`(?:открой|запусти|открыть|запустить)\s+([а-яa-z0-9\-]+)`).FindStringSubmatch(msgLower); match != nil {
		return IntentOpenApp, Params{"app": match[1]}
	}

	return IntentNone, nil
}
