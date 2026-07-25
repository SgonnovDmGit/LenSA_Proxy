package storage

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/SgonnovDmGit/LenSA_Proxy/internal/domain/proxy"
)

const connectionSection = "connection"

// utf8BOM — UTF-8 Byte Order Mark. Записывается в начало .ini-файла, чтобы
// редакторы Windows (Блокнот) корректно определяли кодировку и отображали
// кириллицу в имени интерфейса. При чтении BOM отрезается перед парсингом.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// SettingsStorage читает и пишет не-secret настройки в .ini-файл.
// Формат: секция [connection], ключи interface/port/auth.
// Не имеет внешних зависимостей — используется ручной парсер .ini.
type SettingsStorage struct {
	path string
}

// NewSettingsStorage создаёт хранилище настроек по указанному пути к .ini-файлу.
func NewSettingsStorage(path string) *SettingsStorage {
	return &SettingsStorage{path: path}
}

// Load читает настройки из .ini-файла. При отсутствии, повреждении или
// неполном файле возвращает defaults для отсутствующих полей, валидные — берёт.
// Пароль не читается и не хранится.
func (s *SettingsStorage) Load() (proxy.PersistedSettings, error) {
	settings := proxy.DefaultPersistedSettings()

	data, err := os.ReadFile(s.path)
	if err != nil {
		// Нет файла или нет прав на чтение — defaults, тихо.
		return settings, nil
	}
	// Отрезаем UTF-8 BOM в начале файла, если он присутствует —
	// иначе первый токен [connection] не распознаётся как секция.
	data = bytes.TrimPrefix(data, utf8BOM)

	section := ""
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		if section != connectionSection {
			continue
		}
		switch key {
		case "interface":
			settings.InterfaceName = value
		case "port":
			port, perr := strconv.ParseUint(value, 10, 16)
			if perr != nil {
				continue
			}
			if port < uint64(proxy.MinPort) {
				continue
			}
			settings.Port = uint16(port)
		case "auth":
			settings.AuthEnabled = value == "1" || strings.EqualFold(value, "true")
		case "theme":
			settings.Theme = proxy.ThemeFromString(value)
		default:
			// unknown ключ — игнорируем
		}
	}
	// Ошибка сканера не критична — берём то, что удалось распарсить.
	return settings, nil
}

// Save записывает не-secret настройки в .ini-файл. Формат:
//
//	[connection]
//	interface = Ethernet
//	port = 8080
//	auth = 0
//	theme = system
//
// Credentials не сохраняются.
func (s *SettingsStorage) Save(settings proxy.PersistedSettings) error {
	var b strings.Builder
	b.WriteString("[connection]\n")
	fmt.Fprintf(&b, "interface = %s\n", settings.InterfaceName)
	fmt.Fprintf(&b, "port = %d\n", settings.Port)
	auth := "0"
	if settings.AuthEnabled {
		auth = "1"
	}
	fmt.Fprintf(&b, "auth = %s\n", auth)
	fmt.Fprintf(&b, "theme = %s\n", settings.Theme.String())
	// Препендим UTF-8 BOM, чтобы Windows-редакторы корректно открывали файл.
	content := append(utf8BOM, b.String()...)
	return os.WriteFile(s.path, content, 0644)
}
