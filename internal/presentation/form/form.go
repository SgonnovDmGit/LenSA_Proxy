// Package form — платформенно-нейтральная view-model формы настроек
// прокси и парсинг введённых значений. Переиспользуется presentation-
// реализациями (windows, fyneui); не зависит от конкретного UI-тулкита.
package form

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"net/netip"
	"strconv"
	"strings"

	"github.com/SgonnovDmGit/LenSA_Proxy/internal/domain/proxy"
)

type DotTone uint8

const (
	DotMuted DotTone = iota
	DotWarning
	DotSuccess
	DotDanger
)

type ActionTone uint8

const (
	ActionPrimary ActionTone = iota
	ActionDanger
)

type ViewModel struct {
	Status        string
	Description   string
	Address       string
	Clients       string
	ActionText    string
	ActionEnabled bool
	FormEnabled   bool
	Dot           DotTone
	Action        ActionTone
}

type FormValues struct {
	InterfaceIndex int
	Port           string
	AuthEnabled    bool
	Username       string
	Password       string
}

type AuthControlState struct {
	CredentialsEnabled    bool
	CredentialsReadOnly   bool
	GenerateEnabled       bool
	CopyLoginEnabled      bool
	PasswordActionEnabled bool
}

func ConnectionHost(address string) string {
	address = strings.TrimSpace(address)
	if address == "" || address == "—" {
		return "—"
	}
	parsed, err := netip.ParseAddrPort(address)
	if err != nil {
		return "—"
	}
	return parsed.Addr().String()
}

func GenerateCredentialPair(random io.Reader) (proxy.Credentials, error) {
	if random == nil {
		return proxy.Credentials{}, errors.New("random source is nil")
	}
	value := make([]byte, 18)
	if _, err := io.ReadFull(random, value); err != nil {
		return proxy.Credentials{}, err
	}
	return proxy.Credentials{
		Username: "lensa-" + hex.EncodeToString(value[:3]),
		Password: base64.RawURLEncoding.EncodeToString(value[3:]),
	}, nil
}

func MapAuthControlState(formEnabled, authEnabled, closing bool, username, password string) AuthControlState {
	available := authEnabled && !closing
	return AuthControlState{
		CredentialsEnabled:    available,
		CredentialsReadOnly:   available && !formEnabled,
		GenerateEnabled:       available && formEnabled,
		CopyLoginEnabled:      available && strings.TrimSpace(username) != "",
		PasswordActionEnabled: available && password != "",
	}
}

func MapSnapshot(snapshot proxy.Snapshot) ViewModel {
	address := strings.TrimSpace(snapshot.ProxyAddress())
	if address == "" {
		address = "—"
	}
	clients := snapshot.Clients
	if clients < 0 {
		clients = 0
	}
	model := ViewModel{
		Address: address,
		Clients: strconv.Itoa(clients),
		Action:  ActionPrimary,
	}
	switch snapshot.State {
	case proxy.StateStopped:
		model.Status = "ОСТАНОВЛЕН"
		model.Description = "Готов к запуску"
		model.ActionText = "Запустить"
		model.ActionEnabled = true
		model.FormEnabled = true
		model.Dot = DotMuted
	case proxy.StateStarting:
		model.Status = "ЗАПУСК"
		model.Description = "Открываю порт…"
		if snapshot.Config.Port != 0 {
			model.Description = "Открываю порт " + strconv.Itoa(int(snapshot.Config.Port)) + "…"
		}
		model.ActionText = "Запускаю…"
		model.Dot = DotWarning
	case proxy.StateRunning:
		model.Status = "РАБОТАЕТ"
		model.Description = "Прокси доступен в локальной сети"
		model.ActionText = "Остановить"
		model.ActionEnabled = true
		model.Dot = DotSuccess
		model.Action = ActionDanger
	case proxy.StateStopping:
		model.Status = "ОСТАНОВКА"
		model.Description = "Закрываю активные соединения…"
		model.ActionText = "Останавливаю…"
		model.Dot = DotWarning
		model.Action = ActionDanger
	case proxy.StateError:
		model.Status = "ОШИБКА"
		model.Description = strings.TrimSpace(snapshot.ErrorMessage)
		if model.Description == "" {
			model.Description = "Не удалось выполнить операцию"
		}
		model.ActionText = "Повторить"
		model.ActionEnabled = true
		model.FormEnabled = true
		model.Dot = DotDanger
	default:
		model.Status = "ОШИБКА"
		model.Description = "Неизвестное состояние"
		model.ActionText = "Повторить"
		model.Dot = DotDanger
	}
	return model
}

// PreviewAddress вычисляет адрес bind'а из текущих значений формы без
// запуска сервера. Credentials принудительно сбрасываются — на bind-адрес
// они не влияют. При невалидной форме возвращает "—".
func PreviewAddress(values FormValues, interfaces []proxy.NetworkInterface) string {
	values.AuthEnabled = false
	config, err := ParseForm(values, interfaces)
	if err != nil {
		return "—"
	}
	return config.BindAddress()
}

func ParseForm(values FormValues, interfaces []proxy.NetworkInterface) (proxy.Config, error) {
	if values.InterfaceIndex < 0 || values.InterfaceIndex >= len(interfaces) {
		return proxy.Config{}, proxy.ErrInterfaceNameRequired
	}
	port, err := ParsePort(values.Port)
	if err != nil {
		return proxy.Config{}, err
	}
	config := proxy.Config{
		Interface:   interfaces[values.InterfaceIndex],
		Port:        port,
		AuthEnabled: values.AuthEnabled,
	}
	if values.AuthEnabled {
		config.Credentials = proxy.Credentials{
			Username: values.Username,
			Password: values.Password,
		}
	}
	if err := config.Validate(); err != nil {
		return proxy.Config{}, err
	}
	return config, nil
}

func ParsePort(value string) (uint16, error) {
	portText := strings.TrimSpace(value)
	if portText == "" || strings.IndexFunc(portText, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return 0, proxy.ErrPortOutOfRange
	}
	port, err := strconv.ParseUint(portText, 10, 16)
	if err != nil || port < uint64(proxy.MinPort) {
		return 0, proxy.ErrPortOutOfRange
	}
	return uint16(port), nil
}

// SafeServiceMessage выбирает безопасный текст ошибки сервиса для показа
// пользователю: message показывается только если входит в список известных
// безопасных текстов (не несёт технических деталей), иначе — fallback,
// а при пустом fallback — общее «Не удалось выполнить операцию».
func SafeServiceMessage(message, fallback string) string {
	message = strings.TrimSpace(message)
	for _, safe := range []string{
		"Не удалось запустить прокси-сервер",
		"Не удалось остановить прокси-сервер",
		"Выбранный порт уже занят",
		"Недостаточно прав для открытия порта",
		proxy.ErrInterfaceNameRequired.Error(),
		proxy.ErrInterfaceAddressInvalid.Error(),
		proxy.ErrInterfaceAddressNotPrivate.Error(),
		proxy.ErrPortOutOfRange.Error(),
		proxy.ErrUsernameRequired.Error(),
		proxy.ErrPasswordRequired.Error(),
	} {
		if message == safe {
			return message
		}
	}
	if strings.TrimSpace(fallback) == "" {
		return "Не удалось выполнить операцию"
	}
	return fallback
}

// SafeFormError возвращает текст ошибки валидации формы, безопасный для
// показа пользователю: только известные sentinel-ошибки domain без
// технических деталей; для остального — общий текст.
func SafeFormError(err error) string {
	for _, safe := range []error{
		proxy.ErrInterfaceNameRequired,
		proxy.ErrInterfaceAddressInvalid,
		proxy.ErrInterfaceAddressNotPrivate,
		proxy.ErrPortOutOfRange,
		proxy.ErrUsernameRequired,
		proxy.ErrPasswordRequired,
	} {
		if errors.Is(err, safe) {
			return safe.Error()
		}
	}
	return "Проверьте параметры подключения"
}
