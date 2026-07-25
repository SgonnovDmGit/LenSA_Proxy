package storage

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SgonnovDmGit/LenSA_Proxy/internal/domain/proxy"
)

func TestLoadAbsentFileReturnsDefaults(t *testing.T) {
	storage := NewSettingsStorage(filepath.Join(t.TempDir(), "nonexistent.ini"))

	got, err := storage.Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if want := proxy.DefaultPersistedSettings(); got != want {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
}

func TestLoadEmptyFileReturnsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.ini")
	if err := os.WriteFile(path, []byte{}, 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	storage := NewSettingsStorage(path)

	got, err := storage.Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if want := proxy.DefaultPersistedSettings(); got != want {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
}

func TestLoadPartialFileReturnsDefaultsForMissingFields(t *testing.T) {
	// Файл содержит только interface без port — отсутствующие поля
	// должны получить значения по умолчанию, валидные — берутся.
	content := "[connection]\ninterface = Ethernet\n"
	path := filepath.Join(t.TempDir(), "partial.ini")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	storage := NewSettingsStorage(path)

	got, err := storage.Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if got.InterfaceName != "Ethernet" {
		t.Fatalf("InterfaceName = %q, want %q", got.InterfaceName, "Ethernet")
	}
	if got.Port != proxy.DefaultPort {
		t.Fatalf("Port = %d, want default %d", got.Port, proxy.DefaultPort)
	}
	if got.AuthEnabled {
		t.Fatalf("AuthEnabled = true, want false")
	}
}

func TestLoadCorruptedPortFallsBackToDefault(t *testing.T) {
	// Повреждённый port (не число) — fallback на DefaultPort,
	// остальные валидные поля берутся.
	content := "[connection]\ninterface = Wi-Fi\nport = abc\nauth = 1\n"
	path := filepath.Join(t.TempDir(), "corrupted.ini")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	storage := NewSettingsStorage(path)

	got, err := storage.Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if got.InterfaceName != "Wi-Fi" {
		t.Fatalf("InterfaceName = %q, want %q", got.InterfaceName, "Wi-Fi")
	}
	if got.Port != proxy.DefaultPort {
		t.Fatalf("Port = %d, want default %d", got.Port, proxy.DefaultPort)
	}
	if !got.AuthEnabled {
		t.Fatalf("AuthEnabled = false, want true")
	}
}

func TestLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "roundtrip.ini")
	storage := NewSettingsStorage(path)

	original := proxy.PersistedSettings{
		InterfaceName: "Ethernet",
		Port:          9090,
		AuthEnabled:   true,
	}
	if err := storage.Save(original); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Save должен записывать UTF-8 BOM в начало файла.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatalf("saved file must start with UTF-8 BOM, got first bytes: % x", data[:3])
	}

	// Load должен отрезать BOM и корректно прочитать файл с BOM.
	got, err := storage.Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if got != original {
		t.Fatalf("round-trip Load() = %+v, want %+v", got, original)
	}
}

func TestLoadFileWithBOM(t *testing.T) {
	// Файл с UTF-8 BOM в начале (ручная запись BOM + content) —
	// Load должен отрезать BOM и корректно распарсить настройки.
	content := []byte("[connection]\ninterface = Ethernet\nport = 8080\nauth = 1\n")
	file := append([]byte{0xEF, 0xBB, 0xBF}, content...)
	path := filepath.Join(t.TempDir(), "bom.ini")
	if err := os.WriteFile(path, file, 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	storage := NewSettingsStorage(path)

	got, err := storage.Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	want := proxy.PersistedSettings{
		InterfaceName: "Ethernet",
		Port:          8080,
		AuthEnabled:   true,
	}
	if got != want {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
}

func TestLoadFileWithoutBOM(t *testing.T) {
	// Файл без BOM — обратная совместимость: Load должен работать
	// так же, как и с BOM.
	content := "[connection]\ninterface = Wi-Fi\nport = 3128\nauth = 0\n"
	path := filepath.Join(t.TempDir(), "no-bom.ini")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	storage := NewSettingsStorage(path)

	got, err := storage.Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	want := proxy.PersistedSettings{
		InterfaceName: "Wi-Fi",
		Port:          3128,
		AuthEnabled:   false,
	}
	if got != want {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
}

func TestLoadIgnoresUnknownKeysAndExtraSections(t *testing.T) {
	content := strings.Join([]string{
		"; комментарий",
		"[other]",
		"unknown = value",
		"port = 7777",
		"",
		"[connection]",
		"interface = Ethernet",
		"port = 8080",
		"auth = 1",
		"extra = ignored",
		"",
	}, "\n")
	path := filepath.Join(t.TempDir(), "unknown.ini")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	storage := NewSettingsStorage(path)

	got, err := storage.Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	want := proxy.PersistedSettings{
		InterfaceName: "Ethernet",
		Port:          8080,
		AuthEnabled:   true,
	}
	if got != want {
		t.Fatalf("Load() = %+v, want %+v (unknown keys/sections must be ignored)", got, want)
	}
}

func TestSaveCreatesCorrectFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save.ini")
	storage := NewSettingsStorage(path)

	settings := proxy.PersistedSettings{
		InterfaceName: "Wi-Fi",
		Port:          3128,
		AuthEnabled:   false,
	}
	if err := storage.Save(settings); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatalf("saved file must start with UTF-8 BOM, got first bytes: % x", data[:3])
	}
	content := string(data)

	if !strings.Contains(content, "[connection]") {
		t.Fatalf("saved file missing [connection] section:\n%s", content)
	}
	if !strings.Contains(content, "interface = Wi-Fi") {
		t.Fatalf("saved file missing interface key:\n%s", content)
	}
	if !strings.Contains(content, "port = 3128") {
		t.Fatalf("saved file missing port key:\n%s", content)
	}
	if !strings.Contains(content, "auth = 0") {
		t.Fatalf("saved file missing auth key:\n%s", content)
	}
	if strings.Contains(content, "password") || strings.Contains(content, "username") {
		t.Fatalf("saved file must not contain credentials:\n%s", content)
	}
}
