//go:build windows

package windows

import (
	"testing"

	"github.com/SgonnovDmGit/LenSA_Proxy/internal/domain/proxy"
	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
)

func TestResolvePaletteLight(t *testing.T) {
	palette := resolvePalette(proxy.ThemeLight, false)
	light := lightPalette()
	if palette != light {
		t.Fatalf("resolvePalette(ThemeLight, false) != lightPalette()")
	}
}

func TestResolvePaletteDark(t *testing.T) {
	palette := resolvePalette(proxy.ThemeDark, false)
	dark := darkPalette()
	if palette != dark {
		t.Fatalf("resolvePalette(ThemeDark, false) != darkPalette()")
	}
}

func TestResolvePaletteSystemDark(t *testing.T) {
	original := systemDarkModeDetector
	defer func() { systemDarkModeDetector = original }()
	systemDarkModeDetector = func() bool { return true }

	palette := resolvePalette(proxy.ThemeSystem, false)
	dark := darkPalette()
	if palette != dark {
		t.Fatalf("resolvePalette(ThemeSystem, false) with dark mode != darkPalette()")
	}
}

func TestResolvePaletteSystemLight(t *testing.T) {
	original := systemDarkModeDetector
	defer func() { systemDarkModeDetector = original }()
	systemDarkModeDetector = func() bool { return false }

	palette := resolvePalette(proxy.ThemeSystem, false)
	light := lightPalette()
	if palette != light {
		t.Fatalf("resolvePalette(ThemeSystem, false) with light mode != lightPalette()")
	}
}

func TestResolvePaletteHighContrastAlwaysLight(t *testing.T) {
	original := systemDarkModeDetector
	defer func() { systemDarkModeDetector = original }()
	systemDarkModeDetector = func() bool { return true }

	tests := []struct {
		name  string
		theme proxy.Theme
	}{
		{name: "light", theme: proxy.ThemeLight},
		{name: "dark", theme: proxy.ThemeDark},
		{name: "system", theme: proxy.ThemeSystem},
	}
	light := lightPalette()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			palette := resolvePalette(tt.theme, true)
			if palette != light {
				t.Fatalf("resolvePalette(%d, true) != lightPalette (high contrast must override)", tt.theme)
			}
		})
	}
}

func TestDarkPaletteColors(t *testing.T) {
	palette := darkPalette()
	tests := []struct {
		name string
		got  win.COLORREF
		want win.COLORREF
	}{
		{name: "face", got: palette.faceColor, want: win.RGB(0x1a, 0x1b, 0x1e)},
		{name: "window", got: palette.windowColor, want: win.RGB(0x25, 0x26, 0x29)},
		{name: "text", got: palette.textColor, want: win.RGB(0xe4, 0xe4, 0xe7)},
		{name: "muted", got: palette.mutedColor, want: win.RGB(0x9c, 0xa3, 0xaf)},
		{name: "border", got: palette.borderColor, want: win.RGB(0x3f, 0x40, 0x45)},
		{name: "shadow", got: palette.shadowColor, want: win.RGB(0x2d, 0x2e, 0x32)},
		{name: "blue", got: palette.blueColor, want: win.RGB(0x3b, 0x82, 0xf6)},
		{name: "bluePressed", got: palette.bluePressedColor, want: win.RGB(0x25, 0x63, 0xeb)},
		{name: "red", got: palette.redColor, want: win.RGB(0xef, 0x44, 0x44)},
		{name: "redPressed", got: palette.redPressedColor, want: win.RGB(0xdc, 0x26, 0x26)},
		{name: "dotSuccess", got: palette.dotSuccessColor, want: win.RGB(0x22, 0xc5, 0x5e)},
		{name: "dotWarning", got: palette.dotWarningColor, want: win.RGB(0xf5, 0x9e, 0x0b)},
		{name: "dotDanger", got: palette.dotDangerColor, want: win.RGB(0xef, 0x44, 0x44)},
		{name: "dotMuted", got: palette.dotMutedColor, want: win.RGB(0x6b, 0x72, 0x80)},
		{name: "icon", got: palette.iconColor, want: win.RGB(0xe4, 0xe4, 0xe7)},
		{name: "disabledIcon", got: palette.disabledIconColor, want: win.RGB(0x6b, 0x72, 0x80)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("%s = %v, want %v", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestLightPaletteUsesSystemColors(t *testing.T) {
	palette := lightPalette()
	tests := []struct {
		name string
		got  win.COLORREF
		want win.COLORREF
	}{
		{name: "face", got: palette.faceColor, want: win.GetSysColor(co.COLOR_BTNFACE)},
		{name: "window", got: palette.windowColor, want: win.GetSysColor(co.COLOR_WINDOW)},
		{name: "text", got: palette.textColor, want: win.GetSysColor(co.COLOR_WINDOWTEXT)},
		{name: "muted", got: palette.mutedColor, want: win.GetSysColor(co.COLOR_GRAYTEXT)},
		{name: "icon", got: palette.iconColor, want: win.GetSysColor(co.COLOR_BTNTEXT)},
		{name: "disabledIcon", got: palette.disabledIconColor, want: win.GetSysColor(co.COLOR_GRAYTEXT)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("%s = %v, want %v (system color)", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestLightPaletteActionColors(t *testing.T) {
	palette := lightPalette()
	tests := []struct {
		name string
		got  win.COLORREF
		want win.COLORREF
	}{
		{name: "blue", got: palette.blueColor, want: win.RGB(37, 99, 235)},
		{name: "bluePressed", got: palette.bluePressedColor, want: win.RGB(29, 78, 216)},
		{name: "red", got: palette.redColor, want: win.RGB(220, 38, 38)},
		{name: "redPressed", got: palette.redPressedColor, want: win.RGB(185, 28, 28)},
		{name: "dotSuccess", got: palette.dotSuccessColor, want: win.RGB(22, 163, 74)},
		{name: "dotWarning", got: palette.dotWarningColor, want: win.RGB(180, 83, 9)},
		{name: "dotDanger", got: palette.dotDangerColor, want: win.RGB(220, 38, 38)},
		{name: "dotMuted", got: palette.dotMutedColor, want: win.RGB(124, 135, 151)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("%s = %v, want %v", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestIsHighContrast(t *testing.T) {
	// На тестовой машине High Contrast обычно выключен.
	// Проверяем, что функция не паникует и возвращает bool.
	result := isHighContrast()
	_ = result
}

func TestIsSystemDarkMode(t *testing.T) {
	// Проверяем, что функция не паникует и возвращает bool.
	// На Win7 ключ отсутствует → false. На Win10+ зависит от настроек.
	result := isSystemDarkMode()
	_ = result
}
