//go:build linux || darwin || fyneui

package fyneui

import "fyne.io/fyne/v2/app"

// RunApp — composition point: создаёт Fyne-приложение и передаёт в Run.
// Отделён от fyneui.go: пакет app тянет glfw (CGO), а файлы окна и тесты
// должны собираться без него (in-memory test driver).
func RunApp(service Service) (int, error) {
	return Run(app.New(), service)
}
