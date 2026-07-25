//go:build windows

package windows

import (
	"syscall"
	"unsafe"

	"github.com/SgonnovDmGit/LenSA_Proxy/internal/domain/proxy"
	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
)

// uxtheme DLL и SetWindowTheme для снятия/восстановления visual styles контролов.
// Снятие visual styles необходимо, чтобы WM_CTLCOLOR* обрабатывались корректно
// в тёмной теме на Windows 7 и новее.
var (
	uxtheme            = syscall.NewLazyDLL("uxtheme.dll")
	procSetWindowTheme = uxtheme.NewProc("SetWindowTheme")
)

// disableVisualStyles снимает visual styles с конкретного контрола — контрол
// рисуется классически и уважает WM_CTLCOLOR*.
func disableVisualStyles(hwnd win.HWND) {
	empty, _ := syscall.UTF16PtrFromString("")
	_, _, _ = procSetWindowTheme.Call(uintptr(hwnd), uintptr(unsafe.Pointer(empty)), uintptr(unsafe.Pointer(empty)))
}

// resetVisualStyles возвращает стандартное темирование контрола.
func resetVisualStyles(hwnd win.HWND) {
	_, _, _ = procSetWindowTheme.Call(uintptr(hwnd), 0, 0)
}

// themePalette содержит все цвета для отрисовки окна.
// Light palette использует системные цвета Windows, dark palette —
// hardcoded hex colors. Поле highContrast=true всегда даёт light palette.
type themePalette struct {
	faceColor         win.COLORREF
	windowColor       win.COLORREF
	textColor         win.COLORREF
	mutedColor        win.COLORREF
	borderColor       win.COLORREF
	shadowColor       win.COLORREF
	blueColor         win.COLORREF
	bluePressedColor  win.COLORREF
	redColor          win.COLORREF
	redPressedColor   win.COLORREF
	dotSuccessColor   win.COLORREF
	dotWarningColor   win.COLORREF
	dotDangerColor    win.COLORREF
	dotMutedColor     win.COLORREF
	iconColor         win.COLORREF
	disabledIconColor win.COLORREF
}

// highContrastInfo — структура для SystemParametersInfo(SPI_GETHIGHCONTRAST).
// Соответствует Win32 HIGHCONTRAST: cbSize + dwFlags.
type highContrastInfo struct {
	size  uint32
	flags uint32
}

// hcfHighContrastOn — флаг HCF_HIGHCONTRASTON: High Contrast mode активен.
const hcfHighContrastOn uint32 = 0x00000001

// systemDarkModeDetector определяет, активна ли системная тёмная тема Windows.
// Package-level var для mockability в тестах — заменяется на fake в theme_test.go.
var systemDarkModeDetector = isSystemDarkMode

// resolvePalette возвращает palette для заданной темы.
// ThemeDark → dark palette, ThemeLight → light palette (system colors),
// ThemeSystem → dark если системная тёмная тема, иначе light.
// При highContrast=true — всегда light (system colors), тема не применяется.
func resolvePalette(theme proxy.Theme, highContrast bool) themePalette {
	if highContrast {
		return lightPalette()
	}
	switch theme {
	case proxy.ThemeDark:
		return darkPalette()
	case proxy.ThemeLight:
		return lightPalette()
	default: // ThemeSystem
		if systemDarkModeDetector() {
			return darkPalette()
		}
		return lightPalette()
	}
}

// darkPalette возвращает тёмную palette с hardcoded hex colors.
// Цвета соответствуют mockup v0.2-themes: тёмный фон, светлый текст,
// приглушённые границы, контрастные action/dot colors.
func darkPalette() themePalette {
	return themePalette{
		faceColor:         win.RGB(0x1a, 0x1b, 0x1e),
		windowColor:       win.RGB(0x25, 0x26, 0x29),
		textColor:         win.RGB(0xe4, 0xe4, 0xe7),
		mutedColor:        win.RGB(0x9c, 0xa3, 0xaf),
		borderColor:       win.RGB(0x3f, 0x40, 0x45),
		shadowColor:       win.RGB(0x2d, 0x2e, 0x32),
		blueColor:         win.RGB(0x3b, 0x82, 0xf6),
		bluePressedColor:  win.RGB(0x25, 0x63, 0xeb),
		redColor:          win.RGB(0xef, 0x44, 0x44),
		redPressedColor:   win.RGB(0xdc, 0x26, 0x26),
		dotSuccessColor:   win.RGB(0x22, 0xc5, 0x5e),
		dotWarningColor:   win.RGB(0xf5, 0x9e, 0x0b),
		dotDangerColor:    win.RGB(0xef, 0x44, 0x44),
		dotMutedColor:     win.RGB(0x6b, 0x72, 0x80),
		iconColor:         win.RGB(0xe4, 0xe4, 0xe7),
		disabledIconColor: win.RGB(0x6b, 0x72, 0x80),
	}
}

// lightPalette возвращает светлую palette из системных цветов Windows.
// Action/dot colors остаются hardcoded — как в v0.1.0.
func lightPalette() themePalette {
	return themePalette{
		faceColor:         win.GetSysColor(co.COLOR_BTNFACE),
		windowColor:       win.GetSysColor(co.COLOR_WINDOW),
		textColor:         win.GetSysColor(co.COLOR_WINDOWTEXT),
		mutedColor:        win.GetSysColor(co.COLOR_GRAYTEXT),
		borderColor:       win.GetSysColor(co.COLOR_BTNSHADOW),
		shadowColor:       win.GetSysColor(co.COLOR_BTNSHADOW),
		blueColor:         win.RGB(37, 99, 235),
		bluePressedColor:  win.RGB(29, 78, 216),
		redColor:          win.RGB(220, 38, 38),
		redPressedColor:   win.RGB(185, 28, 28),
		dotSuccessColor:   win.RGB(22, 163, 74),
		dotWarningColor:   win.RGB(180, 83, 9),
		dotDangerColor:    win.RGB(220, 38, 38),
		dotMutedColor:     win.RGB(124, 135, 151),
		iconColor:         win.GetSysColor(co.COLOR_BTNTEXT),
		disabledIconColor: win.GetSysColor(co.COLOR_GRAYTEXT),
	}
}

// isSystemDarkMode определяет, активна ли системная тёмная тема Windows 10+.
// Читает registry HKCU\Software\Microsoft\Windows\CurrentVersion\Themes\Personalize\AppsUseLightTheme.
// 0 = dark mode, 1 = light mode. На Win7 ключ отсутствует → false (light).
func isSystemDarkMode() bool {
	regVal, err := win.HKEY_CURRENT_USER.RegGetValue(
		"Software\\Microsoft\\Windows\\CurrentVersion\\Themes\\Personalize",
		"AppsUseLightTheme",
		co.RRF_RT_REG_DWORD,
	)
	if err != nil {
		return false
	}
	lightTheme, ok := regVal.Dword()
	if !ok {
		return false
	}
	return lightTheme == 0
}

// isHighContrast определяет, активен ли Windows High Contrast mode.
// Использует SystemParametersInfo(SPI_GETHIGHCONTRAST). При ошибке — false.
func isHighContrast() bool {
	var hc highContrastInfo
	hc.size = uint32(unsafe.Sizeof(hc))
	err := win.SystemParametersInfo(co.SPI_GETHIGHCONTRAST, hc.size, unsafe.Pointer(&hc), 0)
	if err != nil {
		return false
	}
	return hc.flags&hcfHighContrastOn != 0
}
