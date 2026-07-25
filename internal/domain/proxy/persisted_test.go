package proxy

import "testing"

func TestDefaultPersistedSettings(t *testing.T) {
	got := DefaultPersistedSettings()
	want := PersistedSettings{
		InterfaceName: "",
		Port:          DefaultPort,
		AuthEnabled:   false,
		Theme:         ThemeSystem,
	}
	if got != want {
		t.Fatalf("DefaultPersistedSettings() = %+v, want %+v", got, want)
	}
}
