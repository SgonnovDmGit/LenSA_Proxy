package keyring

import (
	"errors"
	"testing"

	"github.com/SgonnovDmGit/LenSA_Proxy/internal/domain/proxy"
	"github.com/zalando/go-keyring"
)

// MockInit глобален для процесса — тесты не должны параллелиться
// с обращениями к реальному keyring.

func TestStoreLoadMissing(t *testing.T) {
	keyring.MockInit()
	store := NewStore()
	credentials, found, err := store.Load()
	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}
	if found {
		t.Fatalf("Load: expected found=false, got %+v", credentials)
	}
}

func TestStoreSaveLoadDeleteRoundTrip(t *testing.T) {
	keyring.MockInit()
	store := NewStore()
	want := proxy.Credentials{Username: "lensa-abc123", Password: "p@ss word"}

	if err := store.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, found, err := store.Load()
	if err != nil || !found {
		t.Fatalf("Load after Save: found=%v err=%v", found, err)
	}
	if got != want {
		t.Fatalf("Load: got %+v, want %+v", got, want)
	}

	// Перезапись — атомарная замена пары.
	want.Username = "lensa-zzz999"
	if err := store.Save(want); err != nil {
		t.Fatalf("Save overwrite: %v", err)
	}
	got, found, _ = store.Load()
	if !found || got != want {
		t.Fatalf("Load after overwrite: got %+v found=%v, want %+v", got, found, want)
	}

	if err := store.Delete(); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, found, err = store.Load(); err != nil || found {
		t.Fatalf("Load after Delete: found=%v err=%v", found, err)
	}
}

func TestStoreDeleteMissingIsNotError(t *testing.T) {
	keyring.MockInit()
	store := NewStore()
	if err := store.Delete(); err != nil {
		t.Fatalf("Delete on missing record: %v", err)
	}
}

func TestStoreLoadCorruptedValue(t *testing.T) {
	keyring.MockInit()
	// Запись без разделителя и с пустым логином — считается отсутствующей.
	_ = keyring.Set(serviceName, accountName, "no-separator-here")
	store := NewStore()
	if _, found, err := store.Load(); err != nil || found {
		t.Fatalf("corrupted value: found=%v err=%v", found, err)
	}
	_ = keyring.Set(serviceName, accountName, "\nonly-password")
	if _, found, err := store.Load(); err != nil || found {
		t.Fatalf("empty username: found=%v err=%v", found, err)
	}
}

func TestStoreProviderErrorMapsToUnavailable(t *testing.T) {
	keyring.MockInitWithError(errors.New("dbus: connection refused"))
	store := NewStore()

	if _, _, err := store.Load(); !errors.Is(err, proxy.ErrStoreUnavailable) {
		t.Fatalf("Load: want ErrStoreUnavailable, got %v", err)
	}
	if err := store.Save(proxy.Credentials{Username: "u", Password: "p"}); !errors.Is(err, proxy.ErrStoreUnavailable) {
		t.Fatalf("Save: want ErrStoreUnavailable, got %v", err)
	}
	if err := store.Delete(); !errors.Is(err, proxy.ErrStoreUnavailable) {
		t.Fatalf("Delete: want ErrStoreUnavailable, got %v", err)
	}
}
