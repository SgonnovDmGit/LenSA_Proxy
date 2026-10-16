//go:build linux || darwin || fyneui || fyneuitest

package fyneui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"

	"github.com/SgonnovDmGit/LenSA_Proxy/internal/domain/proxy"
)

// lensaTheme оборачивает стандартную тему Fyne и подставляет variant
// по выбору пользователя: Light/Dark принудительно, System — variant,
// который передаёт движок (следование ОС). Mode меняется на главном
// потоке; после смены вызывается Settings().SetTheme для реаплая.
type lensaTheme struct {
	base fyne.Theme
	mode proxy.Theme
}

func newLensaTheme(mode proxy.Theme) *lensaTheme {
	return &lensaTheme{base: theme.DefaultTheme(), mode: mode}
}

func (t *lensaTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch t.mode {
	case proxy.ThemeLight:
		variant = theme.VariantLight
	case proxy.ThemeDark:
		variant = theme.VariantDark
	}
	return t.base.Color(name, variant)
}

func (t *lensaTheme) Font(style fyne.TextStyle) fyne.Resource {
	return t.base.Font(style)
}

func (t *lensaTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return t.base.Icon(name)
}

func (t *lensaTheme) Size(name fyne.ThemeSizeName) float32 {
	return t.base.Size(name)
}
