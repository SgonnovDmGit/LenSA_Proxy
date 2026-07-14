package windows

import (
	"bytes"
	"errors"
	"io"
	"net/netip"
	"testing"

	"github.com/SgonnovDmGit/LenSA_Proxy/internal/domain/proxy"
)

func TestMapSnapshot(t *testing.T) {
	iface := proxy.NetworkInterface{
		Index:   4,
		Name:    "Ethernet",
		Address: netip.MustParsePrefix("192.168.10.20/24"),
	}
	tests := []struct {
		name     string
		snapshot proxy.Snapshot
		want     viewModel
	}{
		{
			name:     "stopped without address",
			snapshot: proxy.Snapshot{State: proxy.StateStopped},
			want: viewModel{
				status:        "ОСТАНОВЛЕН",
				description:   "Готов к запуску",
				address:       "—",
				clients:       "0",
				actionText:    "Запустить",
				actionEnabled: true,
				formEnabled:   true,
				dot:           dotMuted,
				action:        actionPrimary,
			},
		},
		{
			name: "starting",
			snapshot: proxy.Snapshot{
				State:  proxy.StateStarting,
				Config: proxy.Config{Interface: iface, Port: 8080},
			},
			want: viewModel{
				status:      "ЗАПУСК",
				description: "Открываю порт 8080…",
				address:     "192.168.10.20:8080",
				clients:     "0",
				actionText:  "Запускаю…",
				dot:         dotWarning,
				action:      actionPrimary,
			},
		},
		{
			name: "running uses actual address",
			snapshot: proxy.Snapshot{
				State:   proxy.StateRunning,
				Config:  proxy.Config{Interface: iface, Port: 8080},
				Address: "192.168.10.20:49152",
				Clients: 3,
			},
			want: viewModel{
				status:        "РАБОТАЕТ",
				description:   "Прокси доступен в локальной сети",
				address:       "192.168.10.20:49152",
				clients:       "3",
				actionText:    "Остановить",
				actionEnabled: true,
				dot:           dotSuccess,
				action:        actionDanger,
			},
		},
		{
			name: "stopping clamps clients",
			snapshot: proxy.Snapshot{
				State:   proxy.StateStopping,
				Config:  proxy.Config{Interface: iface, Port: 8080},
				Clients: -2,
			},
			want: viewModel{
				status:      "ОСТАНОВКА",
				description: "Закрываю активные соединения…",
				address:     "192.168.10.20:8080",
				clients:     "0",
				actionText:  "Останавливаю…",
				dot:         dotWarning,
				action:      actionDanger,
			},
		},
		{
			name: "error",
			snapshot: proxy.Snapshot{
				State:        proxy.StateError,
				ErrorMessage: "Выбранный порт уже занят",
			},
			want: viewModel{
				status:        "ОШИБКА",
				description:   "Выбранный порт уже занят",
				address:       "—",
				clients:       "0",
				actionText:    "Повторить",
				actionEnabled: true,
				formEnabled:   true,
				dot:           dotDanger,
				action:        actionPrimary,
			},
		},
		{
			name:     "unknown",
			snapshot: proxy.Snapshot{State: proxy.State(255)},
			want: viewModel{
				status:      "ОШИБКА",
				description: "Неизвестное состояние",
				address:     "—",
				clients:     "0",
				actionText:  "Повторить",
				dot:         dotDanger,
				action:      actionPrimary,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mapSnapshot(tt.snapshot); got != tt.want {
				t.Fatalf("mapSnapshot() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestConnectionHost(t *testing.T) {
	tests := []struct {
		name    string
		address string
		want    string
	}{
		{name: "IPv4", address: "192.168.1.80:8080", want: "192.168.1.80"},
		{name: "IPv6", address: "[2001:db8::1]:443", want: "2001:db8::1"},
		{name: "empty", want: "—"},
		{name: "placeholder", address: "—", want: "—"},
		{name: "URL is invalid", address: "http://192.168.1.80:8080", want: "—"},
		{name: "unbracketed IPv6 is invalid", address: "2001:db8::1:443", want: "—"},
		{name: "invalid port", address: "192.168.1.80:70000", want: "—"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := connectionHost(tt.address); got != tt.want {
				t.Fatalf("connectionHost(%q) = %q, want %q", tt.address, got, tt.want)
			}
		})
	}
}

func TestGenerateCredentialPair(t *testing.T) {
	random := make([]byte, 18)
	for i := range random {
		random[i] = byte(i)
	}
	got, err := generateCredentialPair(bytes.NewReader(random))
	if err != nil {
		t.Fatalf("generateCredentialPair() error = %v", err)
	}
	want := proxy.Credentials{Username: "lensa-000102", Password: "AwQFBgcICQoLDA0ODxAR"}
	if got != want {
		t.Fatalf("generateCredentialPair() = %+v, want %+v", got, want)
	}
}

func TestGenerateCredentialPairRejectsInvalidRandomInput(t *testing.T) {
	tests := []struct {
		name   string
		random *bytes.Reader
	}{
		{name: "nil"},
		{name: "short", random: bytes.NewReader(make([]byte, 17))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var random io.Reader
			if tt.random != nil {
				random = tt.random
			}
			got, err := generateCredentialPair(random)
			if err == nil {
				t.Fatal("generateCredentialPair() error = nil, want error")
			}
			if got != (proxy.Credentials{}) {
				t.Fatalf("generateCredentialPair() = %+v, want empty credentials", got)
			}
		})
	}
}

func TestMapAuthControlState(t *testing.T) {
	tests := []struct {
		name        string
		formEnabled bool
		authEnabled bool
		closing     bool
		username    string
		password    string
		want        authControlState
	}{
		{name: "auth off", formEnabled: true},
		{
			name:        "stopped with values",
			formEnabled: true,
			authEnabled: true,
			username:    "lensa",
			password:    "secret",
			want: authControlState{
				credentialsEnabled:    true,
				generateEnabled:       true,
				copyLoginEnabled:      true,
				passwordActionEnabled: true,
			},
		},
		{
			name:        "running with values",
			authEnabled: true,
			username:    "lensa",
			password:    "secret",
			want: authControlState{
				credentialsEnabled:    true,
				credentialsReadOnly:   true,
				copyLoginEnabled:      true,
				passwordActionEnabled: true,
			},
		},
		{
			name:        "running empty values",
			authEnabled: true,
			want: authControlState{
				credentialsEnabled:  true,
				credentialsReadOnly: true,
			},
		},
		{
			name:        "error is editable",
			formEnabled: true,
			authEnabled: true,
			want: authControlState{
				credentialsEnabled: true,
				generateEnabled:    true,
			},
		},
		{name: "closing disables everything", formEnabled: true, authEnabled: true, closing: true, username: "lensa", password: "secret"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mapAuthControlState(tt.formEnabled, tt.authEnabled, tt.closing, tt.username, tt.password); got != tt.want {
				t.Fatalf("mapAuthControlState() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestParseForm(t *testing.T) {
	validInterface := proxy.NetworkInterface{
		Index:   4,
		Name:    "Ethernet",
		Address: netip.MustParsePrefix("192.168.10.20/24"),
	}
	invalidInterface := proxy.NetworkInterface{Index: 5, Name: "Public", Address: netip.MustParsePrefix("8.8.8.8/24")}
	interfaces := []proxy.NetworkInterface{validInterface, invalidInterface}
	tests := []struct {
		name    string
		values  formValues
		want    proxy.Config
		wantErr error
	}{
		{
			name:   "valid without auth clears credentials",
			values: formValues{interfaceIndex: 0, port: " 8080 ", username: "ignored", password: "ignored"},
			want:   proxy.Config{Interface: validInterface, Port: 8080},
		},
		{
			name: "valid with auth",
			values: formValues{
				interfaceIndex: 0,
				port:           "65535",
				authEnabled:    true,
				username:       "lensa",
				password:       "proxy-pass",
			},
			want: proxy.Config{
				Interface:   validInterface,
				Port:        65535,
				AuthEnabled: true,
				Credentials: proxy.Credentials{Username: "lensa", Password: "proxy-pass"},
			},
		},
		{name: "no selection", values: formValues{interfaceIndex: -1, port: "8080"}, wantErr: proxy.ErrInterfaceNameRequired},
		{name: "selection outside list", values: formValues{interfaceIndex: 2, port: "8080"}, wantErr: proxy.ErrInterfaceNameRequired},
		{name: "empty port", values: formValues{interfaceIndex: 0}, wantErr: proxy.ErrPortOutOfRange},
		{name: "non numeric port", values: formValues{interfaceIndex: 0, port: "80x0"}, wantErr: proxy.ErrPortOutOfRange},
		{name: "port below minimum", values: formValues{interfaceIndex: 0, port: "1023"}, wantErr: proxy.ErrPortOutOfRange},
		{name: "port above maximum", values: formValues{interfaceIndex: 0, port: "65536"}, wantErr: proxy.ErrPortOutOfRange},
		{name: "public interface", values: formValues{interfaceIndex: 1, port: "8080"}, wantErr: proxy.ErrInterfaceAddressNotPrivate},
		{
			name:    "missing username",
			values:  formValues{interfaceIndex: 0, port: "8080", authEnabled: true, username: "  ", password: "secret"},
			wantErr: proxy.ErrUsernameRequired,
		},
		{
			name:    "missing password",
			values:  formValues{interfaceIndex: 0, port: "8080", authEnabled: true, username: "lensa"},
			wantErr: proxy.ErrPasswordRequired,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseForm(tt.values, interfaces)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("parseForm() error = %v, want %v", err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Fatalf("parseForm() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestParsePort(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    uint16
		wantErr bool
	}{
		{name: "valid", value: " 8081 ", want: 8081},
		{name: "minimum", value: "1024", want: 1024},
		{name: "maximum", value: "65535", want: 65535},
		{name: "empty", wantErr: true},
		{name: "below minimum", value: "1023", wantErr: true},
		{name: "above maximum", value: "65536", wantErr: true},
		{name: "non numeric", value: "808x", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePort(tt.value)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parsePort(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("parsePort(%q) = %d, want %d", tt.value, got, tt.want)
			}
		})
	}
}
