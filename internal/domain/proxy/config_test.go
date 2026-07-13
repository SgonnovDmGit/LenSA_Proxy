package proxy

import (
	"errors"
	"net/netip"
	"testing"
)

func TestConfigValidate(t *testing.T) {
	validInterface := NetworkInterface{
		Name:    "Ethernet",
		Address: netip.MustParsePrefix("192.168.1.42/24"),
	}

	tests := []struct {
		name    string
		config  Config
		wantErr error
	}{
		{
			name: "valid without auth",
			config: Config{
				Interface: validInterface,
				Port:      DefaultPort,
			},
		},
		{
			name: "valid with auth",
			config: Config{
				Interface:   validInterface,
				Port:        DefaultPort,
				AuthEnabled: true,
				Credentials: Credentials{Username: "lensa", Password: "secret"},
			},
		},
		{
			name: "missing interface name",
			config: Config{
				Interface: NetworkInterface{Address: validInterface.Address},
				Port:      DefaultPort,
			},
			wantErr: ErrInterfaceNameRequired,
		},
		{
			name: "invalid prefix",
			config: Config{
				Interface: NetworkInterface{Name: "Ethernet"},
				Port:      DefaultPort,
			},
			wantErr: ErrInterfaceAddressInvalid,
		},
		{
			name: "ipv6 listener",
			config: Config{
				Interface: NetworkInterface{Name: "Ethernet", Address: netip.MustParsePrefix("fd00::42/64")},
				Port:      DefaultPort,
			},
			wantErr: ErrInterfaceAddressInvalid,
		},
		{
			name: "public listener",
			config: Config{
				Interface: NetworkInterface{Name: "Ethernet", Address: netip.MustParsePrefix("203.0.113.42/24")},
				Port:      DefaultPort,
			},
			wantErr: ErrInterfaceAddressNotPrivate,
		},
		{
			name: "loopback listener",
			config: Config{
				Interface: NetworkInterface{Name: "Loopback", Address: netip.MustParsePrefix("127.0.0.1/8")},
				Port:      DefaultPort,
			},
			wantErr: ErrInterfaceAddressNotPrivate,
		},
		{
			name: "link local listener",
			config: Config{
				Interface: NetworkInterface{Name: "Ethernet", Address: netip.MustParsePrefix("169.254.1.42/16")},
				Port:      DefaultPort,
			},
			wantErr: ErrInterfaceAddressNotPrivate,
		},
		{
			name: "privileged port",
			config: Config{
				Interface: validInterface,
				Port:      1023,
			},
			wantErr: ErrPortOutOfRange,
		},
		{
			name: "zero port",
			config: Config{
				Interface: validInterface,
			},
			wantErr: ErrPortOutOfRange,
		},
		{
			name: "missing username",
			config: Config{
				Interface:   validInterface,
				Port:        DefaultPort,
				AuthEnabled: true,
				Credentials: Credentials{Password: "secret"},
			},
			wantErr: ErrUsernameRequired,
		},
		{
			name: "missing password",
			config: Config{
				Interface:   validInterface,
				Port:        DefaultPort,
				AuthEnabled: true,
				Credentials: Credentials{Username: "lensa"},
			},
			wantErr: ErrPasswordRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr == nil && err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("Validate() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestNetworkInterfaceValues(t *testing.T) {
	iface := NetworkInterface{
		Name:    "Ethernet",
		Address: netip.MustParsePrefix("192.168.1.42/24"),
	}

	if got, want := iface.DisplayName(), "Ethernet · 192.168.1.42/24"; got != want {
		t.Fatalf("DisplayName() = %q, want %q", got, want)
	}
	if got, want := iface.Subnet(), netip.MustParsePrefix("192.168.1.0/24"); got != want {
		t.Fatalf("Subnet() = %v, want %v", got, want)
	}
}

func TestConfigBindAddress(t *testing.T) {
	config := Config{
		Interface: NetworkInterface{
			Name:    "Ethernet",
			Address: netip.MustParsePrefix("192.168.1.42/24"),
		},
		Port: 8080,
	}

	if got, want := config.BindAddress(), "192.168.1.42:8080"; got != want {
		t.Fatalf("BindAddress() = %q, want %q", got, want)
	}
}
