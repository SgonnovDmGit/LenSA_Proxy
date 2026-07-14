//go:build windows

package windows

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	"github.com/SgonnovDmGit/LenSA_Proxy/internal/domain/proxy"
	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/ui"
	"github.com/rodrigocfd/windigo/win"
)

const (
	windowWidth       = 560
	windowHeight      = 567
	snapshotTimerID   = 1
	snapshotInterval  = 200
	iconResourceID    = 2
	stopTimeout       = 5 * time.Second
	operationFallback = "Не удалось выполнить операцию"
)

const windowStyle = co.WS_CAPTION | co.WS_SYSMENU | co.WS_CLIPCHILDREN | co.WS_BORDER |
	co.WS_VISIBLE | co.WS_MINIMIZEBOX

const (
	dialogSetDefaultID    co.WM = co.WM_USER + 1
	tooltipAddToolMessage co.WM = co.WM_USER + 50
	tooltipAlwaysTip      co.WS = 0x01
	tooltipNoPrefix       co.WS = 0x02
	tooltipUseWindowID          = 0x0001
	tooltipSubclass             = 0x0010
)

type tooltipInfo struct {
	size     uint32
	flags    uint32
	parent   win.HWND
	id       uintptr
	rect     win.RECT
	instance win.HINSTANCE
	text     *uint16
	param    win.LPARAM
	reserved uintptr
}

type Service interface {
	Interfaces() ([]proxy.NetworkInterface, error)
	Start(proxy.Config) error
	Stop(context.Context) error
	Snapshot() proxy.Snapshot
}

type operation uint8

const (
	operationNone operation = iota
	operationStart
	operationStop
)

type iconKind uint8

const (
	iconCopy iconKind = iota
	iconEye
	iconEyeOff
)

type operationResult struct {
	operation operation
	failed    bool
}

type windowResources struct {
	cardBrush        win.HBRUSH
	blueBrush        win.HBRUSH
	bluePressedBrush win.HBRUSH
	redBrush         win.HBRUSH
	redPressedBrush  win.HBRUSH
	faceBrush        win.HBRUSH
	windowBrush      win.HBRUSH
	shadowBrush      win.HBRUSH
	titleFont        win.HFONT
	sectionFont      win.HFONT
	statusFont       win.HFONT
	buttonFont       win.HFONT
	iconPen          win.HPEN
	disabledIconPen  win.HPEN
	ownedBrushes     []win.HBRUSH
	ownedFonts       []win.HFONT
	ownedPens        []win.HPEN
}

type appWindow struct {
	service         Service
	interfaces      []proxy.NetworkInterface
	resources       *windowResources
	wnd             *ui.Main
	title           *ui.Static
	section         *ui.Static
	settingsBox     []*ui.Static
	statusBox       []*ui.Static
	muted           []*ui.Static
	ifaceLabel      *ui.Static
	iface           *ui.ComboBox
	portLabel       *ui.Static
	port            *ui.Edit
	copyPort        *ui.Button
	auth            *ui.CheckBox
	generate        *ui.Button
	loginLabel      *ui.Static
	login           *ui.Edit
	copyLogin       *ui.Button
	passLabel       *ui.Static
	password        *ui.Edit
	revealPassword  *ui.Button
	copyPassword    *ui.Button
	statusDot       *ui.Static
	statusText      *ui.Static
	description     *ui.Static
	hostLabel       *ui.Static
	host            *ui.Edit
	copyHost        *ui.Button
	clientsName     *ui.Static
	clients         *ui.Static
	networkName     *ui.Static
	network         *ui.Static
	action          *ui.Button
	results         chan operationResult
	closeResult     chan bool
	inFlight        operation
	pending         proxy.Config
	current         viewModel
	ready           bool
	closing         bool
	timer           bool
	passwordVisible bool
	passwordMask    win.WPARAM
	tooltip         win.HWND
	tooltipTexts    [][]uint16
	tooltipInfos    []*tooltipInfo
}

func Run(service Service) (int, error) {
	return run(service, 0)
}

func RunPackaged(service Service) (int, error) {
	return run(service, iconResourceID)
}

func run(service Service, iconID uint16) (int, error) {
	if service == nil {
		return 1, errors.New("windows service is nil")
	}
	interfaces, err := service.Interfaces()
	if err != nil {
		return 1, err
	}
	resources, err := newWindowResources()
	if err != nil {
		return 1, err
	}
	defer resources.close()
	window := newAppWindow(service, interfaces, resources, iconID)
	window.bindEvents()
	return window.wnd.RunAsMain(), nil
}

func newAppWindow(service Service, interfaces []proxy.NetworkInterface, resources *windowResources, iconID uint16) *appWindow {
	clientWidth, clientHeight := ui.Dpi(windowWidth, windowHeight)
	options := ui.OptsMain().
		Title("LenSA Proxy").
		Size(clientWidth, clientHeight).
		Style(windowStyle).
		ClassBrush(resources.faceBrush)
	if iconID != 0 {
		options.ClassIconId(iconID)
	}
	wnd := ui.NewMain(options)
	window := &appWindow{
		service:     service,
		interfaces:  append([]proxy.NetworkInterface(nil), interfaces...),
		resources:   resources,
		wnd:         wnd,
		results:     make(chan operationResult, 4),
		closeResult: make(chan bool, 1),
	}
	window.createControls()
	return window
}

func (w *appWindow) createControls() {
	resize := ui.LAY_RESIZE_HOLD
	w.title = ui.NewStatic(w.wnd, ui.OptsStatic().
		Text("LenSA Proxy").
		Position(ui.Dpi(20, 17)).
		Size(ui.Dpi(520, 30)).
		CtrlStyle(co.SS_LEFT|co.SS_NOPREFIX).
		Layout(resize))
	w.section = ui.NewStatic(w.wnd, ui.OptsStatic().
		Text("Подключение").
		Position(ui.Dpi(27, 57)).
		Size(ui.Dpi(506, 22)).
		CtrlStyle(co.SS_LEFT|co.SS_NOPREFIX).
		Layout(resize))

	settingsFill := ui.NewStatic(w.wnd, ui.OptsStatic().
		Position(ui.Dpi(20, 84)).
		Size(ui.Dpi(520, 282)).
		CtrlStyle(co.SS_WHITERECT).
		Layout(resize))
	settingsFrame := ui.NewStatic(w.wnd, ui.OptsStatic().
		Position(ui.Dpi(20, 84)).
		Size(ui.Dpi(520, 282)).
		CtrlStyle(co.SS_ETCHEDFRAME).
		Layout(resize))
	w.ifaceLabel = ui.NewStatic(w.wnd, ui.OptsStatic().
		Text("Сетевой интерфейс").
		Position(ui.Dpi(37, 99)).
		Size(ui.Dpi(486, 18)).
		CtrlStyle(co.SS_LEFT|co.SS_NOPREFIX).
		Layout(resize))
	interfaceNames := make([]string, len(w.interfaces))
	for i := range w.interfaces {
		interfaceNames[i] = w.interfaces[i].DisplayName()
	}
	selected := -1
	if len(interfaceNames) != 0 {
		selected = 0
	}
	w.iface = ui.NewComboBox(w.wnd, ui.OptsComboBox().
		Position(ui.Dpi(37, 120)).
		Width(ui.DpiX(486)).
		Texts(interfaceNames...).
		Select(selected).
		Layout(resize))
	w.portLabel = ui.NewStatic(w.wnd, ui.OptsStatic().
		Text("Порт").
		Position(ui.Dpi(37, 154)).
		Size(ui.Dpi(486, 18)).
		CtrlStyle(co.SS_LEFT|co.SS_NOPREFIX).
		Layout(resize))
	w.port = ui.NewEdit(w.wnd, ui.OptsEdit().
		Text(strconv.Itoa(int(proxy.DefaultPort))).
		Position(ui.Dpi(37, 175)).
		Width(ui.DpiX(444)).
		Height(ui.DpiY(27)).
		CtrlStyle(co.ES_AUTOHSCROLL|co.ES_NOHIDESEL|co.ES_NUMBER).
		Layout(resize))
	w.copyPort = ui.NewButton(w.wnd, ui.OptsButton().
		Text("Копировать порт").
		Position(ui.Dpi(489, 175)).
		Width(ui.DpiX(34)).
		Height(ui.DpiY(27)).
		CtrlStyle(co.BS_OWNERDRAW).
		Layout(ui.LAY_MOVE_HOLD))
	w.auth = ui.NewCheckBox(w.wnd, ui.OptsCheckBox().
		Text("Требовать авторизацию").
		Position(ui.Dpi(37, 212)).
		Size(ui.Dpi(330, 28)).
		Layout(ui.LAY_HOLD_HOLD))
	w.generate = ui.NewButton(w.wnd, ui.OptsButton().
		Text("Сгенерировать").
		Position(ui.Dpi(387, 212)).
		Width(ui.DpiX(136)).
		Height(ui.DpiY(28)).
		Layout(ui.LAY_MOVE_HOLD))
	w.loginLabel = ui.NewStatic(w.wnd, ui.OptsStatic().
		Text("Логин").
		Position(ui.Dpi(37, 246)).
		Size(ui.Dpi(486, 18)).
		CtrlStyle(co.SS_LEFT|co.SS_NOPREFIX).
		Layout(resize))
	w.login = ui.NewEdit(w.wnd, ui.OptsEdit().
		Position(ui.Dpi(37, 267)).
		Width(ui.DpiX(444)).
		Height(ui.DpiY(27)).
		Layout(resize))
	w.copyLogin = ui.NewButton(w.wnd, ui.OptsButton().
		Text("Копировать логин").
		Position(ui.Dpi(489, 267)).
		Width(ui.DpiX(34)).
		Height(ui.DpiY(27)).
		CtrlStyle(co.BS_OWNERDRAW).
		Layout(ui.LAY_MOVE_HOLD))
	w.passLabel = ui.NewStatic(w.wnd, ui.OptsStatic().
		Text("Пароль").
		Position(ui.Dpi(37, 302)).
		Size(ui.Dpi(486, 18)).
		CtrlStyle(co.SS_LEFT|co.SS_NOPREFIX).
		Layout(resize))
	w.password = ui.NewEdit(w.wnd, ui.OptsEdit().
		Position(ui.Dpi(37, 323)).
		Width(ui.DpiX(400)).
		Height(ui.DpiY(27)).
		CtrlStyle(co.ES_AUTOHSCROLL|co.ES_NOHIDESEL|co.ES_PASSWORD).
		Layout(resize))
	w.revealPassword = ui.NewButton(w.wnd, ui.OptsButton().
		Text("Показать пароль").
		Position(ui.Dpi(445, 323)).
		Width(ui.DpiX(34)).
		Height(ui.DpiY(27)).
		CtrlStyle(co.BS_OWNERDRAW).
		Layout(ui.LAY_MOVE_HOLD))
	w.copyPassword = ui.NewButton(w.wnd, ui.OptsButton().
		Text("Копировать пароль").
		Position(ui.Dpi(489, 323)).
		Width(ui.DpiX(34)).
		Height(ui.DpiY(27)).
		CtrlStyle(co.BS_OWNERDRAW).
		Layout(ui.LAY_MOVE_HOLD))
	w.settingsBox = []*ui.Static{settingsFill, settingsFrame, w.ifaceLabel, w.portLabel, w.loginLabel, w.passLabel}

	statusFill := ui.NewStatic(w.wnd, ui.OptsStatic().
		Position(ui.Dpi(20, 375)).
		Size(ui.Dpi(520, 172)).
		CtrlStyle(co.SS_WHITERECT).
		Layout(ui.LAY_RESIZE_RESIZE))
	statusFrame := ui.NewStatic(w.wnd, ui.OptsStatic().
		Position(ui.Dpi(20, 375)).
		Size(ui.Dpi(520, 172)).
		CtrlStyle(co.SS_ETCHEDFRAME).
		Layout(ui.LAY_RESIZE_RESIZE))
	w.statusDot = ui.NewStatic(w.wnd, ui.OptsStatic().
		Text("●").
		Position(ui.Dpi(38, 386)).
		Size(ui.Dpi(22, 26)).
		CtrlStyle(co.SS_CENTER|co.SS_CENTERIMAGE|co.SS_NOPREFIX))
	w.statusText = ui.NewStatic(w.wnd, ui.OptsStatic().
		Position(ui.Dpi(66, 388)).
		Size(ui.Dpi(456, 24)).
		CtrlStyle(co.SS_LEFT|co.SS_CENTERIMAGE|co.SS_NOPREFIX).
		Layout(resize))
	w.description = ui.NewStatic(w.wnd, ui.OptsStatic().
		Position(ui.Dpi(66, 413)).
		Size(ui.Dpi(456, 18)).
		CtrlStyle(co.SS_LEFT|co.SS_NOPREFIX).
		Layout(resize))
	w.hostLabel = ui.NewStatic(w.wnd, ui.OptsStatic().
		Text("Хост").
		Position(ui.Dpi(38, 438)).
		Size(ui.Dpi(50, 28)).
		CtrlStyle(co.SS_LEFT|co.SS_CENTERIMAGE|co.SS_NOPREFIX))
	w.host = ui.NewEdit(w.wnd, ui.OptsEdit().
		Position(ui.Dpi(88, 438)).
		Width(ui.DpiX(393)).
		Height(ui.DpiY(28)).
		CtrlStyle(co.ES_AUTOHSCROLL|co.ES_READONLY).
		WndStyle(co.WS_CHILD|co.WS_VISIBLE).
		Layout(resize))
	w.copyHost = ui.NewButton(w.wnd, ui.OptsButton().
		Text("Копировать хост").
		Position(ui.Dpi(489, 438)).
		Width(ui.DpiX(34)).
		Height(ui.DpiY(28)).
		CtrlStyle(co.BS_OWNERDRAW).
		Layout(ui.LAY_MOVE_HOLD))
	w.clientsName = ui.NewStatic(w.wnd, ui.OptsStatic().
		Text("Клиенты").
		Position(ui.Dpi(38, 470)).
		Size(ui.Dpi(200, 16)).
		CtrlStyle(co.SS_LEFT|co.SS_NOPREFIX))
	w.clients = ui.NewStatic(w.wnd, ui.OptsStatic().
		Position(ui.Dpi(38, 486)).
		Size(ui.Dpi(200, 17)).
		CtrlStyle(co.SS_LEFT|co.SS_NOPREFIX))
	w.networkName = ui.NewStatic(w.wnd, ui.OptsStatic().
		Text("Сеть").
		Position(ui.Dpi(280, 470)).
		Size(ui.Dpi(242, 16)).
		CtrlStyle(co.SS_LEFT|co.SS_NOPREFIX))
	w.network = ui.NewStatic(w.wnd, ui.OptsStatic().
		Text("LAN only").
		Position(ui.Dpi(280, 486)).
		Size(ui.Dpi(242, 17)).
		CtrlStyle(co.SS_LEFT|co.SS_NOPREFIX))
	w.action = ui.NewButton(w.wnd, ui.OptsButton().
		Position(ui.Dpi(38, 507)).
		Width(ui.DpiX(484)).
		Height(ui.DpiY(34)).
		CtrlStyle(co.BS_OWNERDRAW).
		Layout(resize))
	w.statusBox = []*ui.Static{
		statusFill, statusFrame, w.statusDot, w.statusText, w.description,
		w.hostLabel, w.clientsName, w.clients, w.networkName, w.network,
	}
	w.muted = []*ui.Static{
		w.ifaceLabel, w.portLabel, w.loginLabel, w.passLabel, w.description,
		w.hostLabel, w.clientsName, w.networkName,
	}
}

func (w *appWindow) bindEvents() {
	w.iface.On().CbnSelChange(w.onFormChanged)
	w.port.On().EnChange(w.onFormChanged)
	w.auth.On().BnClicked(func() {
		if !w.ready {
			return
		}
		if !w.auth.IsChecked() {
			w.setPasswordVisible(false)
		}
		w.updateFormEnabled()
		w.refresh()
	})
	w.login.On().EnChange(w.updateFormEnabled)
	w.password.On().EnChange(w.updateFormEnabled)
	w.generate.On().BnClicked(w.generateCredentials)
	w.copyLogin.On().BnClicked(func() { w.copyText(w.login.Text(), "Не удалось скопировать логин") })
	w.revealPassword.On().BnClicked(func() { w.setPasswordVisible(!w.passwordVisible) })
	w.copyPassword.On().BnClicked(func() { w.copyText(w.password.Text(), "Не удалось скопировать пароль") })
	w.copyHost.On().BnClicked(func() { w.copyText(w.host.Text(), "Не удалось скопировать хост") })
	w.copyPort.On().BnClicked(func() { w.copyText(w.port.Text(), "Не удалось скопировать порт") })
	w.action.On().BnClicked(w.onAction)
	w.wnd.On().WmCreate(func(ui.WmCreate) int {
		w.applyFonts()
		w.port.LimitText(5)
		w.login.LimitText(128)
		w.password.LimitText(128)
		passwordMask, _ := w.password.Hwnd().SendMessage(co.EM_GETPASSWORDCHAR, 0, 0)
		w.passwordMask = win.WPARAM(passwordMask)
		if w.passwordMask == 0 {
			w.passwordMask = '*'
		}
		w.installTooltips()
		w.wnd.Hwnd().SendMessage(dialogSetDefaultID, win.WPARAM(w.action.CtrlId()), 0)
		w.ready = true
		if w.wnd.Hwnd().SetTimer(snapshotTimerID, snapshotInterval) == nil {
			w.timer = true
		}
		w.refresh()
		return 0
	})
	w.wnd.On().WmTimer(snapshotTimerID, w.refresh)
	w.wnd.On().WmGetMinMaxInfo(func(event ui.WmGetMinMaxInfo) {
		width, height := minimumWindowSize()
		event.Info().PtMinTrackSize = win.POINT{X: int32(width), Y: int32(height)}
	})
	w.wnd.On().WmCtlColorStatic(w.colorStatic)
	w.wnd.On().WmCtlColorBtn(w.colorButton)
	w.wnd.On().WmDrawItem(func(event ui.WmDrawItem) {
		controlID := event.ControlId()
		switch controlID {
		case int(w.action.CtrlId()):
			drawActionButton(event.DrawItemStruct(), w.current, w.resources)
		case int(w.copyLogin.CtrlId()), int(w.copyPassword.CtrlId()), int(w.copyHost.CtrlId()), int(w.copyPort.CtrlId()):
			drawIconButton(event.DrawItemStruct(), iconCopy, w.resources)
		case int(w.revealPassword.CtrlId()):
			kind := iconEye
			if w.passwordVisible {
				kind = iconEyeOff
			}
			drawIconButton(event.DrawItemStruct(), kind, w.resources)
		}
	})
	w.wnd.On().WmClose(w.close)
	w.wnd.On().WmDestroy(func() {
		if w.timer {
			_ = w.wnd.Hwnd().KillTimer(snapshotTimerID)
		}
		if w.tooltip != 0 {
			_ = w.tooltip.DestroyWindow()
			w.tooltip = 0
		}
		w.tooltipInfos = nil
		w.tooltipTexts = nil
	})
}

func (w *appWindow) applyFonts() {
	setControlFont(w.title.Hwnd(), w.resources.titleFont)
	setControlFont(w.section.Hwnd(), w.resources.sectionFont)
	setControlFont(w.statusDot.Hwnd(), w.resources.titleFont)
	setControlFont(w.statusText.Hwnd(), w.resources.statusFont)
	setControlFont(w.action.Hwnd(), w.resources.buttonFont)
}

func setControlFont(hwnd win.HWND, font win.HFONT) {
	hwnd.SendMessage(co.WM_SETFONT, win.WPARAM(font), 1)
}

func (w *appWindow) onFormChanged() {
	if w.ready {
		w.refresh()
	}
}

func (w *appWindow) readForm() formValues {
	return formValues{
		interfaceIndex: w.iface.SelectedIndex(),
		port:           w.port.Text(),
		authEnabled:    w.auth.IsChecked(),
		username:       w.login.Text(),
		password:       w.password.Text(),
	}
}

func (w *appWindow) previewAddress() string {
	values := w.readForm()
	values.authEnabled = false
	config, err := parseForm(values, w.interfaces)
	if err != nil {
		return "—"
	}
	return config.BindAddress()
}

func (w *appWindow) onAction() {
	if !w.ready || w.closing || w.inFlight != operationNone {
		return
	}
	snapshot := w.service.Snapshot()
	switch snapshot.State {
	case proxy.StateStopped, proxy.StateError:
		config, err := parseForm(w.readForm(), w.interfaces)
		if err != nil {
			w.showError(safeFormError(err))
			return
		}
		w.pending = config
		w.inFlight = operationStart
		w.applySnapshot(proxy.Snapshot{State: proxy.StateStarting, Config: config})
		go func() {
			err := w.service.Start(config)
			w.results <- operationResult{operation: operationStart, failed: err != nil}
		}()
	case proxy.StateRunning:
		w.inFlight = operationStop
		snapshot.State = proxy.StateStopping
		w.applySnapshot(snapshot)
		go w.stop(operationStop)
	}
}

func (w *appWindow) stop(kind operation) {
	ctx, cancel := context.WithTimeout(context.Background(), stopTimeout)
	err := w.service.Stop(ctx)
	cancel()
	w.results <- operationResult{operation: kind, failed: err != nil}
}

func (w *appWindow) refresh() {
	if !w.ready {
		return
	}
	select {
	case failed := <-w.closeResult:
		if failed {
			w.showServiceError("Не удалось корректно остановить прокси-сервер")
		}
		_ = w.wnd.Hwnd().DestroyWindow()
		return
	default:
	}
	w.drainResults()
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

func (w *appWindow) drainResults() {
	for {
		select {
		case result := <-w.results:
			if result.operation == w.inFlight {
				w.inFlight = operationNone
			}
			if result.failed && !w.closing {
				fallback := "Не удалось остановить прокси-сервер"
				if result.operation == operationStart {
					fallback = "Не удалось запустить прокси-сервер"
				}
				w.showServiceError(fallback)
			}
		default:
			return
		}
	}
}

func (w *appWindow) applySnapshot(snapshot proxy.Snapshot) {
	model := mapSnapshot(snapshot)
	if model.formEnabled {
		model.address = w.previewAddress()
		if len(w.interfaces) == 0 {
			model.description = "Не найден активный LAN-интерфейс"
			model.actionEnabled = false
		}
	}
	if w.closing {
		model.actionEnabled = false
		model.formEnabled = false
	}
	w.applyView(model)
}

func (w *appWindow) applyView(model viewModel) {
	if model == w.current {
		w.updateFormEnabled()
		return
	}
	if model.formEnabled != w.current.formEnabled {
		w.setPasswordVisible(false)
	}
	w.current = model
	w.statusText.Hwnd().SetWindowText(model.status)
	w.description.Hwnd().SetWindowText(model.description)
	host := connectionHost(model.address)
	w.host.SetText(host)
	w.clients.Hwnd().SetWindowText(model.clients)
	w.action.SetText(model.actionText)
	w.action.Hwnd().EnableWindow(model.actionEnabled && !w.closing)
	w.copyHost.Hwnd().EnableWindow(host != "—" && !w.closing)
	w.updateFormEnabled()
	_ = w.statusDot.Hwnd().InvalidateRect(nil, true)
	_ = w.statusText.Hwnd().InvalidateRect(nil, true)
	_ = w.description.Hwnd().InvalidateRect(nil, true)
	_ = w.action.Hwnd().InvalidateRect(nil, true)
}

func (w *appWindow) updateFormEnabled() {
	formEnabled := w.current.formEnabled && !w.closing
	w.iface.Hwnd().EnableWindow(formEnabled)
	w.port.Hwnd().EnableWindow(formEnabled)
	_, portErr := parsePort(w.port.Text())
	w.copyPort.Hwnd().EnableWindow(portErr == nil && !w.closing)
	w.auth.Hwnd().EnableWindow(formEnabled)
	state := mapAuthControlState(formEnabled, w.auth.IsChecked(), w.closing, w.login.Text(), w.password.Text())
	w.login.Hwnd().EnableWindow(state.credentialsEnabled)
	w.password.Hwnd().EnableWindow(state.credentialsEnabled)
	w.login.Hwnd().SendMessage(co.EM_SETREADONLY, boolWParam(state.credentialsReadOnly), 0)
	w.password.Hwnd().SendMessage(co.EM_SETREADONLY, boolWParam(state.credentialsReadOnly), 0)
	w.generate.Hwnd().EnableWindow(state.generateEnabled)
	w.copyLogin.Hwnd().EnableWindow(state.copyLoginEnabled)
	w.revealPassword.Hwnd().EnableWindow(state.passwordActionEnabled)
	w.copyPassword.Hwnd().EnableWindow(state.passwordActionEnabled)
}

func (w *appWindow) close() {
	if w.closing {
		return
	}
	snapshot := w.service.Snapshot()
	active := w.inFlight != operationNone || snapshot.State == proxy.StateStarting ||
		snapshot.State == proxy.StateRunning || snapshot.State == proxy.StateStopping
	if !active {
		_ = w.wnd.Hwnd().DestroyWindow()
		return
	}
	answer, err := w.wnd.Hwnd().MessageBox(
		"Прокси-сервер ещё выполняет работу. Остановить его и закрыть приложение?",
		"LenSA Proxy",
		co.MB_YESNO|co.MB_ICONWARNING|co.MB_DEFBUTTON2,
	)
	if err != nil || answer != co.ID_YES {
		return
	}
	w.closing = true
	snapshot.State = proxy.StateStopping
	w.applySnapshot(snapshot)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), stopTimeout)
		err := w.service.Stop(ctx)
		cancel()
		w.closeResult <- err != nil
	}()
}

func (w *appWindow) installTooltips() {
	tooltip, err := win.CreateWindowEx(
		0,
		win.ClassNameStr("tooltips_class32"),
		"",
		co.WS_POPUP|tooltipAlwaysTip|tooltipNoPrefix,
		win.POINT{},
		win.SIZE{},
		w.wnd.Hwnd(),
		0,
		0,
		0,
	)
	if err != nil {
		return
	}
	w.tooltip = tooltip
	tools := []struct {
		button *ui.Button
		text   string
	}{
		{w.copyLogin, "Копировать логин"},
		{w.revealPassword, "Показать или скрыть пароль"},
		{w.copyPassword, "Копировать пароль"},
		{w.copyHost, "Копировать хост"},
		{w.copyPort, "Копировать порт"},
	}
	for _, tool := range tools {
		text, err := syscall.UTF16FromString(tool.text)
		if err != nil {
			continue
		}
		w.tooltipTexts = append(w.tooltipTexts, text)
		info := &tooltipInfo{
			size:   uint32(unsafe.Sizeof(tooltipInfo{})),
			flags:  tooltipUseWindowID | tooltipSubclass,
			parent: w.wnd.Hwnd(),
			id:     uintptr(tool.button.Hwnd()),
			text:   &text[0],
		}
		w.tooltipInfos = append(w.tooltipInfos, info)
		tooltip.SendMessage(tooltipAddToolMessage, 0, win.LPARAM(uintptr(unsafe.Pointer(info))))
	}
}

func (w *appWindow) generateCredentials() {
	credentials, err := generateCredentialPair(rand.Reader)
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
	mask := w.passwordMask
	text := "Показать пароль"
	if visible {
		mask = 0
		text = "Скрыть пароль"
	}
	w.password.Hwnd().SendMessage(co.EM_SETPASSWORDCHAR, mask, 0)
	w.passwordVisible = visible
	w.revealPassword.SetText(text)
	_ = w.password.Hwnd().InvalidateRect(nil, true)
	_ = w.revealPassword.Hwnd().InvalidateRect(nil, true)
}

func (w *appWindow) copyText(text, errorMessage string) {
	if text == "" || text == "—" {
		return
	}
	if err := setClipboardText(w.wnd.Hwnd(), text); err != nil {
		w.showError(errorMessage)
	}
}

func boolWParam(value bool) win.WPARAM {
	if value {
		return 1
	}
	return 0
}

func setClipboardText(owner win.HWND, text string) (resultErr error) {
	clipboard, err := win.OpenClipboard(owner)
	if err != nil {
		return err
	}
	defer func() {
		if err := clipboard.CloseClipboard(); resultErr == nil && err != nil {
			resultErr = err
		}
	}()
	if err := clipboard.EmptyClipboard(); err != nil {
		return err
	}
	encoded := utf16.Encode([]rune(text))
	data := make([]byte, (len(encoded)+1)*2)
	for i, value := range encoded {
		binary.LittleEndian.PutUint16(data[i*2:], value)
	}
	return clipboard.SetClipboardData(co.CF_UNICODETEXT, data)
}

func safeFormError(err error) string {
	for _, safe := range []error{
		proxy.ErrInterfaceNameRequired,
		proxy.ErrInterfaceAddressInvalid,
		proxy.ErrInterfaceAddressNotPrivate,
		proxy.ErrPortOutOfRange,
		proxy.ErrUsernameRequired,
		proxy.ErrPasswordRequired,
	} {
		if errors.Is(err, safe) {
			return safe.Error()
		}
	}
	return "Проверьте параметры подключения"
}

func (w *appWindow) showServiceError(fallback string) {
	message := strings.TrimSpace(w.service.Snapshot().ErrorMessage)
	for _, safe := range []string{
		"Не удалось запустить прокси-сервер",
		"Не удалось остановить прокси-сервер",
		"Выбранный порт уже занят",
		"Недостаточно прав для открытия порта",
		proxy.ErrInterfaceNameRequired.Error(),
		proxy.ErrInterfaceAddressInvalid.Error(),
		proxy.ErrInterfaceAddressNotPrivate.Error(),
		proxy.ErrPortOutOfRange.Error(),
		proxy.ErrUsernameRequired.Error(),
		proxy.ErrPasswordRequired.Error(),
	} {
		if message == safe {
			w.showError(message)
			return
		}
	}
	if strings.TrimSpace(fallback) == "" {
		fallback = operationFallback
	}
	w.showError(fallback)
}

func (w *appWindow) showError(message string) {
	if strings.TrimSpace(message) == "" {
		message = operationFallback
	}
	_, _ = w.wnd.Hwnd().MessageBox(message, "LenSA Proxy", co.MB_OK|co.MB_ICONERROR)
}

func (w *appWindow) colorStatic(event ui.WmCtlColor) win.HBRUSH {
	hwnd := event.HwndControl()
	hdc := event.Hdc()
	if hwnd == w.statusDot.Hwnd() {
		_, _ = hdc.SetBkMode(co.BKMODE_TRANSPARENT)
		_, _ = hdc.SetTextColor(dotColor(w.current.dot))
		return w.resources.cardBrush
	}
	if containsStatic(w.settingsBox, hwnd) || containsStatic(w.statusBox, hwnd) {
		_, _ = hdc.SetBkMode(co.BKMODE_OPAQUE)
		_, _ = hdc.SetBkColor(win.GetSysColor(co.COLOR_WINDOW))
		color := win.GetSysColor(co.COLOR_WINDOWTEXT)
		if containsStatic(w.muted, hwnd) {
			color = win.GetSysColor(co.COLOR_GRAYTEXT)
		}
		if hwnd == w.description.Hwnd() && w.current.dot == dotDanger {
			color = dotColor(dotDanger)
		}
		_, _ = hdc.SetTextColor(color)
		return w.resources.cardBrush
	}
	_, _ = hdc.SetBkMode(co.BKMODE_OPAQUE)
	_, _ = hdc.SetBkColor(win.GetSysColor(co.COLOR_BTNFACE))
	_, _ = hdc.SetTextColor(win.GetSysColor(co.COLOR_BTNTEXT))
	return w.resources.faceBrush
}

func (w *appWindow) colorButton(event ui.WmCtlColor) win.HBRUSH {
	if event.HwndControl() == w.auth.Hwnd() {
		_, _ = event.Hdc().SetBkMode(co.BKMODE_OPAQUE)
		_, _ = event.Hdc().SetBkColor(win.GetSysColor(co.COLOR_WINDOW))
		_, _ = event.Hdc().SetTextColor(win.GetSysColor(co.COLOR_WINDOWTEXT))
		return w.resources.cardBrush
	}
	return w.resources.faceBrush
}

func containsStatic(statics []*ui.Static, hwnd win.HWND) bool {
	for _, control := range statics {
		if control.Hwnd() == hwnd {
			return true
		}
	}
	return false
}

func dotColor(tone dotTone) win.COLORREF {
	switch tone {
	case dotWarning:
		return win.RGB(180, 83, 9)
	case dotSuccess:
		return win.RGB(22, 163, 74)
	case dotDanger:
		return win.RGB(220, 38, 38)
	default:
		return win.RGB(124, 135, 151)
	}
}

func drawActionButton(item *win.DRAWITEMSTRUCT, model viewModel, resources *windowResources) {
	if item == nil {
		return
	}
	disabled := item.ItemState&co.ODS_DISABLED != 0
	pressed := item.ItemState&co.ODS_SELECTED != 0
	background := resources.blueBrush
	border := resources.bluePressedBrush
	textColor := win.RGB(255, 255, 255)
	if model.action == actionDanger {
		background = resources.redBrush
		border = resources.redPressedBrush
	}
	if pressed {
		background = border
	}
	if disabled {
		background = resources.faceBrush
		border = resources.shadowBrush
		textColor = win.GetSysColor(co.COLOR_GRAYTEXT)
	}
	_ = item.Hdc.FillRect(&item.RcItem, background)
	_ = item.Hdc.FrameRect(&item.RcItem, border)
	oldFont, fontErr := item.Hdc.SelectObjectFont(resources.buttonFont)
	if fontErr == nil {
		defer item.Hdc.SelectObjectFont(oldFont)
	}
	oldMode, modeErr := item.Hdc.SetBkMode(co.BKMODE_TRANSPARENT)
	if modeErr == nil {
		defer item.Hdc.SetBkMode(oldMode)
	}
	oldColor, colorErr := item.Hdc.SetTextColor(textColor)
	if colorErr == nil {
		defer item.Hdc.SetTextColor(oldColor)
	}
	size, err := item.Hdc.GetTextExtentPoint32(model.actionText)
	if err == nil {
		x := item.RcItem.Left + (item.RcItem.Right-item.RcItem.Left-size.Cx)/2
		y := item.RcItem.Top + (item.RcItem.Bottom-item.RcItem.Top-size.Cy)/2
		if pressed && !disabled {
			x++
			y++
		}
		_ = item.Hdc.TextOut(int(x), int(y), model.actionText)
	}
	if !disabled && item.ItemState&co.ODS_FOCUS != 0 {
		insetX, insetY := int32(ui.DpiX(4)), int32(ui.DpiY(4))
		focus := win.RECT{
			Left:   item.RcItem.Left + insetX,
			Top:    item.RcItem.Top + insetY,
			Right:  item.RcItem.Right - insetX,
			Bottom: item.RcItem.Bottom - insetY,
		}
		_ = item.Hdc.FrameRect(&focus, resources.windowBrush)
	}
}

func drawIconButton(item *win.DRAWITEMSTRUCT, kind iconKind, resources *windowResources) {
	if item == nil {
		return
	}
	disabled := item.ItemState&co.ODS_DISABLED != 0
	pressed := item.ItemState&co.ODS_SELECTED != 0
	_ = item.Hdc.FillRect(&item.RcItem, resources.faceBrush)
	_ = item.Hdc.FrameRect(&item.RcItem, resources.shadowBrush)
	pen := resources.iconPen
	if disabled {
		pen = resources.disabledIconPen
	}
	oldPen, penErr := item.Hdc.SelectObjectPen(pen)
	if penErr == nil {
		defer item.Hdc.SelectObjectPen(oldPen)
	}
	stockBrush, brushErr := win.GetStockObject(co.STOCK_NULL_BRUSH)
	if brushErr == nil {
		oldBrush, selectErr := item.Hdc.SelectObjectBrush(win.HBRUSH(stockBrush))
		if selectErr == nil {
			defer item.Hdc.SelectObjectBrush(oldBrush)
		}
	}
	x := int(item.RcItem.Left + (item.RcItem.Right-item.RcItem.Left)/2)
	y := int(item.RcItem.Top + (item.RcItem.Bottom-item.RcItem.Top)/2)
	if pressed && !disabled {
		x++
		y++
	}
	switch kind {
	case iconCopy:
		_ = item.Hdc.Rectangle(win.RECT{Left: int32(x - 7), Top: int32(y - 7), Right: int32(x + 4), Bottom: int32(y + 4)})
		_ = item.Hdc.Rectangle(win.RECT{Left: int32(x - 3), Top: int32(y - 3), Right: int32(x + 8), Bottom: int32(y + 8)})
	case iconEye, iconEyeOff:
		_ = item.Hdc.Ellipse(win.RECT{Left: int32(x - 8), Top: int32(y - 5), Right: int32(x + 9), Bottom: int32(y + 6)})
		_ = item.Hdc.Ellipse(win.RECT{Left: int32(x - 2), Top: int32(y - 2), Right: int32(x + 3), Bottom: int32(y + 3)})
		if kind == iconEyeOff {
			_, _ = item.Hdc.MoveToEx(x-8, y-8)
			_ = item.Hdc.LineTo(x+8, y+8)
		}
	}
	if !disabled && item.ItemState&co.ODS_FOCUS != 0 {
		focus := win.RECT{
			Left:   item.RcItem.Left + int32(ui.DpiX(3)),
			Top:    item.RcItem.Top + int32(ui.DpiY(3)),
			Right:  item.RcItem.Right - int32(ui.DpiX(3)),
			Bottom: item.RcItem.Bottom - int32(ui.DpiY(3)),
		}
		_ = item.Hdc.FrameRect(&focus, resources.windowBrush)
	}
}

func minimumWindowSize() (int, int) {
	width, height := ui.Dpi(windowWidth, windowHeight)
	rect := win.RECT{Right: int32(width), Bottom: int32(height)}
	if win.AdjustWindowRectEx(&rect, windowStyle, false, 0) != nil {
		return width, height
	}
	return int(rect.Right - rect.Left), int(rect.Bottom - rect.Top)
}

func newWindowResources() (*windowResources, error) {
	resources := &windowResources{}
	var err error
	if resources.faceBrush, err = win.GetSysColorBrush(co.COLOR_BTNFACE); err != nil {
		return nil, err
	}
	if resources.windowBrush, err = win.GetSysColorBrush(co.COLOR_WINDOW); err != nil {
		return nil, err
	}
	resources.cardBrush = resources.windowBrush
	if resources.shadowBrush, err = win.GetSysColorBrush(co.COLOR_BTNSHADOW); err != nil {
		return nil, err
	}
	brushes := []struct {
		field *win.HBRUSH
		color win.COLORREF
	}{
		{&resources.blueBrush, win.RGB(37, 99, 235)},
		{&resources.bluePressedBrush, win.RGB(29, 78, 216)},
		{&resources.redBrush, win.RGB(220, 38, 38)},
		{&resources.redPressedBrush, win.RGB(185, 28, 28)},
	}
	for _, item := range brushes {
		brush, err := win.CreateBrushIndirect(&win.LOGBRUSH{Style: co.BRS_SOLID, Color: item.color})
		if err != nil {
			resources.close()
			return nil, err
		}
		*item.field = brush
		resources.ownedBrushes = append(resources.ownedBrushes, brush)
	}
	fonts := []struct {
		field  *win.HFONT
		height int
		weight co.FW
	}{
		{&resources.titleFont, 22, co.FW_BOLD},
		{&resources.sectionFont, 15, co.FW_BOLD},
		{&resources.statusFont, 17, co.FW_BOLD},
		{&resources.buttonFont, 14, co.FW_SEMIBOLD},
	}
	for _, item := range fonts {
		font, err := newFont(item.height, item.weight)
		if err != nil {
			resources.close()
			return nil, err
		}
		*item.field = font
		resources.ownedFonts = append(resources.ownedFonts, font)
	}
	pens := []struct {
		field *win.HPEN
		color win.COLORREF
	}{
		{&resources.iconPen, win.GetSysColor(co.COLOR_BTNTEXT)},
		{&resources.disabledIconPen, win.GetSysColor(co.COLOR_GRAYTEXT)},
	}
	for _, item := range pens {
		pen, err := win.CreatePen(co.PS_SOLID, ui.DpiX(1), item.color)
		if err != nil {
			resources.close()
			return nil, err
		}
		*item.field = pen
		resources.ownedPens = append(resources.ownedPens, pen)
	}
	return resources, nil
}

func newFont(height int, weight co.FW) (win.HFONT, error) {
	font := win.LOGFONT{
		Height:  -int32(ui.DpiY(height)),
		Weight:  weight,
		CharSet: co.CHARSET_DEFAULT,
		Quality: co.QUALITY_CLEARTYPE,
	}
	font.SetFaceName("Segoe UI")
	return win.CreateFontIndirect(&font)
}

func (r *windowResources) close() {
	for i := len(r.ownedPens) - 1; i >= 0; i-- {
		_ = r.ownedPens[i].DeleteObject()
	}
	for i := len(r.ownedFonts) - 1; i >= 0; i-- {
		_ = r.ownedFonts[i].DeleteObject()
	}
	for i := len(r.ownedBrushes) - 1; i >= 0; i-- {
		_ = r.ownedBrushes[i].DeleteObject()
	}
	r.ownedPens = nil
	r.ownedFonts = nil
	r.ownedBrushes = nil
}
