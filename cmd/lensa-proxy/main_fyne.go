//go:build linux || darwin || fyneui

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/SgonnovDmGit/LenSA_Proxy/internal/application"
	"github.com/SgonnovDmGit/LenSA_Proxy/internal/domain/proxy"
	"github.com/SgonnovDmGit/LenSA_Proxy/internal/infrastructure/keyring"
	"github.com/SgonnovDmGit/LenSA_Proxy/internal/infrastructure/network"
	"github.com/SgonnovDmGit/LenSA_Proxy/internal/infrastructure/storage"
	"github.com/SgonnovDmGit/LenSA_Proxy/internal/presentation/fyneui"
)

const settingsFileName = "LenSA_Proxy.ini"

var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	settingsStorage := storage.NewSettingsStorage(settingsFilePath())
	service, err := application.NewService(
		network.DiscoverInterfaces,
		func(config proxy.Config) (application.Server, error) {
			return network.NewServer(config)
		},
		application.WithSettings(settingsStorage.Load, settingsStorage.Save),
		application.WithCredentialStore(keyring.NewStore()),
	)
	if err != nil {
		showStartupError()
		os.Exit(1)
	}
	code, err := fyneui.RunApp(service, version)
	if err != nil {
		showStartupError()
		os.Exit(1)
	}
	os.Exit(code)
}

// settingsFilePath вычисляет путь к .ini-файлу рядом с executable.
// При ошибке os.Executable fallback на текущий рабочий каталог.
func settingsFilePath() string {
	exe, err := os.Executable()
	if err != nil {
		return settingsFileName
	}
	return filepath.Join(filepath.Dir(exe), settingsFileName)
}

func showStartupError() {
	fmt.Fprintln(os.Stderr, "Не удалось открыть LenSA Proxy. Проверьте доступность сетевых интерфейсов и повторите попытку.")
}
