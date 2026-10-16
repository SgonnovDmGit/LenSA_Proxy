package form

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
		want     ViewModel
	}{
		{
			name:     "stopped without address",
			snapshot: proxy.Snapshot{State: proxy.StateStopped},
			want: ViewModel{
				Status:        "ОСТАНОВЛЕН",
				Description:   "Готов к запуску",
				Address:       "—",
				Clients:       "0",
				ActionText:    "Запустить",
				ActionEnabled: true,
				FormEnabled:   true,
				Dot:           DotMuted,
				Action:        ActionPrimary,
			},
		},
		{
			name: "starting",
			snapshot: proxy.Snapshot{
				State:  proxy.StateStarting,
				Config: proxy.Config{Interface: iface, Port: 8080},
			},
			want: ViewModel{
				Status:      "ЗАПУСК",
				Description: "Открываю порт 8080…",
				Address:     "192.168.10.20:8080",
				Clients:     "0",
				ActionText:  "Запускаю…",
				Dot:         DotWarning,
				Action:      ActionPrimary,
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
			want: ViewModel{
				Status:        "РАБОТАЕТ",
				Description:   "Прокси доступен в локальной сети",
				Address:       "192.168.10.20:49152",
				Clients:       "3",
				ActionText:    "Остановить",
				ActionEnabled: true,
				Dot:           DotSuccess,
				Action:        ActionDanger,
			},
		},
		{
			name: "stopping clamps clients",
			snapshot: proxy.Snapshot{
				State:   proxy.StateStopping,
				Config:  proxy.Config{Interface: iface, Port: 8080},
				Clients: -2,
			},
			want: ViewModel{
				Status:      "ОСТАНОВКА",
				Description: "Закрываю активные соединения…",
				Address:     "192.168.10.20:8080",
				Clients:     "0",
				ActionText:  "Останавливаю…",
				Dot:         DotWarning,
				Action:      ActionDanger,
			},
		},
		{
			name: "error",
			snapshot: proxy.Snapshot{
				State:        proxy.StateError,
				ErrorMessage: "Выбранный порт уже занят",
			},
			want: ViewModel{
				Status:        "ОШИБКА",
				Description:   "Выбранный порт уже занят",
				Address:       "—",
				Clients:       "0",
				ActionText:    "Повторить",
				ActionEnabled: true,
				FormEnabled:   true,
				Dot:           DotDanger,
				Action:        ActionPrimary,
			},
		},
		{
			name:     "unknown",
			snapshot: proxy.Snapshot{State: proxy.State(255)},
			want: ViewModel{
				Status:      "ОШИБКА",
				Description: "Неизвестное состояние",
				Address:     "—",
				Clients:     "0",
				ActionText:  "Повторить",
				Dot:         DotDanger,
				Action:      ActionPrimary,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MapSnapshot(tt.snapshot); got != tt.want {
				t.Fatalf("MapSnapshot() = %+v, want %+v", got, tt.want)
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
			if got := ConnectionHost(tt.address); got != tt.want {
				t.Fatalf("ConnectionHost(%q) = %q, want %q", tt.address, got, tt.want)
			}
		})
	}
}

func TestGenerateCredentialPair(t *testing.T) {
	random := make([]byte, 18)
	for i := range random {
		random[i] = byte(i)
	}
	got, err := GenerateCredentialPair(bytes.NewReader(random))
	if err != nil {
		t.Fatalf("GenerateCredentialPair() error = %v", err)
	}
	want := proxy.Credentials{Username: "lensa-000102", Password: "AwQFBgcICQoLDA0ODxAR"}
	if got != want {
		t.Fatalf("GenerateCredentialPair() = %+v, want %+v", got, want)
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
			got, err := GenerateCredentialPair(random)
			if err == nil {
				t.Fatal("GenerateCredentialPair() error = nil, want error")
			}
			if got != (proxy.Credentials{}) {
				t.Fatalf("GenerateCredentialPair() = %+v, want empty credentials", got)
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
		want        AuthControlState
	}{
		{name: "auth off", formEnabled: true},
		{
			name:        "stopped with values",
			formEnabled: true,
			authEnabled: true,
			username:    "lensa",
			password:    "secret",
			want: AuthControlState{
				CredentialsEnabled:    true,
				GenerateEnabled:       true,
				CopyLoginEnabled:      true,
				PasswordActionEnabled: true,
			},
		},
		{
			name:        "running with values",
			authEnabled: true,
			username:    "lensa",
			password:    "secret",
			want: AuthControlState{
				CredentialsEnabled:    true,
				CredentialsReadOnly:   true,
				CopyLoginEnabled:      true,
				PasswordActionEnabled: true,
			},
		},
		{
			name:        "running empty values",
			authEnabled: true,
			want: AuthControlState{
				CredentialsEnabled:  true,
				CredentialsReadOnly: true,
			},
		},
		{
			name:        "error is editable",
			formEnabled: true,
			authEnabled: true,
			want: AuthControlState{
				CredentialsEnabled: true,
				GenerateEnabled:    true,
			},
		},
		{name: "closing disables everything", formEnabled: true, authEnabled: true, closing: true, username: "lensa", password: "secret"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MapAuthControlState(tt.formEnabled, tt.authEnabled, tt.closing, tt.username, tt.password); got != tt.want {
				t.Fatalf("MapAuthControlState() = %+v, want %+v", got, tt.want)
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
		values  FormValues
		want    proxy.Config
		wantErr error
	}{
		{
			name:   "valid without auth clears credentials",
			values: FormValues{InterfaceIndex: 0, Port: " 8080 ", Username: "ignored", Password: "ignored"},
			want:   proxy.Config{Interface: validInterface, Port: 8080},
		},
		{
			name: "valid with auth",
			values: FormValues{
				InterfaceIndex: 0,
				Port:           "65535",
				AuthEnabled:    true,
				Username:       "lensa",
				Password:       "proxy-pass",
			},
			want: proxy.Config{
				Interface:   validInterface,
				Port:        65535,
				AuthEnabled: true,
				Credentials: proxy.Credentials{Username: "lensa", Password: "proxy-pass"},
			},
		},
		{name: "no selection", values: FormValues{InterfaceIndex: -1, Port: "8080"}, wantErr: proxy.ErrInterfaceNameRequired},
		{name: "selection outside list", values: FormValues{InterfaceIndex: 2, Port: "8080"}, wantErr: proxy.ErrInterfaceNameRequired},
		{name: "empty port", values: FormValues{InterfaceIndex: 0}, wantErr: proxy.ErrPortOutOfRange},
		{name: "non numeric port", values: FormValues{InterfaceIndex: 0, Port: "80x0"}, wantErr: proxy.ErrPortOutOfRange},
		{name: "port below minimum", values: FormValues{InterfaceIndex: 0, Port: "1023"}, wantErr: proxy.ErrPortOutOfRange},
		{name: "port above maximum", values: FormValues{InterfaceIndex: 0, Port: "65536"}, wantErr: proxy.ErrPortOutOfRange},
		{name: "public interface", values: FormValues{InterfaceIndex: 1, Port: "8080"}, wantErr: proxy.ErrInterfaceAddressNotPrivate},
		{
			name:    "missing username",
			values:  FormValues{InterfaceIndex: 0, Port: "8080", AuthEnabled: true, Username: "  ", Password: "secret"},
			wantErr: proxy.ErrUsernameRequired,
		},
		{
			name:    "missing password",
			values:  FormValues{InterfaceIndex: 0, Port: "8080", AuthEnabled: true, Username: "lensa"},
			wantErr: proxy.ErrPasswordRequired,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseForm(tt.values, interfaces)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ParseForm() error = %v, want %v", err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Fatalf("ParseForm() = %+v, want %+v", got, tt.want)
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
			got, err := ParsePort(tt.value)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParsePort(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("ParsePort(%q) = %d, want %d", tt.value, got, tt.want)
			}
		})
	}
}

func TestPreviewAddress(t *testing.T) {
	iface := proxy.NetworkInterface{
		Index:   4,
		Name:    "Ethernet",
		Address: netip.MustParsePrefix("192.168.10.20/24"),
	}
	interfaces := []proxy.NetworkInterface{iface}

	t.Run("valid form", func(t *testing.T) {
		values := FormValues{InterfaceIndex: 0, Port: "8080", AuthEnabled: true, Username: "ignored", Password: "ignored"}
		if got := PreviewAddress(values, interfaces); got != "192.168.10.20:8080" {
			t.Fatalf("PreviewAddress() = %q, want %q", got, "192.168.10.20:8080")
		}
	})
	t.Run("invalid form returns placeholder", func(t *testing.T) {
		if got := PreviewAddress(FormValues{InterfaceIndex: -1, Port: "8080"}, interfaces); got != "—" {
			t.Fatalf("PreviewAddress() = %q, want placeholder", got)
		}
	})
	t.Run("invalid port returns placeholder", func(t *testing.T) {
		if got := PreviewAddress(FormValues{InterfaceIndex: 0, Port: "abc"}, interfaces); got != "—" {
			t.Fatalf("PreviewAddress() = %q, want placeholder", got)
		}
	})
}

func TestSafeFormError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "validation error", err: proxy.ErrPortOutOfRange, want: proxy.ErrPortOutOfRange.Error()},
		{name: "wrapped validation error", err: errors.Join(errors.New("context"), proxy.ErrUsernameRequired), want: proxy.ErrUsernameRequired.Error()},
		{name: "unknown error", err: errors.New("internal secret detail"), want: "Проверьте параметры подключения"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SafeFormError(tt.err); got != tt.want {
				t.Fatalf("SafeFormError() = %q, want %q", got, tt.want)
			}
		})
	}
}
