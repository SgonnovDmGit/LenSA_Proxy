package windows

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

type dotTone uint8

const (
	dotMuted dotTone = iota
	dotWarning
	dotSuccess
	dotDanger
)

type actionTone uint8

const (
	actionPrimary actionTone = iota
	actionDanger
)

type viewModel struct {
	status        string
	description   string
	address       string
	clients       string
	actionText    string
	actionEnabled bool
	formEnabled   bool
	dot           dotTone
	action        actionTone
}

type formValues struct {
	interfaceIndex int
	port           string
	authEnabled    bool
	username       string
	password       string
}

type authControlState struct {
	credentialsEnabled    bool
	credentialsReadOnly   bool
	generateEnabled       bool
	copyLoginEnabled      bool
	passwordActionEnabled bool
}

func connectionParts(address string) (string, string) {
	address = strings.TrimSpace(address)
	if address == "" || address == "—" {
		return "—", "—"
	}
	parsed, err := netip.ParseAddrPort(address)
	if err != nil {
		return "—", "—"
	}
	return parsed.Addr().String(), strconv.Itoa(int(parsed.Port()))
}

func generateCredentialPair(random io.Reader) (proxy.Credentials, error) {
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

func mapAuthControlState(formEnabled, authEnabled, closing bool, username, password string) authControlState {
	available := authEnabled && !closing
	return authControlState{
		credentialsEnabled:    available,
		credentialsReadOnly:   available && !formEnabled,
		generateEnabled:       available && formEnabled,
		copyLoginEnabled:      available && strings.TrimSpace(username) != "",
		passwordActionEnabled: available && password != "",
	}
}

func mapSnapshot(snapshot proxy.Snapshot) viewModel {
	address := strings.TrimSpace(snapshot.ProxyAddress())
	if address == "" {
		address = "—"
	}
	clients := snapshot.Clients
	if clients < 0 {
		clients = 0
	}
	model := viewModel{
		address: address,
		clients: strconv.Itoa(clients),
		action:  actionPrimary,
	}
	switch snapshot.State {
	case proxy.StateStopped:
		model.status = "ОСТАНОВЛЕН"
		model.description = "Готов к запуску"
		model.actionText = "Запустить"
		model.actionEnabled = true
		model.formEnabled = true
		model.dot = dotMuted
	case proxy.StateStarting:
		model.status = "ЗАПУСК"
		model.description = "Открываю порт…"
		if snapshot.Config.Port != 0 {
			model.description = "Открываю порт " + strconv.Itoa(int(snapshot.Config.Port)) + "…"
		}
		model.actionText = "Запускаю…"
		model.dot = dotWarning
	case proxy.StateRunning:
		model.status = "РАБОТАЕТ"
		model.description = "Прокси доступен в локальной сети"
		model.actionText = "Остановить"
		model.actionEnabled = true
		model.dot = dotSuccess
		model.action = actionDanger
	case proxy.StateStopping:
		model.status = "ОСТАНОВКА"
		model.description = "Закрываю активные соединения…"
		model.actionText = "Останавливаю…"
		model.dot = dotWarning
		model.action = actionDanger
	case proxy.StateError:
		model.status = "ОШИБКА"
		model.description = strings.TrimSpace(snapshot.ErrorMessage)
		if model.description == "" {
			model.description = "Не удалось выполнить операцию"
		}
		model.actionText = "Повторить"
		model.actionEnabled = true
		model.formEnabled = true
		model.dot = dotDanger
	default:
		model.status = "ОШИБКА"
		model.description = "Неизвестное состояние"
		model.actionText = "Повторить"
		model.dot = dotDanger
	}
	return model
}

func parseForm(values formValues, interfaces []proxy.NetworkInterface) (proxy.Config, error) {
	if values.interfaceIndex < 0 || values.interfaceIndex >= len(interfaces) {
		return proxy.Config{}, proxy.ErrInterfaceNameRequired
	}
	portText := strings.TrimSpace(values.port)
	if portText == "" || strings.IndexFunc(portText, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return proxy.Config{}, proxy.ErrPortOutOfRange
	}
	port, err := strconv.ParseUint(portText, 10, 16)
	if err != nil || port < uint64(proxy.MinPort) {
		return proxy.Config{}, proxy.ErrPortOutOfRange
	}
	config := proxy.Config{
		Interface:   interfaces[values.interfaceIndex],
		Port:        uint16(port),
		AuthEnabled: values.authEnabled,
	}
	if values.authEnabled {
		config.Credentials = proxy.Credentials{
			Username: values.username,
			Password: values.password,
		}
	}
	if err := config.Validate(); err != nil {
		return proxy.Config{}, err
	}
	return config, nil
}
