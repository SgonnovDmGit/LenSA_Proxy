//go:build linux || darwin || fyneui || fyneuitest

package fyneui

import (
	"context"
	"net/netip"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"

	"github.com/SgonnovDmGit/LenSA_Proxy/internal/domain/proxy"
)

type fakeService struct {
	interfaces     []proxy.NetworkInterface
	snapshot       proxy.Snapshot
	settings       proxy.PersistedSettings
	credentials    proxy.Credentials
	hasCredentials bool
	storeAvailable bool
	started        []proxy.Config
	startErr       error
	saved          []proxy.PersistedSettings
}

func (f *fakeService) Interfaces() ([]proxy.NetworkInterface, error) {
	return f.interfaces, nil
}

func (f *fakeService) Start(config proxy.Config) error {
	f.started = append(f.started, config)
	if f.startErr == nil {
		f.snapshot.State = proxy.StateRunning
		f.snapshot.Config = config
	}
	return f.startErr
}

func (f *fakeService) Stop(context.Context) error {
	f.snapshot.State = proxy.StateStopped
	return nil
}

func (f *fakeService) Snapshot() proxy.Snapshot { return f.snapshot }

func (f *fakeService) SaveSettings(settings proxy.PersistedSettings) error {
	f.saved = append(f.saved, settings)
	return nil
}

func (f *fakeService) LoadedSettings() proxy.PersistedSettings { return f.settings }

func (f *fakeService) LoadedCredentials() (proxy.Credentials, bool) {
	return f.credentials, f.hasCredentials
}

func (f *fakeService) CredentialStoreAvailable() bool { return f.storeAvailable }

func testInterfaces() []proxy.NetworkInterface {
	return []proxy.NetworkInterface{
		{Index: 4, Name: "Ethernet", Address: netip.MustParsePrefix("192.168.10.20/24")},
	}
}

func newTestWindow(service *fakeService) *appWindow {
	fyneApp := test.NewApp()
	lensaTheme := newLensaTheme(service.settings.Theme)
	fyneApp.Settings().SetTheme(lensaTheme)
	return newAppWindow(fyneApp, service, service.interfaces, lensaTheme, service.settings.Theme)
}

func newFakeService() *fakeService {
	return &fakeService{
		interfaces:     testInterfaces(),
		snapshot:       proxy.Snapshot{State: proxy.StateStopped},
		settings:       proxy.PersistedSettings{Theme: proxy.ThemeSystem},
		storeAvailable: true,
	}
}

func TestWindowRendersForm(t *testing.T) {
	service := newFakeService()
	w := newTestWindow(service)
	w.applyInitialSettings()

	if got := w.status.Text; got != "ОСТАНОВЛЕН" {
		t.Fatalf("status = %q, want ОСТАНОВЛЕН", got)
	}
	if got := w.action.Text; got != "Запустить" {
		t.Fatalf("action text = %q, want Запустить", got)
	}
	if got := w.host.Text; got != "192.168.10.20" {
		t.Fatalf("host = %q, want 192.168.10.20", got)
	}
	if got := w.port.Text; got != "8080" {
		t.Fatalf("port = %q, want 8080", got)
	}
}

func TestStartDisablesFormFields(t *testing.T) {
	service := newFakeService()
	w := newTestWindow(service)
	w.applyInitialSettings()
	w.onAction()

	deadline := time.Now().Add(2 * time.Second)
	for w.action.Text != "Остановить" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if len(service.started) != 1 {
		t.Fatalf("Start called %d times, want 1", len(service.started))
	}
	if got := service.started[0].Interface.Name; got != "Ethernet" {
		t.Fatalf("started interface = %q, want Ethernet", got)
	}
	// После успешного Start сервис в Running — форма заблокирована.
	if !w.iface.Disabled() {
		t.Fatal("interface select enabled while running")
	}
	if !w.port.Disabled() {
		t.Fatal("port entry enabled while running")
	}
	if got := w.action.Text; got != "Остановить" {
		t.Fatalf("action text = %q, want Остановить", got)
	}
}

func TestInvalidPortDoesNotStart(t *testing.T) {
	service := newFakeService()
	w := newTestWindow(service)
	w.applyInitialSettings()
	w.port.SetText("abc")
	w.onAction()

	if len(service.started) != 0 {
		t.Fatalf("Start called %d times, want 0", len(service.started))
	}
	if got := w.action.Text; got != "Запустить" {
		t.Fatalf("action text = %q, want Запустить", got)
	}
}

func TestThemeToggleChangesVariant(t *testing.T) {
	service := newFakeService()
	w := newTestWindow(service)
	w.applyInitialSettings()

	w.changeTheme(themeDarkLabel)
	if w.lensaTheme.mode != proxy.ThemeDark {
		t.Fatalf("theme mode = %v, want ThemeDark", w.lensaTheme.mode)
	}
	name := theme.ColorNameBackground
	got := w.lensaTheme.Color(name, theme.VariantLight)
	want := theme.DefaultTheme().Color(name, theme.VariantDark)
	if got != want {
		t.Fatalf("dark mode Color(…, VariantLight) = %v, want dark variant %v", got, want)
	}
	if w.currentTheme != proxy.ThemeDark {
		t.Fatalf("currentTheme = %v, want ThemeDark", w.currentTheme)
	}
}

func TestUnavailableStoreShowsWarning(t *testing.T) {
	service := newFakeService()
	service.storeAvailable = false
	w := newTestWindow(service)
	w.applyInitialSettings()

	if !w.storeWarning.Visible() {
		t.Fatal("store warning hidden with unavailable store")
	}

	service = newFakeService()
	w = newTestWindow(service)
	w.applyInitialSettings()
	if w.storeWarning.Visible() {
		t.Fatal("store warning visible with available store")
	}
}

func TestRestoredCredentialsFillForm(t *testing.T) {
	service := newFakeService()
	service.credentials = proxy.Credentials{Username: "lensa-aabbcc", Password: "secret"}
	service.hasCredentials = true
	service.snapshot.Config.AuthEnabled = true
	w := newTestWindow(service)
	w.applyInitialSettings()

	if got := w.login.Text; got != "lensa-aabbcc" {
		t.Fatalf("login = %q, want restored lensa-aabbcc", got)
	}
	if got := w.password.Text; got != "secret" {
		t.Fatalf("password = %q, want restored secret", got)
	}
}

func TestCloseWhileStoppedSavesSettingsAndCloses(t *testing.T) {
	service := newFakeService()
	w := newTestWindow(service)
	w.applyInitialSettings()
	w.closeIntercept()

	if len(service.saved) != 1 {
		t.Fatalf("SaveSettings called %d times, want 1", len(service.saved))
	}
	if got := service.saved[0].InterfaceName; got != "Ethernet" {
		t.Fatalf("saved interface = %q, want Ethernet", got)
	}
}

func TestRunningSnapshotShowsCredentialsReadOnly(t *testing.T) {
	service := newFakeService()
	service.snapshot = proxy.Snapshot{
		State: proxy.StateRunning,
		Config: proxy.Config{
			Interface:   testInterfaces()[0],
			Port:        8080,
			AuthEnabled: true,
			Credentials: proxy.Credentials{Username: "u", Password: "p"},
		},
		Address: "192.168.10.20:8080",
		Clients: 2,
	}
	w := newTestWindow(service)
	w.applyInitialSettings()
	w.refresh()

	if got := w.status.Text; got != "РАБОТАЕТ" {
		t.Fatalf("status = %q, want РАБОТАЕТ", got)
	}
	if got := w.clients.Text; got != "2" {
		t.Fatalf("clients = %q, want 2", got)
	}
	if !w.reveal.Disabled() {
		t.Fatal("reveal enabled without credentials in form")
	}
}

func TestGenerateFillsCredentialPair(t *testing.T) {
	service := newFakeService()
	service.snapshot.Config.AuthEnabled = true
	w := newTestWindow(service)
	w.applyInitialSettings()
	w.auth.SetChecked(true)
	w.generateCredentials()

	if w.login.Text == "" || w.password.Text == "" {
		t.Fatalf("generated pair empty: login=%q", w.login.Text)
	}
	if len(w.login.Text) < 7 || w.login.Text[:6] != "lensa-" {
		t.Fatalf("login = %q, want lensa-* prefix", w.login.Text)
	}
}

func TestApplyViewDoesNotRecreate(t *testing.T) {
	service := newFakeService()
	w := newTestWindow(service)
	w.applyInitialSettings()
	first := w.current
	// Повторный refresh с тем же snapshot не должен перерисовывать заново.
	w.refresh()
	if w.current != first {
		t.Fatal("view model changed on identical refresh")
	}
}

// Защита от гонки: service.Stop в onAction должен вызываться асинхронно и
// приводить к состоянию «остановлено».
func TestStopReturnsToStopped(t *testing.T) {
	service := newFakeService()
	service.snapshot.State = proxy.StateRunning
	service.snapshot.Config = proxy.Config{Interface: testInterfaces()[0], Port: 8080}
	service.snapshot.Address = "192.168.10.20:8080"
	w := newTestWindow(service)
	w.applyInitialSettings()
	w.onAction()

	deadline := time.Now().Add(2 * time.Second)
	for service.snapshot.State != proxy.StateStopped && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if service.snapshot.State != proxy.StateStopped {
		t.Fatalf("state = %v, want stopped", service.snapshot.State)
	}
	w.refresh()
	if got := w.action.Text; got != "Запустить" {
		t.Fatalf("action text = %q, want Запустить", got)
	}
}
