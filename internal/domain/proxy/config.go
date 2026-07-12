package proxy

import (
	"errors"
	"net/netip"
	"strings"
)

const (
	MinPort     uint16 = 1024
	DefaultPort uint16 = 8080
)

var (
	ErrInterfaceNameRequired      = errors.New("не выбран сетевой интерфейс")
	ErrInterfaceAddressInvalid    = errors.New("сетевой интерфейс не имеет корректного IPv4-адреса")
	ErrInterfaceAddressNotPrivate = errors.New("разрешён только частный LAN IPv4-адрес")
	ErrPortOutOfRange             = errors.New("порт должен быть в диапазоне 1024..65535")
	ErrUsernameRequired           = errors.New("укажите логин для авторизации")
	ErrPasswordRequired           = errors.New("укажите пароль для авторизации")
)

type Credentials struct {
	Username string
	Password string
}

type NetworkInterface struct {
	Index   int
	Name    string
	Address netip.Prefix
}

func (n NetworkInterface) DisplayName() string {
	if !n.Address.IsValid() {
		return n.Name
	}
	return n.Name + " · " + n.Address.String()
}

func (n NetworkInterface) Subnet() netip.Prefix {
	if !n.Address.IsValid() {
		return netip.Prefix{}
	}
	return n.Address.Masked()
}

type Config struct {
	Interface   NetworkInterface
	Port        uint16
	AuthEnabled bool
	Credentials Credentials
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.Interface.Name) == "" {
		return ErrInterfaceNameRequired
	}
	if !c.Interface.Address.IsValid() || !c.Interface.Address.Addr().Is4() {
		return ErrInterfaceAddressInvalid
	}
	if !c.Interface.Address.Addr().IsPrivate() {
		return ErrInterfaceAddressNotPrivate
	}
	if c.Port < MinPort {
		return ErrPortOutOfRange
	}
	if c.AuthEnabled && strings.TrimSpace(c.Credentials.Username) == "" {
		return ErrUsernameRequired
	}
	if c.AuthEnabled && c.Credentials.Password == "" {
		return ErrPasswordRequired
	}
	return nil
}

func (c Config) BindAddress() string {
	if !c.Interface.Address.IsValid() || c.Port == 0 {
		return ""
	}
	return netip.AddrPortFrom(c.Interface.Address.Addr(), c.Port).String()
}
