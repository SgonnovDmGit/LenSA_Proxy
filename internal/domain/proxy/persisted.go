package proxy

// PersistedSettings содержит не-secret настройки, сохраняемые между запусками.
// Credentials не входят в эту структуру — пароль не сохраняется.
type PersistedSettings struct {
	InterfaceName string
	Port          uint16
	AuthEnabled   bool
}

// DefaultPersistedSettings возвращает настройки по умолчанию:
// пустой интерфейс, DefaultPort, авторизация выключена.
func DefaultPersistedSettings() PersistedSettings {
	return PersistedSettings{
		InterfaceName: "",
		Port:          DefaultPort,
		AuthEnabled:   false,
	}
}
