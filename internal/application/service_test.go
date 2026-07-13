package application

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/SgonnovDmGit/LenSA_Proxy/internal/domain/proxy"
)

func TestNewService(t *testing.T) {
	validSource := InterfaceSource(func() ([]proxy.NetworkInterface, error) { return nil, nil })
	validFactory := ServerFactory(func(proxy.Config) (Server, error) { return &fakeServer{}, nil })
	var typedNilSource InterfaceSource
	var typedNilFactory ServerFactory

	tests := []struct {
		name    string
		source  InterfaceSource
		factory ServerFactory
		wantErr error
	}{
		{name: "nil source", factory: validFactory, wantErr: ErrNilInterfaceSource},
		{name: "typed nil source", source: typedNilSource, factory: validFactory, wantErr: ErrNilInterfaceSource},
		{name: "nil factory", source: validSource, wantErr: ErrNilServerFactory},
		{name: "typed nil factory", source: validSource, factory: typedNilFactory, wantErr: ErrNilServerFactory},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, err := NewService(tt.source, tt.factory)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("NewService() error = %v, want %v", err, tt.wantErr)
			}
			if service != nil {
				t.Fatal("NewService() returned a service with invalid dependencies")
			}
		})
	}

	service, err := NewService(validSource, validFactory)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	want := proxy.Snapshot{
		State: proxy.StateStopped,
		Config: proxy.Config{
			Port: proxy.DefaultPort,
		},
	}
	if got := service.Snapshot(); got != want {
		t.Fatalf("initial Snapshot() = %+v, want %+v", got, want)
	}
}

func TestServiceInterfaces(t *testing.T) {
	t.Run("copy", func(t *testing.T) {
		original := []proxy.NetworkInterface{
			{Index: 3, Name: "Ethernet", Address: netip.MustParsePrefix("192.168.1.20/24")},
			{Index: 7, Name: "Wi-Fi", Address: netip.MustParsePrefix("10.0.0.20/8")},
		}
		service := newTestService(t, func() ([]proxy.NetworkInterface, error) {
			return original, nil
		}, func(proxy.Config) (Server, error) {
			return &fakeServer{}, nil
		})

		got, err := service.Interfaces()
		if err != nil {
			t.Fatalf("Interfaces() error = %v", err)
		}
		if !reflect.DeepEqual(got, original) {
			t.Fatalf("Interfaces() = %+v, want %+v", got, original)
		}
		got[0].Name = "changed"
		got = append(got, proxy.NetworkInterface{Name: "extra"})
		if original[0].Name != "Ethernet" || len(original) != 2 {
			t.Fatalf("Interfaces() exposed source slice: %+v", original)
		}
		again, err := service.Interfaces()
		if err != nil {
			t.Fatalf("second Interfaces() error = %v", err)
		}
		if !reflect.DeepEqual(again, original) {
			t.Fatalf("second Interfaces() = %+v, want %+v", again, original)
		}
	})

	t.Run("error", func(t *testing.T) {
		sourceErr := errors.New("interface source failed")
		service := newTestService(t, func() ([]proxy.NetworkInterface, error) {
			return []proxy.NetworkInterface{{Name: "partial"}}, sourceErr
		}, func(proxy.Config) (Server, error) {
			return &fakeServer{}, nil
		})

		interfaces, err := service.Interfaces()
		if !errors.Is(err, sourceErr) {
			t.Fatalf("Interfaces() error = %v, want %v", err, sourceErr)
		}
		if interfaces != nil {
			t.Fatalf("Interfaces() = %+v on error, want nil", interfaces)
		}
	})
}

func TestServiceStartRejectsInvalidConfigWithoutFactory(t *testing.T) {
	var factoryCalls atomic.Int64
	service := newTestService(t, emptyInterfaceSource, func(proxy.Config) (Server, error) {
		factoryCalls.Add(1)
		return &fakeServer{}, nil
	})
	config := proxy.Config{Port: proxy.DefaultPort}

	err := service.Start(config)
	if !errors.Is(err, proxy.ErrInterfaceNameRequired) {
		t.Fatalf("Start() error = %v, want %v", err, proxy.ErrInterfaceNameRequired)
	}
	if got := factoryCalls.Load(); got != 0 {
		t.Fatalf("factory calls = %d, want 0", got)
	}
	assertSnapshot(t, service.Snapshot(), proxy.StateError, config, "", 0, proxy.ErrInterfaceNameRequired.Error())
}

func TestServiceStartPublishesStartingBeforeFactoryWork(t *testing.T) {
	factoryEntered := make(chan proxy.Config, 1)
	factoryRelease := make(chan struct{})
	server := &fakeServer{address: "192.168.1.20:49152"}
	service := newTestService(t, emptyInterfaceSource, func(config proxy.Config) (Server, error) {
		factoryEntered <- config
		<-factoryRelease
		return server, nil
	})
	config := validConfig()
	result := make(chan error, 1)
	go func() { result <- service.Start(config) }()

	if got := receive(t, factoryEntered); got != config {
		t.Fatalf("factory config = %+v, want %+v", got, config)
	}
	assertSnapshot(t, service.Snapshot(), proxy.StateStarting, config, "", 0, "")
	if got := server.startCalls.Load(); got != 0 {
		t.Fatalf("server Start calls while factory blocked = %d, want 0", got)
	}
	close(factoryRelease)
	if err := receive(t, result); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
}

func TestServiceStartSuccessAddressClientsAndSnapshotCopy(t *testing.T) {
	server := &fakeServer{address: "192.168.1.20:49152"}
	server.clients.Store(4)
	var factoryCalls atomic.Int64
	var factoryConfig proxy.Config
	service := newTestService(t, emptyInterfaceSource, func(config proxy.Config) (Server, error) {
		factoryCalls.Add(1)
		factoryConfig = config
		return server, nil
	})
	config := validConfig()

	if err := service.Start(config); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if got := factoryCalls.Load(); got != 1 {
		t.Fatalf("factory calls = %d, want 1", got)
	}
	if factoryConfig != config {
		t.Fatalf("factory config = %+v, want %+v", factoryConfig, config)
	}
	if got := server.startCalls.Load(); got != 1 {
		t.Fatalf("server Start calls = %d, want 1", got)
	}
	assertSnapshot(t, service.Snapshot(), proxy.StateRunning, config, server.address, 4, "")

	server.clients.Store(9)
	snapshot := service.Snapshot()
	if snapshot.Clients != 9 {
		t.Fatalf("dynamic Clients = %d, want 9", snapshot.Clients)
	}
	snapshot.State = proxy.StateError
	snapshot.Config.Port = 1234
	snapshot.Address = "changed"
	snapshot.Clients = -1
	snapshot.ErrorMessage = "changed"
	assertSnapshot(t, service.Snapshot(), proxy.StateRunning, config, server.address, 9, "")
}

func TestServiceStartFactoryError(t *testing.T) {
	factoryErr := errors.New("bind failed with technical-secret")
	service := newTestService(t, emptyInterfaceSource, func(proxy.Config) (Server, error) {
		return nil, factoryErr
	})
	config := validConfig()

	if err := service.Start(config); !errors.Is(err, factoryErr) {
		t.Fatalf("Start() error = %v, want %v", err, factoryErr)
	}
	snapshot := service.Snapshot()
	assertSnapshot(t, snapshot, proxy.StateError, config, "", 0, startErrorMessage)
	if strings.Contains(snapshot.ErrorMessage, "technical-secret") {
		t.Fatalf("unsafe snapshot error = %q", snapshot.ErrorMessage)
	}
}

func TestServiceStartRejectsNilServer(t *testing.T) {
	tests := []struct {
		name   string
		server Server
	}{
		{name: "nil"},
		{name: "typed nil", server: (*fakeServer)(nil)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := newTestService(t, emptyInterfaceSource, func(proxy.Config) (Server, error) {
				return tt.server, nil
			})
			config := validConfig()

			if err := service.Start(config); !errors.Is(err, ErrNilServer) {
				t.Fatalf("Start() error = %v, want %v", err, ErrNilServer)
			}
			assertSnapshot(t, service.Snapshot(), proxy.StateError, config, "", 0, startErrorMessage)
		})
	}
}

func TestServiceStartErrorCleansServer(t *testing.T) {
	startErr := errors.New("listen failed")
	server := &fakeServer{start: func() error { return startErr }}
	service := newTestService(t, emptyInterfaceSource, func(proxy.Config) (Server, error) {
		return server, nil
	})
	config := validConfig()

	if err := service.Start(config); !errors.Is(err, startErr) {
		t.Fatalf("Start() error = %v, want %v", err, startErr)
	}
	if got := server.startCalls.Load(); got != 1 {
		t.Fatalf("server Start calls = %d, want 1", got)
	}
	if got := server.stopCalls.Load(); got != 1 {
		t.Fatalf("cleanup Stop calls = %d, want 1", got)
	}
	assertSnapshot(t, service.Snapshot(), proxy.StateError, config, "", 0, startErrorMessage)
	if got := server.clientCalls.Load(); got != 0 {
		t.Fatalf("failed server Clients calls = %d, want 0", got)
	}
	if err := service.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() after failed Start error = %v", err)
	}
	if got := server.stopCalls.Load(); got != 1 {
		t.Fatalf("Stop calls after failed Start = %d, want 1", got)
	}
}

func TestServiceStartRetryUsesNewServer(t *testing.T) {
	startErr := errors.New("first start failed")
	first := &fakeServer{start: func() error { return startErr }}
	second := &fakeServer{address: "192.168.1.20:49153"}
	var factoryCalls atomic.Int64
	service := newTestService(t, emptyInterfaceSource, func(proxy.Config) (Server, error) {
		if factoryCalls.Add(1) == 1 {
			return first, nil
		}
		return second, nil
	})
	config := validConfig()

	if err := service.Start(config); !errors.Is(err, startErr) {
		t.Fatalf("first Start() error = %v, want %v", err, startErr)
	}
	if err := service.Start(config); err != nil {
		t.Fatalf("retry Start() error = %v", err)
	}
	if got := factoryCalls.Load(); got != 2 {
		t.Fatalf("factory calls = %d, want 2", got)
	}
	if first.startCalls.Load() != 1 || first.stopCalls.Load() != 1 {
		t.Fatalf("first server starts=%d stops=%d, want 1 and 1", first.startCalls.Load(), first.stopCalls.Load())
	}
	if second.startCalls.Load() != 1 {
		t.Fatalf("second server starts=%d, want 1", second.startCalls.Load())
	}
	assertSnapshot(t, service.Snapshot(), proxy.StateRunning, config, second.address, 0, "")
}

func TestServiceRejectsDoubleAndConcurrentStart(t *testing.T) {
	t.Run("running", func(t *testing.T) {
		server := &fakeServer{address: "192.168.1.20:8080"}
		var factoryCalls atomic.Int64
		service := newTestService(t, emptyInterfaceSource, func(proxy.Config) (Server, error) {
			factoryCalls.Add(1)
			return server, nil
		})
		config := validConfig()

		if err := service.Start(config); err != nil {
			t.Fatalf("first Start() error = %v", err)
		}
		if err := service.Start(config); !errors.Is(err, ErrStartNotAllowed) {
			t.Fatalf("second Start() error = %v, want %v", err, ErrStartNotAllowed)
		}
		if factoryCalls.Load() != 1 || server.startCalls.Load() != 1 {
			t.Fatalf("factory calls=%d starts=%d, want 1 and 1", factoryCalls.Load(), server.startCalls.Load())
		}
	})

	t.Run("starting", func(t *testing.T) {
		factoryEntered := make(chan struct{})
		factoryRelease := make(chan struct{})
		var factoryCalls atomic.Int64
		service := newTestService(t, emptyInterfaceSource, func(proxy.Config) (Server, error) {
			factoryCalls.Add(1)
			close(factoryEntered)
			<-factoryRelease
			return &fakeServer{address: "192.168.1.20:8080"}, nil
		})
		config := validConfig()
		firstResult := make(chan error, 1)
		go func() { firstResult <- service.Start(config) }()
		wait(t, factoryEntered)

		secondResult := make(chan error, 1)
		go func() { secondResult <- service.Start(config) }()
		if err := receive(t, secondResult); !errors.Is(err, ErrStartNotAllowed) {
			t.Fatalf("concurrent Start() error = %v, want %v", err, ErrStartNotAllowed)
		}
		if got := factoryCalls.Load(); got != 1 {
			t.Fatalf("factory calls while Starting = %d, want 1", got)
		}
		close(factoryRelease)
		if err := receive(t, firstResult); err != nil {
			t.Fatalf("first Start() error = %v", err)
		}
	})
}

func TestServiceStopBeforeStartAndNilContext(t *testing.T) {
	var factoryCalls atomic.Int64
	service := newTestService(t, emptyInterfaceSource, func(proxy.Config) (Server, error) {
		factoryCalls.Add(1)
		return &fakeServer{}, nil
	})
	var typedNilContext *fakeContext

	for name, ctx := range map[string]context.Context{
		"nil":       nil,
		"typed nil": typedNilContext,
	} {
		t.Run(name, func(t *testing.T) {
			if err := service.Stop(ctx); !errors.Is(err, ErrNilStopContext) {
				t.Fatalf("Stop() error = %v, want %v", err, ErrNilStopContext)
			}
		})
	}
	if err := service.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() before Start error = %v", err)
	}
	if err := service.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop() before Start error = %v", err)
	}
	if got := factoryCalls.Load(); got != 0 {
		t.Fatalf("factory calls = %d, want 0", got)
	}
	want := proxy.Snapshot{State: proxy.StateStopped, Config: proxy.Config{Port: proxy.DefaultPort}}
	if got := service.Snapshot(); got != want {
		t.Fatalf("Snapshot() = %+v, want %+v", got, want)
	}
}

func TestServiceStopPublishesStoppingThenStopped(t *testing.T) {
	stopEntered := make(chan struct{})
	stopRelease := make(chan struct{})
	server := &fakeServer{
		address: "192.168.1.20:49152",
		stop: func(context.Context) error {
			close(stopEntered)
			<-stopRelease
			return nil
		},
	}
	server.clients.Store(6)
	service := newTestService(t, emptyInterfaceSource, func(proxy.Config) (Server, error) {
		return server, nil
	})
	config := validConfig()
	if err := service.Start(config); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	result := make(chan error, 1)
	go func() { result <- service.Stop(context.Background()) }()
	wait(t, stopEntered)
	assertSnapshot(t, service.Snapshot(), proxy.StateStopping, config, server.address, 6, "")
	close(stopRelease)
	if err := receive(t, result); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	assertSnapshot(t, service.Snapshot(), proxy.StateStopped, config, "", 0, "")
	if got := server.stopCalls.Load(); got != 1 {
		t.Fatalf("server Stop calls = %d, want 1", got)
	}
}

func TestServiceStopErrorPublishesSafeErrorAndClearsServer(t *testing.T) {
	stopErr := errors.New("shutdown failed with technical-secret")
	server := &fakeServer{
		address: "192.168.1.20:49152",
		stop:    func(context.Context) error { return stopErr },
	}
	server.clients.Store(3)
	service := newTestService(t, emptyInterfaceSource, func(proxy.Config) (Server, error) {
		return server, nil
	})
	config := validConfig()
	if err := service.Start(config); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if err := service.Stop(context.Background()); !errors.Is(err, stopErr) {
		t.Fatalf("Stop() error = %v, want %v", err, stopErr)
	}
	clientCalls := server.clientCalls.Load()
	snapshot := service.Snapshot()
	assertSnapshot(t, snapshot, proxy.StateError, config, "", 0, stopErrorMessage)
	if strings.Contains(snapshot.ErrorMessage, "technical-secret") {
		t.Fatalf("unsafe snapshot error = %q", snapshot.ErrorMessage)
	}
	if got := server.clientCalls.Load(); got != clientCalls {
		t.Fatalf("cleared server Clients calls changed from %d to %d", clientCalls, got)
	}
	if err := service.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop() error = %v", err)
	}
	if got := server.stopCalls.Load(); got != 1 {
		t.Fatalf("server Stop calls = %d, want 1", got)
	}
}

func TestServiceStopDuringStartingWaitsAndStopsServer(t *testing.T) {
	startEntered := make(chan struct{})
	startRelease := make(chan struct{})
	server := &fakeServer{
		address: "192.168.1.20:49152",
		start: func() error {
			close(startEntered)
			<-startRelease
			return nil
		},
	}
	service := newTestService(t, emptyInterfaceSource, func(proxy.Config) (Server, error) {
		return server, nil
	})
	config := validConfig()
	startResult := make(chan error, 1)
	go func() { startResult <- service.Start(config) }()
	wait(t, startEntered)
	assertSnapshot(t, service.Snapshot(), proxy.StateStarting, config, "", 0, "")

	stopResult := make(chan error, 1)
	go func() { stopResult <- service.Stop(context.Background()) }()
	assertBlocked(t, stopResult)
	if got := server.stopCalls.Load(); got != 0 {
		t.Fatalf("server Stop calls while Start blocked = %d, want 0", got)
	}
	close(startRelease)
	if err := receive(t, startResult); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := receive(t, stopResult); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if server.startCalls.Load() != 1 || server.stopCalls.Load() != 1 {
		t.Fatalf("server starts=%d stops=%d, want 1 and 1", server.startCalls.Load(), server.stopCalls.Load())
	}
	assertSnapshot(t, service.Snapshot(), proxy.StateStopped, config, "", 0, "")
}

func TestServiceStartDuringStoppingIsRejectedWithoutRestart(t *testing.T) {
	stopEntered := make(chan struct{})
	stopRelease := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(stopRelease) })
	server := &fakeServer{
		address: "192.168.1.20:49152",
		stop: func(context.Context) error {
			close(stopEntered)
			<-stopRelease
			return nil
		},
	}
	var factoryCalls atomic.Int64
	service := newTestService(t, emptyInterfaceSource, func(proxy.Config) (Server, error) {
		factoryCalls.Add(1)
		return server, nil
	})
	config := validConfig()
	if err := service.Start(config); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	stopResult := make(chan error, 1)
	go func() { stopResult <- service.Stop(context.Background()) }()
	wait(t, stopEntered)
	if state := service.Snapshot().State; state != proxy.StateStopping {
		t.Fatalf("state while Stop blocked = %v, want %v", state, proxy.StateStopping)
	}

	startResult := make(chan error, 1)
	go func() { startResult <- service.Start(config) }()
	if err := receive(t, startResult); !errors.Is(err, ErrStartNotAllowed) {
		t.Fatalf("Start() during Stopping error = %v, want %v", err, ErrStartNotAllowed)
	}
	if got := factoryCalls.Load(); got != 1 {
		t.Fatalf("factory calls before Stop release = %d, want 1", got)
	}
	releaseOnce.Do(func() { close(stopRelease) })
	if err := receive(t, stopResult); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if got := factoryCalls.Load(); got != 1 {
		t.Fatalf("factory calls after Stop = %d, want 1", got)
	}
	assertSnapshot(t, service.Snapshot(), proxy.StateStopped, config, "", 0, "")
}

func TestLifecycleErrorMessage(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		fallback string
		want     string
	}{
		{name: "port in use", err: errors.Join(errors.New("listen failed"), syscall.EADDRINUSE), fallback: "fallback", want: portInUseErrorMessage},
		{name: "permission", err: errors.Join(errors.New("listen failed"), os.ErrPermission), fallback: "fallback", want: permissionErrorMessage},
		{name: "validation", err: proxy.ErrPasswordRequired, fallback: "fallback", want: proxy.ErrPasswordRequired.Error()},
		{name: "unknown", err: errors.New("technical-secret"), fallback: "fallback", want: "fallback"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := lifecycleErrorMessage(tt.err, tt.fallback); got != tt.want {
				t.Fatalf("lifecycleErrorMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLifecycleErrorMessageForOccupiedPort(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	defer listener.Close()

	conflict, err := net.Listen("tcp4", listener.Addr().String())
	if err == nil {
		conflict.Close()
		t.Fatal("second net.Listen() succeeded, want occupied-port error")
	}
	if got := lifecycleErrorMessage(err, "fallback"); got != portInUseErrorMessage {
		t.Fatalf("lifecycleErrorMessage() = %q, want %q; listen error = %v", got, portInUseErrorMessage, err)
	}
}

func TestServiceConcurrentSnapshot(t *testing.T) {
	stopEntered := make(chan struct{})
	stopRelease := make(chan struct{})
	server := &fakeServer{
		address: "192.168.1.20:49152",
		stop: func(context.Context) error {
			close(stopEntered)
			<-stopRelease
			return nil
		},
	}
	service := newTestService(t, emptyInterfaceSource, func(proxy.Config) (Server, error) {
		return server, nil
	})
	config := validConfig()
	if err := service.Start(config); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	stopResult := make(chan error, 1)
	go func() { stopResult <- service.Stop(context.Background()) }()
	wait(t, stopEntered)

	const goroutines = 16
	const iterations = 2000
	var readers sync.WaitGroup
	var invalid atomic.Bool
	readers.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(seed int) {
			defer readers.Done()
			for j := 0; j < iterations; j++ {
				server.clients.Store(int64(seed + j))
				snapshot := service.Snapshot()
				if snapshot.State != proxy.StateStopping && snapshot.State != proxy.StateStopped {
					invalid.Store(true)
				}
				snapshot.Address = "mutated"
			}
		}(i)
	}
	close(stopRelease)
	if err := receive(t, stopResult); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	readers.Wait()
	if invalid.Load() {
		t.Fatal("Snapshot() returned an invalid state during concurrent access")
	}
	assertSnapshot(t, service.Snapshot(), proxy.StateStopped, config, "", 0, "")
}

type fakeServer struct {
	address     string
	start       func() error
	stop        func(context.Context) error
	clients     atomic.Int64
	startCalls  atomic.Int64
	stopCalls   atomic.Int64
	clientCalls atomic.Int64
}

func (s *fakeServer) Start() error {
	s.startCalls.Add(1)
	if s.start != nil {
		return s.start()
	}
	return nil
}

func (s *fakeServer) Stop(ctx context.Context) error {
	s.stopCalls.Add(1)
	if s.stop != nil {
		return s.stop(ctx)
	}
	return nil
}

func (s *fakeServer) Address() string {
	return s.address
}

func (s *fakeServer) Clients() int {
	s.clientCalls.Add(1)
	return int(s.clients.Load())
}

type fakeContext struct{}

func (*fakeContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (*fakeContext) Done() <-chan struct{}       { return nil }
func (*fakeContext) Err() error                  { return nil }
func (*fakeContext) Value(any) any               { return nil }

func validConfig() proxy.Config {
	return proxy.Config{
		Interface: proxy.NetworkInterface{
			Index:   3,
			Name:    "Ethernet",
			Address: netip.MustParsePrefix("192.168.1.20/24"),
		},
		Port: proxy.DefaultPort,
	}
}

func emptyInterfaceSource() ([]proxy.NetworkInterface, error) {
	return nil, nil
}

func newTestService(t *testing.T, source InterfaceSource, factory ServerFactory) *Service {
	t.Helper()
	service, err := NewService(source, factory)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service
}

func assertSnapshot(t *testing.T, got proxy.Snapshot, state proxy.State, config proxy.Config, address string, clients int, errorMessage string) {
	t.Helper()
	want := proxy.Snapshot{
		State:        state,
		Config:       config,
		Address:      address,
		Clients:      clients,
		ErrorMessage: errorMessage,
	}
	if got != want {
		t.Fatalf("Snapshot() = %+v, want %+v", got, want)
	}
}

func wait(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for signal")
	}
}

func receive[T any](t *testing.T, values <-chan T) T {
	t.Helper()
	select {
	case value := <-values:
		return value
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for value")
		var zero T
		return zero
	}
}

func assertBlocked(t *testing.T, result <-chan error) {
	t.Helper()
	select {
	case err := <-result:
		t.Fatalf("operation returned while expected to block: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
}
