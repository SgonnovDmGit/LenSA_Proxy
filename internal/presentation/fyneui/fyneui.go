//go:build linux || darwin || fyneui || fyneuitest

// Package fyneui — presentation-слой LenSA Proxy на Fyne для Linux и macOS.
// Паритет с Windows UI (internal/presentation/windows): та же форма
// настроек, статус-блок и русский текст интерфейса. Общая view-логика
// живёт в internal/presentation/form.
//
// Тег fyneuitest — для запуска view-тестов на Windows-хосте без CGO:
// файлы пакета не импортируют fyne.io/fyne/v2/app (glfw), а тесты
// используют in-memory test driver.
package fyneui

import (
	"context"
	"crypto/rand"
	_ "embed"
	"errors"
	"image/color"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/SgonnovDmGit/LenSA_Proxy/internal/domain/proxy"
	"github.com/SgonnovDmGit/LenSA_Proxy/internal/presentation/form"
)

//go:embed icon.png
var iconPNG []byte

const (
	snapshotInterval = 200 * time.Millisecond
	stopTimeout      = 5 * time.Second
	themeLightLabel  = "Светлая"
	themeDarkLabel   = "Тёмная"
	themeSystemLabel = "Системная"
)

type operation uint8

const (
	operationNone operation = iota
	operationStart
	operationStop
)

// Service — контракт application-слоя с точки зрения этого presentation.
type Service interface {
	Interfaces() ([]proxy.NetworkInterface, error)
	Start(proxy.Config) error
	Stop(context.Context) error
	Snapshot() proxy.Snapshot
	SaveSettings(proxy.PersistedSettings) error
	LoadedSettings() proxy.PersistedSettings
	LoadedCredentials() (proxy.Credentials, bool)
	CredentialStoreAvailable() bool
}

type appWindow struct {
	app        fyne.App
	window     fyne.Window
	service    Service
	lensaTheme *lensaTheme
	interfaces []proxy.NetworkInterface

	iface        *widget.Select
	port         *widget.Entry
	copyPort     *widget.Button
	auth         *widget.Check
	generate     *widget.Button
	login        *widget.Entry
	copyLogin    *widget.Button
	password     *widget.Entry
	reveal       *widget.Button
	copyPassword *widget.Button
	storeWarning *canvas.Text
	dot          *canvas.Text
	status       *widget.Label
	description  *widget.Label
	host         *widget.Entry
	copyHost     *widget.Button
	clients      *widget.Label
	action       *widget.Button
	themeGroup   *widget.RadioGroup

	current         form.ViewModel
	currentTheme    proxy.Theme
	inFlight        operation
	pending         proxy.Config
	ready           bool
	closing         bool
	passwordVisible bool
	version         string
}

// Run создаёт окно и конфигурирует Fyne-приложение. Вызывается из run.go
// после app.New, чтобы этот файл не тянул glfw (CGO) в view-тесты.
func Run(fyneApp fyne.App, service Service, version string) (int, error) {
	if service == nil {
		return 1, errors.New("fyne service is nil")
	}
	interfaces, err := service.Interfaces()
	if err != nil {
		return 1, err
	}
	themeMode := service.LoadedSettings().Theme
	fyneTheme := newLensaTheme(themeMode)
	fyneApp.Settings().SetTheme(fyneTheme)
	w := newAppWindow(fyneApp, service, interfaces, fyneTheme, themeMode, version)
	done := make(chan struct{})
	go w.runTicker(done)
	w.applyInitialSettings()
	w.window.ShowAndRun()
	close(done)
	return 0, nil
}

func newAppWindow(fyneApp fyne.App, service Service, interfaces []proxy.NetworkInterface, lensaTheme *lensaTheme, themeMode proxy.Theme, version string) *appWindow {
	window := fyneApp.NewWindow("LenSA Proxy")
	w := &appWindow{
		app:          fyneApp,
		window:       window,
		service:      service,
		lensaTheme:   lensaTheme,
		interfaces:   append([]proxy.NetworkInterface(nil), interfaces...),
		currentTheme: themeMode,
		version:      version,
	}
	window.SetIcon(fyne.NewStaticResource("lensa-proxy.png", iconPNG))
	window.SetContent(w.buildContent())
	window.SetCloseIntercept(w.closeIntercept)
	window.Resize(fyne.NewSize(560, 720))
	w.ready = true
	return w
}

func (w *appWindow) buildContent() fyne.CanvasObject {
	resize := layout.NewSpacer

	w.themeGroup = widget.NewRadioGroup([]string{themeLightLabel, themeDarkLabel, themeSystemLabel}, w.changeTheme)
	w.themeGroup.Horizontal = true
	w.themeGroup.Required = true
	title := widget.NewLabelWithStyle("LenSA Proxy", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	header := container.NewBorder(nil, nil,
		container.NewHBox(title, widget.NewLabel(form.VersionText(w.version))),
		w.themeGroup)

	w.iface = w.newInterfaceSelect()
	w.port = widget.NewEntry()
	w.port.SetText(strconv.Itoa(int(proxy.DefaultPort)))
	w.port.Validator = func(value string) error {
		_, err := form.ParsePort(value)
		return err
	}
	w.port.OnChanged = func(string) {
		if w.ready {
			w.onFormChanged()
		}
	}
	w.copyPort = widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		w.copyText(w.port.Text)
	})

	w.auth = widget.NewCheck("Требовать авторизацию", func(checked bool) {
		if !w.ready {
			return
		}
		if !checked {
			w.setPasswordVisible(false)
		}
		w.updateFormEnabled()
		w.refresh()
	})
	w.generate = widget.NewButton("Сгенерировать", w.generateCredentials)

	w.login = widget.NewEntry()
	w.login.OnChanged = func(string) {
		if w.ready {
			w.updateFormEnabled()
		}
	}
	w.copyLogin = widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		w.copyText(w.login.Text)
	})
	w.password = widget.NewEntry()
	w.password.Password = true
	w.password.OnChanged = func(string) {
		if w.ready {
			w.updateFormEnabled()
		}
	}
	w.reveal = widget.NewButtonWithIcon("", theme.VisibilityIcon(), func() {
		w.setPasswordVisible(!w.passwordVisible)
	})
	w.copyPassword = widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		w.copyText(w.password.Text)
	})

	w.storeWarning = canvas.NewText(
		"Системное хранилище недоступно — пароль не сохранится",
		theme.Color(theme.ColorNameWarning),
	)
	w.storeWarning.TextSize = theme.Size(theme.SizeNameCaptionText)

	settingsCard := widget.NewCard("Подключение", "", container.NewVBox(
		w.fieldLabel("Сетевой интерфейс"), w.iface,
		w.fieldLabel("Порт"), container.NewBorder(nil, nil, nil, w.copyPort, w.port),
		container.NewBorder(nil, nil, w.auth, w.generate),
		w.fieldLabel("Логин"), container.NewBorder(nil, nil, nil, w.copyLogin, w.login),
		w.fieldLabel("Пароль"), container.NewBorder(nil, nil, nil,
			container.NewHBox(w.reveal, w.copyPassword), w.password),
		w.storeWarning,
	))

	w.dot = canvas.NewText("●", theme.Color(theme.ColorNameForeground))
	w.dot.TextSize = theme.Size(theme.SizeNameSubHeadingText)
	w.status = widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	w.description = widget.NewLabel("")
	w.host = widget.NewEntry()
	w.host.Disable()
	w.copyHost = widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		w.copyText(w.host.Text)
	})
	w.clients = widget.NewLabel("")
	statusCard := widget.NewCard("", "", container.NewVBox(
		container.NewHBox(w.dot, w.status),
		container.NewPadded(w.description),
		w.fieldLabel("Хост"),
		container.NewBorder(nil, nil, nil, w.copyHost, w.host),
		container.NewGridWithColumns(2,
			container.NewVBox(w.fieldLabel("Клиенты"), w.clients),
			container.NewVBox(w.fieldLabel("Сеть"), widget.NewLabel("LAN only")),
		),
	))

	w.action = widget.NewButton("", w.onAction)
	w.action.Importance = widget.HighImportance

	return container.NewPadded(container.NewVBox(
		header, settingsCard, statusCard, w.action, resize(),
	))
}

func (w *appWindow) newInterfaceSelect() *widget.Select {
	names := make([]string, len(w.interfaces))
	for i := range w.interfaces {
		names[i] = w.interfaces[i].DisplayName()
	}
	selectWidget := widget.NewSelect(names, func(string) {
		if w.ready {
			w.onFormChanged()
		}
	})
	selectWidget.PlaceHolder = "—"
	if len(names) != 0 {
		selectWidget.SetSelectedIndex(0)
	}
	return selectWidget
}

func (w *appWindow) fieldLabel(text string) *widget.Label {
	label := widget.NewLabel(text)
	label.TextStyle = fyne.TextStyle{}
	label.Importance = widget.LowImportance
	return label
}

// runTicker — цикл опроса snapshot, как WM_TIMER в Windows-версии.
func (w *appWindow) runTicker(done <-chan struct{}) {
	ticker := time.NewTicker(snapshotInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			fyne.Do(w.refresh)
		case <-done:
			return
		}
	}
}

func (w *appWindow) readForm() form.FormValues {
	return form.FormValues{
		InterfaceIndex: w.iface.SelectedIndex(),
		Port:           w.port.Text,
		AuthEnabled:    w.auth.Checked,
		Username:       w.login.Text,
		Password:       w.password.Text,
	}
}

// readPersistedSettings маппит form values в PersistedSettings, явно
// отбрасывая credentials. Порт при невалидном значении — DefaultPort.
func (w *appWindow) readPersistedSettings() proxy.PersistedSettings {
	values := w.readForm()
	settings := proxy.PersistedSettings{
		AuthEnabled: values.AuthEnabled,
		Theme:       w.currentTheme,
	}
	if values.InterfaceIndex >= 0 && values.InterfaceIndex < len(w.interfaces) {
		settings.InterfaceName = w.interfaces[values.InterfaceIndex].Name
	}
	port, err := form.ParsePort(values.Port)
	if err != nil {
		port = proxy.DefaultPort
	}
	settings.Port = port
	return settings
}

// applyInitialSettings применяет загруженные настройки к форме: интерфейс
// по имени, порт, auth, восстановленная credential-пара, тема и
// видимость предупреждения о недоступном хранилище.
func (w *appWindow) applyInitialSettings() {
	config := w.service.Snapshot().Config
	for i, iface := range w.interfaces {
		if iface.Name == config.Interface.Name {
			w.iface.SetSelectedIndex(i)
			break
		}
	}
	if config.Port >= proxy.MinPort {
		w.port.SetText(strconv.Itoa(int(config.Port)))
	}
	w.auth.SetChecked(config.AuthEnabled)
	if credentials, found := w.service.LoadedCredentials(); found {
		w.login.SetText(credentials.Username)
		w.password.SetText(credentials.Password)
	}
	w.themeGroup.SetSelected(themeLabel(w.currentTheme))
	if w.service.CredentialStoreAvailable() {
		w.storeWarning.Hide()
	}
	w.refresh()
}

func themeLabel(themeMode proxy.Theme) string {
	switch themeMode {
	case proxy.ThemeLight:
		return themeLightLabel
	case proxy.ThemeDark:
		return themeDarkLabel
	default:
		return themeSystemLabel
	}
}

func (w *appWindow) changeTheme(label string) {
	themeMode := proxy.ThemeSystem
	switch label {
	case themeLightLabel:
		themeMode = proxy.ThemeLight
	case themeDarkLabel:
		themeMode = proxy.ThemeDark
	}
	if w.currentTheme == themeMode {
		return
	}
	w.currentTheme = themeMode
	w.lensaTheme.mode = themeMode
	w.app.Settings().SetTheme(w.lensaTheme)
	w.refresh()
}

func (w *appWindow) onFormChanged() {
	w.refresh()
}

func (w *appWindow) onAction() {
	if w.closing || w.inFlight != operationNone {
		return
	}
	snapshot := w.service.Snapshot()
	switch snapshot.State {
	case proxy.StateStopped, proxy.StateError:
		config, err := form.ParseForm(w.readForm(), w.interfaces)
		if err != nil {
			w.showError(form.SafeFormError(err))
			return
		}
		w.pending = config
		w.inFlight = operationStart
		w.applySnapshot(proxy.Snapshot{State: proxy.StateStarting, Config: config})
		go func() {
			err := w.service.Start(config)
			fyne.Do(func() { w.finishOperation(operationStart, err) })
		}()
	case proxy.StateRunning:
		w.inFlight = operationStop
		snapshot.State = proxy.StateStopping
		w.applySnapshot(snapshot)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), stopTimeout)
			err := w.service.Stop(ctx)
			cancel()
			fyne.Do(func() { w.finishOperation(operationStop, err) })
		}()
	}
}

func (w *appWindow) finishOperation(kind operation, err error) {
	if w.inFlight == kind {
		w.inFlight = operationNone
	}
	if err != nil && !w.closing {
		fallback := "Не удалось остановить прокси-сервер"
		if kind == operationStart {
			fallback = "Не удалось запустить прокси-сервер"
		}
		w.showServiceError(fallback)
	}
	w.refresh()
}

func (w *appWindow) refresh() {
	snapshot := w.service.Snapshot()
	if w.closing {
		snapshot.State = proxy.StateStopping
	} else {
		switch w.inFlight {
		case operationStart:
			if snapshot.State == proxy.StateStopped || snapshot.State == proxy.StateError {
				snapshot = proxy.Snapshot{State: proxy.StateStarting, Config: w.pending}
			}
		case operationStop:
			if snapshot.State == proxy.StateRunning {
				snapshot.State = proxy.StateStopping
			}
		}
	}
	w.applySnapshot(snapshot)
}

func (w *appWindow) applySnapshot(snapshot proxy.Snapshot) {
	model := form.MapSnapshot(snapshot)
	if model.FormEnabled {
		model.Address = form.PreviewAddress(w.readForm(), w.interfaces)
		if len(w.interfaces) == 0 {
			model.Description = "Не найден активный LAN-интерфейс"
			model.ActionEnabled = false
		}
	}
	if w.closing {
		model.ActionEnabled = false
		model.FormEnabled = false
	}
	w.applyView(model)
}

func (w *appWindow) applyView(model form.ViewModel) {
	if model == w.current {
		w.updateFormEnabled()
		return
	}
	if model.FormEnabled != w.current.FormEnabled {
		w.setPasswordVisible(false)
	}
	w.current = model
	w.status.SetText(model.Status)
	w.description.SetText(model.Description)
	w.dot.Color = dotColor(model.Dot)
	w.dot.Refresh()
	host := form.ConnectionHost(model.Address)
	w.host.SetText(host)
	w.clients.SetText(model.Clients)
	w.action.SetText(model.ActionText)
	w.action.Importance = widget.HighImportance
	if model.Action == form.ActionDanger {
		w.action.Importance = widget.DangerImportance
	}
	setEnabled(w.action, model.ActionEnabled && !w.closing)
	setEnabled(w.copyHost, host != "—" && !w.closing)
	w.updateFormEnabled()
}

func (w *appWindow) updateFormEnabled() {
	formEnabled := w.current.FormEnabled && !w.closing
	setEnabled(w.iface, formEnabled)
	setEnabled(w.port, formEnabled)
	_, portErr := form.ParsePort(w.port.Text)
	setEnabled(w.copyPort, portErr == nil && !w.closing)
	setEnabled(w.auth, formEnabled)
	state := form.MapAuthControlState(formEnabled, w.auth.Checked, w.closing, w.login.Text, w.password.Text)
	setEnabled(w.login, state.CredentialsEnabled && !state.CredentialsReadOnly)
	setEnabled(w.password, state.CredentialsEnabled && !state.CredentialsReadOnly)
	setEnabled(w.generate, state.GenerateEnabled)
	setEnabled(w.copyLogin, state.CopyLoginEnabled)
	setEnabled(w.reveal, state.PasswordActionEnabled)
	setEnabled(w.copyPassword, state.PasswordActionEnabled)
	setEnabled(w.themeGroup, !w.closing)
}

func setEnabled(object fyne.Disableable, enabled bool) {
	if enabled {
		object.Enable()
	} else {
		object.Disable()
	}
}

func dotColor(tone form.DotTone) color.Color {
	switch tone {
	case form.DotWarning:
		return theme.Color(theme.ColorNameWarning)
	case form.DotSuccess:
		return theme.Color(theme.ColorNameSuccess)
	case form.DotDanger:
		return theme.Color(theme.ColorNameError)
	default:
		return theme.Color(theme.ColorNameDisabled)
	}
}

func (w *appWindow) generateCredentials() {
	credentials, err := form.GenerateCredentialPair(rand.Reader)
	if err != nil {
		w.showError("Не удалось сгенерировать данные авторизации")
		return
	}
	w.login.SetText(credentials.Username)
	w.password.SetText(credentials.Password)
	w.setPasswordVisible(false)
	w.updateFormEnabled()
}

func (w *appWindow) setPasswordVisible(visible bool) {
	if visible == w.passwordVisible {
		return
	}
	w.password.Password = !visible
	w.password.Refresh()
	w.passwordVisible = visible
	icon := theme.VisibilityIcon()
	if visible {
		icon = theme.VisibilityOffIcon()
	}
	w.reveal.SetIcon(icon)
}

func (w *appWindow) copyText(text string) {
	if text == "" || text == "—" {
		return
	}
	w.app.Clipboard().SetContent(text)
}

func (w *appWindow) showError(message string) {
	dialog.ShowError(errors.New(message), w.window)
}

func (w *appWindow) showServiceError(fallback string) {
	message := w.service.Snapshot().ErrorMessage
	w.showError(form.SafeServiceMessage(message, fallback))
}

// closeIntercept — аналог WM_CLOSE в Windows-версии: настройки
// захватываются сразу; при активном прокси — подтверждение, Stop с
// таймаутом, затем SaveSettings и реальное закрытие окна.
func (w *appWindow) closeIntercept() {
	if w.closing {
		return
	}
	pending := w.readPersistedSettings()
	snapshot := w.service.Snapshot()
	active := w.inFlight != operationNone || snapshot.State == proxy.StateStarting ||
		snapshot.State == proxy.StateRunning || snapshot.State == proxy.StateStopping
	if !active {
		_ = w.service.SaveSettings(pending)
		w.window.Close()
		return
	}
	confirm := dialog.NewConfirm(
		"LenSA Proxy",
		"Прокси-сервер ещё выполняет работу. Остановить его и закрыть приложение?",
		func(ok bool) {
			if !ok {
				return
			}
			w.closing = true
			snapshot.State = proxy.StateStopping
			w.applySnapshot(snapshot)
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), stopTimeout)
				err := w.service.Stop(ctx)
				cancel()
				fyne.Do(func() {
					if err != nil {
						w.showServiceError("Не удалось корректно остановить прокси-сервер")
					}
					_ = w.service.SaveSettings(pending)
					w.window.Close()
				})
			}()
		},
		w.window,
	)
	confirm.SetConfirmText("Остановить и закрыть")
	confirm.SetDismissText("Отмена")
	confirm.Show()
}
