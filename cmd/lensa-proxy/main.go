//go:build windows && !fyneui

package main

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/SgonnovDmGit/LenSA_Proxy/internal/application"
	"github.com/SgonnovDmGit/LenSA_Proxy/internal/domain/proxy"
	"github.com/SgonnovDmGit/LenSA_Proxy/internal/infrastructure/keyring"
	"github.com/SgonnovDmGit/LenSA_Proxy/internal/infrastructure/network"
	"github.com/SgonnovDmGit/LenSA_Proxy/internal/infrastructure/storage"
	"github.com/SgonnovDmGit/LenSA_Proxy/internal/presentation/windows"
	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
)

const settingsFileName = "LenSA_Proxy.ini"

var (
	version  = "dev"
	commit   = "unknown"
	packaged = "false"
)

func main() {
	runtime.LockOSThread()
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
	run := windows.Run
	if packaged == "true" {
		run = windows.RunPackaged
	}
	code, err := run(service)
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
	_, _ = win.HWND(0).MessageBox(
		"Не удалось открыть LenSA Proxy. Проверьте доступность сетевых интерфейсов и повторите попытку.",
		"LenSA Proxy",
		co.MB_OK|co.MB_ICONERROR|co.MB_TASKMODAL,
	)
}
