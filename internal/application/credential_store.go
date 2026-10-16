package application

import (
	"github.com/SgonnovDmGit/LenSA_Proxy/internal/domain/proxy"
)

// CredentialStore — кроссплатформенное хранилище credential-пары
// (логин + пароль) в системном хранилище ОС. Интерфейс определяется
// потребителем (application-слой); реализация — internal/infrastructure/keyring.
//
// Пара сохраняется и восстанавливается атомарно: логин без пароля
// бесполезен — generateCredentialPair генерирует оба поля случайно.
type CredentialStore interface {
	// Load возвращает сохранённую пару. (zero, false, nil) — записи нет.
	Load() (proxy.Credentials, bool, error)
	// Save атомарно перезаписывает пару.
	Save(credentials proxy.Credentials) error
	// Delete удаляет запись; отсутствие записи — не ошибка.
	Delete() error
}
