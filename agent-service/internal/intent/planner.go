package intent

// ToolMapping — детерминированное соответствие intent → инструмент.
// Используется для прямого вызова инструмента без LLM-рассуждения.
type ToolMapping struct {
	ToolName string
	ArgsFrom func(Params) map[string]interface{}
}

// IntentPlan — результат планирования: инструмент + аргументы + шаблон ответа.
type IntentPlan struct {
	ToolName     string
	Args         map[string]interface{}
	ResponseTmpl string
	Direct       bool
}

var intentToolMap = map[string]ToolMapping{
	IntentHardwareInfo: {
		ToolName: "full_system_report",
		ArgsFrom: func(_ Params) map[string]interface{} {
			return map[string]interface{}{}
		},
	},
	"system_info": {
		ToolName: "full_system_report",
		ArgsFrom: func(_ Params) map[string]interface{} {
			return map[string]interface{}{}
		},
	},
	"hardware_info": {
		ToolName: "full_system_report", 
		ArgsFrom: func(_ Params) map[string]interface{} {
			return map[string]interface{}{}
		},
	},
	IntentOpenApp: {
		ToolName: "findapp",
		ArgsFrom: func(p Params) map[string]interface{} {
			return map[string]interface{}{"name": p["app"]}
		},
	},
	IntentOpenFolder: {
		ToolName: "execute",
		ArgsFrom: func(p Params) map[string]interface{} {
			folder := p["folder"]
			path := folderPath(folder)
			return map[string]interface{}{"command": "xdg-open " + path}
		},
	},
	IntentAddToAutostart: {
		ToolName: "findapp",
		ArgsFrom: func(p Params) map[string]interface{} {
			return map[string]interface{}{"name": p["app"]}
		},
	},
}

// PlanIntent — детерминированный планировщик: intent + параметры → план выполнения.
// Возвращает nil если для данного интента нет прямого маппинга на инструмент.
func PlanIntent(intentType string, params Params) *IntentPlan {
	mapping, ok := intentToolMap[intentType]
	if !ok {
		return nil
	}
	return &IntentPlan{
		ToolName:     mapping.ToolName,
		Args:         mapping.ArgsFrom(params),
		ResponseTmpl: "",
		Direct:       true,
	}
}

// KnownIntents — список всех зарегистрированных интентов с описанием.
func KnownIntents() []string {
	return []string{
		IntentRememberFact,
		IntentAddSynonym, 
		IntentAddToAutostart,
		IntentOpenApp,
		IntentOpenFolder,
		IntentHardwareInfo,
		"system_info",
		"hardware_info",
	}
}

func folderPath(folder string) string {
	switch folder {
	case "downloads":
		return "$HOME/Загрузки"
	case "autostart":
		return "$HOME/.config/autostart"
	case "home":
		return "$HOME"
	case "root":
		return "/"
	default:
		return "$HOME"
	}
}
