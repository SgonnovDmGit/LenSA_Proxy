package proxy

// Theme — выбор цветовой темы приложения.
// ThemeLight — всегда светлая, ThemeDark — всегда тёмная,
// ThemeSystem — следует системной теме Windows (Win10+ dark preference,
// на Win7 всегда светлая). Значение по умолчанию — ThemeSystem.
type Theme uint8

const (
	// ThemeLight — принудительно светлая тема (системные цвета Windows).
	ThemeLight Theme = iota
	// ThemeDark — принудительно тёмная тема (override palette).
	ThemeDark
	// ThemeSystem — следует системной теме: dark на Win10+ если выбрана,
	// иначе light. Значение по умолчанию.
	ThemeSystem
)

// DefaultTheme возвращает тему по умолчанию — ThemeSystem.
func DefaultTheme() Theme {
	return ThemeSystem
}

// ThemeFromString парсит строковое представление темы.
// "light" → ThemeLight, "dark" → ThemeDark, "system" → ThemeSystem.
// Пустая строка и неизвестные значения → ThemeSystem (default).
func ThemeFromString(value string) Theme {
	switch value {
	case "light":
		return ThemeLight
	case "dark":
		return ThemeDark
	case "system":
		return ThemeSystem
	default:
		return ThemeSystem
	}
}

// String возвращает строковое представление темы:
// "light", "dark" или "system".
func (t Theme) String() string {
	switch t {
	case ThemeLight:
		return "light"
	case ThemeDark:
		return "dark"
	case ThemeSystem:
		return "system"
	default:
		return "system"
	}
}
