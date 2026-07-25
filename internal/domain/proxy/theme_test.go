package proxy

import "testing"

func TestThemeFromString(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  Theme
	}{
		{name: "light", value: "light", want: ThemeLight},
		{name: "dark", value: "dark", want: ThemeDark},
		{name: "system", value: "system", want: ThemeSystem},
		{name: "empty defaults to system", value: "", want: ThemeSystem},
		{name: "unknown defaults to system", value: "auto", want: ThemeSystem},
		{name: "case sensitive unknown", value: "Light", want: ThemeSystem},
		{name: "whitespace is unknown", value: " light ", want: ThemeSystem},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ThemeFromString(tt.value); got != tt.want {
				t.Fatalf("ThemeFromString(%q) = %d, want %d", tt.value, got, tt.want)
			}
		})
	}
}

func TestThemeString(t *testing.T) {
	tests := []struct {
		theme Theme
		want  string
	}{
		{ThemeLight, "light"},
		{ThemeDark, "dark"},
		{ThemeSystem, "system"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.theme.String(); got != tt.want {
				t.Fatalf("Theme(%d).String() = %q, want %q", tt.theme, got, tt.want)
			}
		})
	}
}

func TestThemeStringRoundTrip(t *testing.T) {
	themes := []Theme{ThemeLight, ThemeDark, ThemeSystem}
	for _, theme := range themes {
		t.Run(theme.String(), func(t *testing.T) {
			roundTrip := ThemeFromString(theme.String())
			if roundTrip != theme {
				t.Fatalf("round-trip failed: ThemeFromString(%q) = %d, want %d",
					theme.String(), roundTrip, theme)
			}
		})
	}
}

func TestDefaultTheme(t *testing.T) {
	if got := DefaultTheme(); got != ThemeSystem {
		t.Fatalf("DefaultTheme() = %d, want %d (ThemeSystem)", got, ThemeSystem)
	}
}
