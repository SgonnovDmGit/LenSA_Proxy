package application

import (
	"context"
	"errors"
	"os"
	"reflect"
	"sync"
	"syscall"
	"time"

	"github.com/SgonnovDmGit/LenSA_Proxy/internal/domain/proxy"
)

const (
	startErrorMessage         = "Не удалось запустить прокси-сервер"
	stopErrorMessage          = "Не удалось остановить прокси-сервер"
	portInUseErrorMessage     = "Выбранный порт уже занят"
	permissionErrorMessage    = "Недостаточно прав для открытия порта"
	windowsAddressInUseErrno  = syscall.Errno(10048)
	failedStartCleanupTimeout = 5 * time.Second
)

var (
	ErrNilInterfaceSource = errors.New("источник сетевых интерфейсов не задан")
	ErrNilServerFactory   = errors.New("фабрика прокси-сервера не задана")
	ErrNilServer          = errors.New("прокси-сервер не создан")
	ErrNilStopContext     = errors.New("контекст остановки не задан")
	ErrStartNotAllowed    = errors.New("запуск прокси-сервера недоступен в текущем состоянии")
)

type Server interface {
	Start() error
	Stop(context.Context) error
	Address() string
	Clients() int
}

type InterfaceSource func() ([]proxy.NetworkInterface, error)

type ServerFactory func(proxy.Config) (Server, error)

// SettingsLoader читает сохранённые не-secret настройки при старте.
type SettingsLoader func() (proxy.PersistedSettings, error)

// SettingsSaver записывает не-secret настройки (credentials отбрасываются вызывающей стороной).
type SettingsSaver func(proxy.PersistedSettings) error

// Option настраивает Service через функциональные опции.
type Option func(*Service)

// WithSettings подключает загрузку и сохранение не-secret настроек.
// Loader применяется к initial snapshot: если InterfaceName найден среди
// обнаруженных интерфейсов — используется он, иначе первый доступный.
// Port валидируется (MinPort..65535), при невалидности — DefaultPort.
func WithSettings(loader SettingsLoader, saver SettingsSaver) Option {
	return func(s *Service) {
		s.settingsLoader = loader
		s.settingsSaver = saver
	}
}

type Service struct {
	operationMutex  sync.Mutex
	snapshotMutex   sync.RWMutex
	interfaceSource InterfaceSource
	serverFactory   ServerFactory
	settingsLoader  SettingsLoader
	settingsSaver   SettingsSaver
	loadedSettings  proxy.PersistedSettings
	snapshot        proxy.Snapshot
	server          Server
}

func NewService(interfaceSource InterfaceSource, serverFactory ServerFactory, opts ...Option) (*Service, error) {
	if isNilValue(interfaceSource) {
		return nil, ErrNilInterfaceSource
	}
	if isNilValue(serverFactory) {
		return nil, ErrNilServerFactory
	}
	service := &Service{
		interfaceSource: interfaceSource,
		serverFactory:   serverFactory,
		loadedSettings:  proxy.DefaultPersistedSettings(),
		snapshot: proxy.Snapshot{
			State: proxy.StateStopped,
			Config: proxy.Config{
				Port: proxy.DefaultPort,
			},
		},
	}
	for _, opt := range opts {
		opt(service)
	}
	if service.settingsLoader != nil {
		loaded, _ := service.settingsLoader()
		service.applyPersistedSettings(loaded)
	}
	return service, nil
}

// applyPersistedSettings применяет загруженные настройки к initial snapshot.
// InterfaceName ищется среди обнаруженных интерфейсов; если не найден или пуст —
// fallback на первый доступный. Port валидируется, при невалидности — DefaultPort.
// Theme сохраняется в loadedSettings для последующего чтения presentation-слоем.
func (s *Service) applyPersistedSettings(settings proxy.PersistedSettings) {
	s.snapshotMutex.Lock()
	s.loadedSettings = settings
	s.snapshotMutex.Unlock()
	port := settings.Port
	if port < proxy.MinPort {
		port = proxy.DefaultPort
	}
	config := proxy.Config{
		Port:        port,
		AuthEnabled: settings.AuthEnabled,
	}
	if interfaces, err := s.interfaceSource(); err == nil && len(interfaces) > 0 {
		found := false
		for _, iface := range interfaces {
			if iface.Name == settings.InterfaceName {
				config.Interface = iface
				found = true
				break
			}
		}
		if !found {
			config.Interface = interfaces[0]
		}
	}
	s.snapshotMutex.Lock()
	s.snapshot = proxy.Snapshot{
		State:  proxy.StateStopped,
		Config: config,
	}
	s.snapshotMutex.Unlock()
}

// SaveSettings сохраняет текущие не-secret настройки через SettingsSaver.
// Если saver не задан — no-op. Ошибка записи возвращается вызывающей стороне.
func (s *Service) SaveSettings(settings proxy.PersistedSettings) error {
	if s.settingsSaver == nil {
		return nil
	}
	return s.settingsSaver(settings)
}

// LoadedSettings возвращает настройки, загруженные при старте через SettingsLoader.
// Presentation-слой использует Theme из loadedSettings для выбора палитры.
// Если loader не задан — возвращает defaults.
func (s *Service) LoadedSettings() proxy.PersistedSettings {
	s.snapshotMutex.RLock()
	defer s.snapshotMutex.RUnlock()
	return s.loadedSettings
}

func (s *Service) Interfaces() ([]proxy.NetworkInterface, error) {
	interfaces, err := s.interfaceSource()
	if err != nil {
		return nil, err
	}
	if interfaces == nil {
		return nil, nil
	}
	result := make([]proxy.NetworkInterface, len(interfaces))
	copy(result, interfaces)
	return result, nil
}

func (s *Service) Start(config proxy.Config) error {
	if !s.operationMutex.TryLock() {
		return ErrStartNotAllowed
	}
	defer s.operationMutex.Unlock()

	snapshot, _ := s.current()
	if snapshot.State != proxy.StateStopped && snapshot.State != proxy.StateError {
		return ErrStartNotAllowed
	}
	if err := config.Validate(); err != nil {
		s.failStart(config, lifecycleErrorMessage(err, startErrorMessage))
		return err
	}

	s.publish(proxy.Snapshot{State: proxy.StateStarting, Config: config}, nil)
	server, err := s.serverFactory(config)
	if err != nil {
		s.failStart(config, lifecycleErrorMessage(err, startErrorMessage))
		return err
	}
	if isNilValue(server) {
		s.failStart(config, startErrorMessage)
		return ErrNilServer
	}
	if err := server.Start(); err != nil {
		cleanupContext, cancelCleanup := context.WithTimeout(context.Background(), failedStartCleanupTimeout)
		_ = server.Stop(cleanupContext)
		cancelCleanup()
		s.failStart(config, lifecycleErrorMessage(err, startErrorMessage))
		return err
	}

	address := server.Address()
	s.publish(proxy.Snapshot{
		State:   proxy.StateRunning,
		Config:  config,
		Address: address,
	}, server)
	return nil
}

func (s *Service) Stop(ctx context.Context) error {
	if isNilValue(ctx) {
		return ErrNilStopContext
	}

	s.operationMutex.Lock()
	defer s.operationMutex.Unlock()

	snapshot, server := s.current()
	if snapshot.State != proxy.StateRunning {
		return nil
	}
	if isNilValue(server) {
		s.failStop(snapshot.Config)
		return ErrNilServer
	}

	snapshot.State = proxy.StateStopping
	snapshot.Clients = 0
	snapshot.ErrorMessage = ""
	s.publish(snapshot, server)

	err := server.Stop(ctx)
	if err != nil {
		s.failStop(snapshot.Config)
		return err
	}
	s.publish(proxy.Snapshot{State: proxy.StateStopped, Config: snapshot.Config}, nil)
	return nil
}

func (s *Service) Snapshot() proxy.Snapshot {
	snapshot, server := s.current()
	if !isNilValue(server) {
		snapshot.Clients = server.Clients()
	}
	return snapshot
}

func (s *Service) current() (proxy.Snapshot, Server) {
	s.snapshotMutex.RLock()
	defer s.snapshotMutex.RUnlock()
	return s.snapshot, s.server
}

func (s *Service) publish(snapshot proxy.Snapshot, server Server) {
	s.snapshotMutex.Lock()
	s.snapshot = snapshot
	s.server = server
	s.snapshotMutex.Unlock()
}

func (s *Service) failStart(config proxy.Config, message string) {
	s.publish(proxy.Snapshot{
		State:        proxy.StateError,
		Config:       config,
		ErrorMessage: message,
	}, nil)
}

func (s *Service) failStop(config proxy.Config) {
	s.publish(proxy.Snapshot{
		State:        proxy.StateError,
		Config:       config,
		ErrorMessage: stopErrorMessage,
	}, nil)
}

func lifecycleErrorMessage(err error, fallback string) string {
	if errors.Is(err, syscall.EADDRINUSE) || errors.Is(err, windowsAddressInUseErrno) {
		return portInUseErrorMessage
	}
	if errors.Is(err, os.ErrPermission) {
		return permissionErrorMessage
	}
	for _, validationError := range []error{
		proxy.ErrInterfaceNameRequired,
		proxy.ErrInterfaceAddressInvalid,
		proxy.ErrInterfaceAddressNotPrivate,
		proxy.ErrPortOutOfRange,
		proxy.ErrUsernameRequired,
		proxy.ErrPasswordRequired,
	} {
		if errors.Is(err, validationError) {
			return validationError.Error()
		}
	}
	return fallback
}

func isNilValue(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
