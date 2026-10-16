// Package keyring реализует application.CredentialStore через системное
// хранилище ОС: Windows Credential Manager, macOS Keychain,
// Linux Secret Service (dbus).
package keyring

import (
	"errors"
	"fmt"
	"strings"

	"github.com/SgonnovDmGit/LenSA_Proxy/internal/domain/proxy"
	"github.com/zalando/go-keyring"
)

const (
	serviceName = "LenSA_Proxy"
	accountName = "proxy-credentials"
	separator   = "\n"
)

// Store хранит credential-пару в системном хранилище ОС одной записью
// service=LenSA_Proxy, account=proxy-credentials со значением
// "username\npassword". Username — не секрет, но хранится вместе с паролем:
// пара восстанавливается атомарно, логин без пароля бесполезен.
type Store struct{}

func NewStore() *Store {
	return &Store{}
}

func (s *Store) Load() (proxy.Credentials, bool, error) {
	raw, err := keyring.Get(serviceName, accountName)
	if errors.Is(err, keyring.ErrNotFound) {
		return proxy.Credentials{}, false, nil
	}
	if err != nil {
		return proxy.Credentials{}, false, mapStoreError(err)
	}
	username, password, found := strings.Cut(raw, separator)
	if !found || username == "" {
		// Повреждённая или устаревшая запись — считаем отсутствующей.
		return proxy.Credentials{}, false, nil
	}
	return proxy.Credentials{Username: username, Password: password}, true, nil
}

func (s *Store) Save(credentials proxy.Credentials) error {
	value := credentials.Username + separator + credentials.Password
	if err := keyring.Set(serviceName, accountName, value); err != nil {
		return mapStoreError(err)
	}
	return nil
}

func (s *Store) Delete() error {
	err := keyring.Delete(serviceName, accountName)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return mapStoreError(err)
	}
	return nil
}

// mapStoreError оборачивает ошибки провайдера в proxy.ErrStoreUnavailable,
// сохраняя исходную причину для диагностики.
func mapStoreError(err error) error {
	return fmt.Errorf("%w: %w", proxy.ErrStoreUnavailable, err)
}
