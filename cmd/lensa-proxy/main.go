//go:build windows

package main

import (
	"os"
	"runtime"

	"github.com/SgonnovDmGit/LenSA_Proxy/internal/application"
	"github.com/SgonnovDmGit/LenSA_Proxy/internal/domain/proxy"
	"github.com/SgonnovDmGit/LenSA_Proxy/internal/infrastructure/network"
	"github.com/SgonnovDmGit/LenSA_Proxy/internal/presentation/windows"
	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
)

var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	runtime.LockOSThread()
	service, err := application.NewService(
		network.DiscoverInterfaces,
		func(config proxy.Config) (application.Server, error) {
			return network.NewServer(config)
		},
	)
	if err != nil {
		showStartupError()
		os.Exit(1)
	}
	code, err := windows.Run(service)
	if err != nil {
		showStartupError()
		os.Exit(1)
	}
	os.Exit(code)
}

func showStartupError() {
	_, _ = win.HWND(0).MessageBox(
		"Не удалось открыть LenSA Proxy. Проверьте доступность сетевых интерфейсов и повторите попытку.",
		"LenSA Proxy",
		co.MB_OK|co.MB_ICONERROR|co.MB_TASKMODAL,
	)
}
