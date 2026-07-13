package network

import (
	"errors"
	"io"
	"net"
	"net/netip"
	"sync"
	"testing"
	"time"
)

func TestManagedListenerFiltersSourcesAfterAccept(t *testing.T) {
	inner := newManagedTestListener()
	denied := newManagedTestConn(&net.TCPAddr{IP: net.ParseIP("192.168.2.10"), Port: 1000})
	malformed := newManagedTestConn(managedTestAddr("not-an-address"))
	allowed := newManagedTestConn(&net.TCPAddr{IP: net.ParseIP("192.168.1.10"), Port: 1001})
	inner.queue(denied, nil)
	inner.queue(malformed, nil)
	inner.queue(allowed, nil)
	listener := newManagedListener(inner, netip.MustParsePrefix("192.168.1.0/24"), 1, time.Minute)
	managedTestCleanup(t, listener)

	connection, err := listener.Accept()
	if err != nil {
		t.Fatalf("Accept() error = %v", err)
	}
	if connection.RemoteAddr() != allowed.RemoteAddr() {
		t.Fatalf("RemoteAddr() = %v, want %v", connection.RemoteAddr(), allowed.RemoteAddr())
	}
	if inner.acceptCallCount() != 3 {
		t.Fatalf("inner Accept() calls = %d, want 3", inner.acceptCallCount())
	}
	if denied.closeCallCount() != 1 || malformed.closeCallCount() != 1 {
		t.Fatalf("filtered Close() calls = %d, %d, want 1, 1", denied.closeCallCount(), malformed.closeCallCount())
	}
	if allowed.closeCallCount() != 0 {
		t.Fatalf("allowed Close() calls = %d, want 0", allowed.closeCallCount())
	}
	if listener.ConnectionCount() != 1 || listener.ClientCount() != 1 {
		t.Fatalf("counts = %d connections, %d clients, want 1, 1", listener.ConnectionCount(), listener.ClientCount())
	}
	if listener.Addr().String() != inner.Addr().String() {
		t.Fatalf("Addr() = %v, want %v", listener.Addr(), inner.Addr())
	}

	if err := connection.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if listener.ConnectionCount() != 0 || listener.ClientCount() != 0 {
		t.Fatalf("counts after Close() = %d connections, %d clients", listener.ConnectionCount(), listener.ClientCount())
	}
}

func TestManagedListenerTracksNormalizedUniqueClients(t *testing.T) {
	inner := newManagedTestListener()
	first := newManagedTestConn(&net.TCPAddr{IP: net.IP{192, 168, 1, 10}, Port: 1000})
	second := newManagedTestConn(&net.TCPAddr{IP: net.ParseIP("192.168.1.10"), Port: 1001})
	third := newManagedTestConn(managedTestAddr("192.168.1.11:1002"))
	inner.queue(first, nil)
	inner.queue(second, nil)
	inner.queue(third, nil)
	listener := newManagedListener(inner, netip.MustParsePrefix("192.168.1.0/24"), 3, 0)
	managedTestCleanup(t, listener)

	connections := make([]net.Conn, 3)
	for index := range connections {
		connection, err := listener.Accept()
		if err != nil {
			t.Fatalf("Accept() %d error = %v", index, err)
		}
		connections[index] = connection
	}
	if listener.ConnectionCount() != 3 || listener.ClientCount() != 2 {
		t.Fatalf("counts = %d connections, %d clients, want 3, 2", listener.ConnectionCount(), listener.ClientCount())
	}

	if err := connections[0].Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}
	if err := connections[0].Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if first.closeCallCount() != 1 {
		t.Fatalf("underlying Close() calls = %d, want 1", first.closeCallCount())
	}
	if listener.ConnectionCount() != 2 || listener.ClientCount() != 2 {
		t.Fatalf("counts after first close = %d connections, %d clients, want 2, 2", listener.ConnectionCount(), listener.ClientCount())
	}

	if err := connections[1].Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if listener.ConnectionCount() != 1 || listener.ClientCount() != 1 {
		t.Fatalf("counts after second close = %d connections, %d clients, want 1, 1", listener.ConnectionCount(), listener.ClientCount())
	}
	if err := connections[2].Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if listener.ConnectionCount() != 0 || listener.ClientCount() != 0 {
		t.Fatalf("final counts = %d connections, %d clients", listener.ConnectionCount(), listener.ClientCount())
	}
}

func TestManagedListenerIgnoresClientAddressZones(t *testing.T) {
	inner := newManagedTestListener()
	first := newManagedTestConn(managedTestAddr("[fd00::10%zone-a]:1000"))
	second := newManagedTestConn(managedTestAddr("[fd00::10%zone-b]:1001"))
	inner.queue(first, nil)
	inner.queue(second, nil)
	listener := newManagedListener(inner, netip.MustParsePrefix("fd00::/64"), 2, 0)
	managedTestCleanup(t, listener)

	firstConnection, err := listener.Accept()
	if err != nil {
		t.Fatalf("first Accept() error = %v", err)
	}
	secondConnection, err := listener.Accept()
	if err != nil {
		t.Fatalf("second Accept() error = %v", err)
	}
	if listener.ClientCount() != 1 || listener.ConnectionCount() != 2 {
		t.Fatalf("counts = %d clients, %d connections, want 1, 2", listener.ClientCount(), listener.ConnectionCount())
	}
	_ = firstConnection.Close()
	_ = secondConnection.Close()
}

func TestManagedListenerReleasesCapacityAfterAcceptError(t *testing.T) {
	inner := newManagedTestListener()
	acceptErr := errors.New("accept failed")
	inner.queue(nil, acceptErr)
	listener := newManagedListener(inner, netip.MustParsePrefix("192.168.1.0/24"), 1, 0)
	managedTestCleanup(t, listener)

	connection, err := listener.Accept()
	if connection != nil || !errors.Is(err, acceptErr) {
		t.Fatalf("Accept() = %v, %v, want nil, %v", connection, err, acceptErr)
	}

	allowed := newManagedTestConn(&net.TCPAddr{IP: net.ParseIP("192.168.1.20"), Port: 1000})
	inner.queue(allowed, nil)
	connection, err = listener.Accept()
	if err != nil {
		t.Fatalf("second Accept() error = %v", err)
	}
	_ = connection.Close()
}

func TestManagedListenerConnectionLimitBlocksAndResumes(t *testing.T) {
	inner := newManagedTestListener()
	first := newManagedTestConn(&net.TCPAddr{IP: net.ParseIP("192.168.1.10"), Port: 1000})
	second := newManagedTestConn(&net.TCPAddr{IP: net.ParseIP("192.168.1.11"), Port: 1001})
	inner.queue(first, nil)
	inner.queue(second, nil)
	listener := newManagedListener(inner, netip.MustParsePrefix("192.168.1.0/24"), 1, 0)
	managedTestCleanup(t, listener)

	firstConnection, err := listener.Accept()
	if err != nil {
		t.Fatalf("first Accept() error = %v", err)
	}
	secondResult := managedTestAcceptAsync(listener)
	managedTestAssertAcceptBlocked(t, secondResult)
	if inner.acceptCallCount() != 1 {
		t.Fatalf("inner Accept() calls while full = %d, want 1", inner.acceptCallCount())
	}
	if listener.ConnectionCount() != 1 {
		t.Fatalf("ConnectionCount() while full = %d, want 1", listener.ConnectionCount())
	}

	if err := firstConnection.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}
	result := managedTestAwaitAccept(t, secondResult)
	if result.err != nil {
		t.Fatalf("second Accept() error = %v", result.err)
	}
	if listener.ConnectionCount() != 1 {
		t.Fatalf("ConnectionCount() after resume = %d, want 1", listener.ConnectionCount())
	}
	_ = result.connection.Close()
}

func TestManagedListenerCloseUnblocksInnerAccept(t *testing.T) {
	inner := newManagedTestListener()
	listener := newManagedListener(inner, netip.MustParsePrefix("192.168.1.0/24"), 1, 0)
	managedTestCleanup(t, listener)

	resultChannel := managedTestAcceptAsync(listener)
	inner.awaitAcceptStart(t)
	if err := listener.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	result := managedTestAwaitAccept(t, resultChannel)
	if result.connection != nil || !errors.Is(result.err, net.ErrClosed) {
		t.Fatalf("Accept() = %v, %v, want nil, %v", result.connection, result.err, net.ErrClosed)
	}
	if inner.closeCallCount() != 1 {
		t.Fatalf("inner Close() calls = %d, want 1", inner.closeCallCount())
	}
}

func TestManagedListenerCloseUnblocksCapacityWait(t *testing.T) {
	inner := newManagedTestListener()
	first := newManagedTestConn(&net.TCPAddr{IP: net.ParseIP("192.168.1.10"), Port: 1000})
	second := newManagedTestConn(&net.TCPAddr{IP: net.ParseIP("192.168.1.11"), Port: 1001})
	inner.queue(first, nil)
	inner.queue(second, nil)
	listener := newManagedListener(inner, netip.MustParsePrefix("192.168.1.0/24"), 1, 0)
	managedTestCleanup(t, listener)

	firstConnection, err := listener.Accept()
	if err != nil {
		t.Fatalf("first Accept() error = %v", err)
	}
	resultChannel := managedTestAcceptAsync(listener)
	managedTestAssertAcceptBlocked(t, resultChannel)
	if inner.acceptCallCount() != 1 {
		t.Fatalf("inner Accept() calls while full = %d, want 1", inner.acceptCallCount())
	}

	if err := listener.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	result := managedTestAwaitAccept(t, resultChannel)
	if result.connection != nil || !errors.Is(result.err, net.ErrClosed) {
		t.Fatalf("Accept() = %v, %v, want nil, %v", result.connection, result.err, net.ErrClosed)
	}
	if inner.acceptCallCount() != 1 {
		t.Fatalf("inner Accept() calls after close = %d, want 1", inner.acceptCallCount())
	}
	if listener.ConnectionCount() != 1 || listener.ClientCount() != 1 {
		t.Fatalf("active counts after listener close = %d, %d, want 1, 1", listener.ConnectionCount(), listener.ClientCount())
	}
	_ = firstConnection.Close()
}

func TestManagedListenerCloseConnections(t *testing.T) {
	inner := newManagedTestListener()
	underlying := []*managedTestConn{
		newManagedTestConn(&net.TCPAddr{IP: net.ParseIP("192.168.1.10"), Port: 1000}),
		newManagedTestConn(&net.TCPAddr{IP: net.ParseIP("192.168.1.10"), Port: 1001}),
		newManagedTestConn(&net.TCPAddr{IP: net.ParseIP("192.168.1.11"), Port: 1002}),
	}
	for _, connection := range underlying {
		inner.queue(connection, nil)
	}
	listener := newManagedListener(inner, netip.MustParsePrefix("192.168.1.0/24"), 3, 0)
	managedTestCleanup(t, listener)

	connections := make([]net.Conn, len(underlying))
	closed := make(chan struct{}, len(underlying))
	for index, connection := range underlying {
		connection.setOnClose(func() {
			_ = listener.ConnectionCount()
			closed <- struct{}{}
		})
		accepted, err := listener.Accept()
		if err != nil {
			t.Fatalf("Accept() %d error = %v", index, err)
		}
		connections[index] = accepted
	}

	done := make(chan struct{})
	go func() {
		listener.CloseConnections()
		close(done)
	}()
	managedTestAwaitSignal(t, done, "CloseConnections")
	for range underlying {
		managedTestAwaitSignal(t, closed, "underlying Close")
	}
	if listener.ConnectionCount() != 0 || listener.ClientCount() != 0 {
		t.Fatalf("counts = %d connections, %d clients, want 0, 0", listener.ConnectionCount(), listener.ClientCount())
	}
	for index, connection := range underlying {
		if connection.closeCallCount() != 1 {
			t.Fatalf("underlying %d Close() calls = %d, want 1", index, connection.closeCallCount())
		}
	}

	listener.CloseConnections()
	for _, connection := range connections {
		_ = connection.Close()
	}
	for index, connection := range underlying {
		if connection.closeCallCount() != 1 {
			t.Fatalf("underlying %d Close() calls after repeated close = %d, want 1", index, connection.closeCallCount())
		}
	}

	next := newManagedTestConn(&net.TCPAddr{IP: net.ParseIP("192.168.1.12"), Port: 1003})
	inner.queue(next, nil)
	nextConnection, err := listener.Accept()
	if err != nil {
		t.Fatalf("Accept() after CloseConnections error = %v", err)
	}
	_ = nextConnection.Close()
}

func TestManagedConnectionRefreshesDirectionalDeadlines(t *testing.T) {
	inner := newManagedTestListener()
	underlying := newManagedTestConn(&net.TCPAddr{IP: net.ParseIP("192.168.1.10"), Port: 1000})
	underlying.readData = []byte("read")
	inner.queue(underlying, nil)
	timeout := time.Minute
	listener := newManagedListener(inner, netip.MustParsePrefix("192.168.1.0/24"), 1, timeout)
	managedTestCleanup(t, listener)

	connection, err := listener.Accept()
	if err != nil {
		t.Fatalf("Accept() error = %v", err)
	}
	readStart := time.Now()
	buffer := make([]byte, 4)
	read, err := connection.Read(buffer)
	readEnd := time.Now()
	if err != nil || read != 4 || string(buffer) != "read" {
		t.Fatalf("Read() = %d, %v, %q", read, err, buffer)
	}
	readDeadlines, writeDeadlines := underlying.deadlines()
	if len(readDeadlines) != 1 || len(writeDeadlines) != 0 {
		t.Fatalf("deadline calls after Read() = %d read, %d write, want 1, 0", len(readDeadlines), len(writeDeadlines))
	}
	if readDeadlines[0].Before(readStart.Add(timeout)) || readDeadlines[0].After(readEnd.Add(timeout)) {
		t.Fatalf("read deadline = %v, want between %v and %v", readDeadlines[0], readStart.Add(timeout), readEnd.Add(timeout))
	}

	writeStart := time.Now()
	written, err := connection.Write([]byte("write"))
	writeEnd := time.Now()
	if err != nil || written != 5 {
		t.Fatalf("Write() = %d, %v", written, err)
	}
	readDeadlines, writeDeadlines = underlying.deadlines()
	if len(readDeadlines) != 1 || len(writeDeadlines) != 1 {
		t.Fatalf("deadline calls after Write() = %d read, %d write, want 1, 1", len(readDeadlines), len(writeDeadlines))
	}
	if writeDeadlines[0].Before(writeStart.Add(timeout)) || writeDeadlines[0].After(writeEnd.Add(timeout)) {
		t.Fatalf("write deadline = %v, want between %v and %v", writeDeadlines[0], writeStart.Add(timeout), writeEnd.Add(timeout))
	}
	if string(underlying.writtenBytes()) != "write" {
		t.Fatalf("written bytes = %q, want %q", underlying.writtenBytes(), "write")
	}
	_ = connection.Close()
}

func TestManagedConnectionPreservesExplicitDeadlines(t *testing.T) {
	inner := newManagedTestListener()
	underlying := newManagedTestConn(&net.TCPAddr{IP: net.ParseIP("192.168.1.10"), Port: 1000})
	underlying.readData = []byte("read")
	inner.queue(underlying, nil)
	listener := newManagedListener(inner, netip.MustParsePrefix("192.168.1.0/24"), 1, time.Minute)
	managedTestCleanup(t, listener)

	connection, err := listener.Accept()
	if err != nil {
		t.Fatalf("Accept() error = %v", err)
	}
	explicit := time.Unix(1, 0)
	if err := connection.SetReadDeadline(explicit); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	if _, err := connection.Read(make([]byte, 4)); err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if err := connection.SetWriteDeadline(explicit); err != nil {
		t.Fatalf("SetWriteDeadline() error = %v", err)
	}
	if _, err := connection.Write([]byte("write")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	readDeadlines, writeDeadlines := underlying.deadlines()
	if len(readDeadlines) != 1 || readDeadlines[0] != explicit {
		t.Fatalf("read deadlines = %v, want only %v", readDeadlines, explicit)
	}
	if len(writeDeadlines) != 1 || writeDeadlines[0] != explicit {
		t.Fatalf("write deadlines = %v, want only %v", writeDeadlines, explicit)
	}
	_ = connection.Close()
}

func TestManagedConnectionForwardsSupportedHalfClose(t *testing.T) {
	inner := newManagedTestListener()
	supported := newManagedTestHalfConn(&net.TCPAddr{IP: net.ParseIP("192.168.1.10"), Port: 1000})
	unsupported := newManagedTestConn(&net.TCPAddr{IP: net.ParseIP("192.168.1.11"), Port: 1001})
	inner.queue(supported, nil)
	inner.queue(unsupported, nil)
	listener := newManagedListener(inner, netip.MustParsePrefix("192.168.1.0/24"), 2, 0)
	managedTestCleanup(t, listener)

	supportedConnection, err := listener.Accept()
	if err != nil {
		t.Fatalf("first Accept() error = %v", err)
	}
	halfClose, ok := supportedConnection.(halfCloseConn)
	if !ok {
		t.Fatal("accepted connection does not support half-close")
	}
	if err := halfClose.CloseRead(); err != nil {
		t.Fatalf("CloseRead() error = %v", err)
	}
	if err := halfClose.CloseWrite(); err != nil {
		t.Fatalf("CloseWrite() error = %v", err)
	}
	if supported.closeReadCallCount() != 1 || supported.closeWriteCallCount() != 1 {
		t.Fatalf("half-close calls = %d read, %d write, want 1, 1", supported.closeReadCallCount(), supported.closeWriteCallCount())
	}

	unsupportedConnection, err := listener.Accept()
	if err != nil {
		t.Fatalf("second Accept() error = %v", err)
	}
	if _, ok := unsupportedConnection.(halfCloseConn); ok {
		t.Fatal("accepted connection unexpectedly supports half-close")
	}
	_ = supportedConnection.Close()
	_ = unsupportedConnection.Close()
}

func TestManagedListenerConcurrentCloseIsRaceSafe(t *testing.T) {
	const connectionCount = 8
	inner := newManagedTestListener()
	underlying := make([]*managedTestConn, connectionCount)
	connections := make([]net.Conn, connectionCount)
	for index := range underlying {
		underlying[index] = newManagedTestConn(&net.TCPAddr{IP: net.ParseIP("192.168.1.10"), Port: 1000 + index})
		inner.queue(underlying[index], nil)
	}
	listener := newManagedListener(inner, netip.MustParsePrefix("192.168.1.0/24"), connectionCount, 0)
	managedTestCleanup(t, listener)
	for index := range connections {
		connection, err := listener.Accept()
		if err != nil {
			t.Fatalf("Accept() %d error = %v", index, err)
		}
		connections[index] = connection
	}

	start := make(chan struct{})
	var wait sync.WaitGroup
	for index := 0; index < connectionCount*4; index++ {
		connection := connections[index%len(connections)]
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_ = connection.Close()
		}()
	}
	for index := 0; index < 4; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			listener.CloseConnections()
		}()
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_ = listener.Close()
		}()
	}
	close(start)
	finished := make(chan struct{})
	go func() {
		wait.Wait()
		close(finished)
	}()
	managedTestAwaitSignal(t, finished, "concurrent closes")

	if listener.ConnectionCount() != 0 || listener.ClientCount() != 0 {
		t.Fatalf("counts = %d connections, %d clients, want 0, 0", listener.ConnectionCount(), listener.ClientCount())
	}
	for index, connection := range underlying {
		if connection.closeCallCount() != 1 {
			t.Fatalf("underlying %d Close() calls = %d, want 1", index, connection.closeCallCount())
		}
	}
	if inner.closeCallCount() != 1 {
		t.Fatalf("inner Close() calls = %d, want 1", inner.closeCallCount())
	}
}

type managedTestAcceptResult struct {
	connection net.Conn
	err        error
}

type managedTestListener struct {
	results       chan managedTestAcceptResult
	acceptStarted chan struct{}
	closed        chan struct{}
	closeOnce     sync.Once
	mutex         sync.Mutex
	acceptCalls   int
	closeCalls    int
	address       net.Addr
}

func newManagedTestListener() *managedTestListener {
	return &managedTestListener{
		results:       make(chan managedTestAcceptResult, 64),
		acceptStarted: make(chan struct{}, 64),
		closed:        make(chan struct{}),
		address:       managedTestAddr("127.0.0.1:8080"),
	}
}

func (l *managedTestListener) Accept() (net.Conn, error) {
	l.mutex.Lock()
	l.acceptCalls++
	l.mutex.Unlock()
	l.acceptStarted <- struct{}{}
	select {
	case <-l.closed:
		return nil, net.ErrClosed
	default:
	}
	select {
	case result := <-l.results:
		return result.connection, result.err
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func (l *managedTestListener) Close() error {
	l.mutex.Lock()
	l.closeCalls++
	l.mutex.Unlock()
	l.closeOnce.Do(func() {
		close(l.closed)
	})
	return nil
}

func (l *managedTestListener) Addr() net.Addr {
	return l.address
}

func (l *managedTestListener) queue(connection net.Conn, err error) {
	l.results <- managedTestAcceptResult{connection: connection, err: err}
}

func (l *managedTestListener) acceptCallCount() int {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.acceptCalls
}

func (l *managedTestListener) closeCallCount() int {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.closeCalls
}

func (l *managedTestListener) awaitAcceptStart(t *testing.T) {
	t.Helper()
	managedTestAwaitSignal(t, l.acceptStarted, "inner Accept")
}

type managedTestAddr string

func (a managedTestAddr) Network() string {
	return "tcp"
}

func (a managedTestAddr) String() string {
	return string(a)
}

type managedTestConn struct {
	mutex          sync.Mutex
	remote         net.Addr
	readData       []byte
	written        []byte
	closeCalls     int
	readDeadlines  []time.Time
	writeDeadlines []time.Time
	onClose        func()
}

func newManagedTestConn(remote net.Addr) *managedTestConn {
	return &managedTestConn{remote: remote}
}

func (c *managedTestConn) Read(buffer []byte) (int, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if len(c.readData) == 0 {
		return 0, io.EOF
	}
	read := copy(buffer, c.readData)
	c.readData = c.readData[read:]
	return read, nil
}

func (c *managedTestConn) Write(buffer []byte) (int, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.written = append(c.written, buffer...)
	return len(buffer), nil
}

func (c *managedTestConn) Close() error {
	c.mutex.Lock()
	c.closeCalls++
	onClose := c.onClose
	c.mutex.Unlock()
	if onClose != nil {
		onClose()
	}
	return nil
}

func (c *managedTestConn) LocalAddr() net.Addr {
	return managedTestAddr("127.0.0.1:8080")
}

func (c *managedTestConn) RemoteAddr() net.Addr {
	return c.remote
}

func (c *managedTestConn) SetDeadline(deadline time.Time) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.readDeadlines = append(c.readDeadlines, deadline)
	c.writeDeadlines = append(c.writeDeadlines, deadline)
	return nil
}

func (c *managedTestConn) SetReadDeadline(deadline time.Time) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.readDeadlines = append(c.readDeadlines, deadline)
	return nil
}

func (c *managedTestConn) SetWriteDeadline(deadline time.Time) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.writeDeadlines = append(c.writeDeadlines, deadline)
	return nil
}

func (c *managedTestConn) closeCallCount() int {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.closeCalls
}

func (c *managedTestConn) deadlines() ([]time.Time, []time.Time) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return append([]time.Time(nil), c.readDeadlines...), append([]time.Time(nil), c.writeDeadlines...)
}

func (c *managedTestConn) writtenBytes() []byte {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return append([]byte(nil), c.written...)
}

func (c *managedTestConn) setOnClose(onClose func()) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.onClose = onClose
}

type managedTestHalfConn struct {
	*managedTestConn
	mutex           sync.Mutex
	closeReadCalls  int
	closeWriteCalls int
}

func newManagedTestHalfConn(remote net.Addr) *managedTestHalfConn {
	return &managedTestHalfConn{managedTestConn: newManagedTestConn(remote)}
}

func (c *managedTestHalfConn) CloseRead() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.closeReadCalls++
	return nil
}

func (c *managedTestHalfConn) CloseWrite() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.closeWriteCalls++
	return nil
}

func (c *managedTestHalfConn) closeReadCallCount() int {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.closeReadCalls
}

func (c *managedTestHalfConn) closeWriteCallCount() int {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.closeWriteCalls
}

func managedTestCleanup(t *testing.T, listener *managedListener) {
	t.Helper()
	t.Cleanup(func() {
		_ = listener.Close()
		listener.CloseConnections()
	})
}

func managedTestAcceptAsync(listener net.Listener) <-chan managedTestAcceptResult {
	result := make(chan managedTestAcceptResult, 1)
	go func() {
		connection, err := listener.Accept()
		result <- managedTestAcceptResult{connection: connection, err: err}
	}()
	return result
}

func managedTestAwaitAccept(t *testing.T, result <-chan managedTestAcceptResult) managedTestAcceptResult {
	t.Helper()
	select {
	case accepted := <-result:
		return accepted
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Accept()")
		return managedTestAcceptResult{}
	}
}

func managedTestAssertAcceptBlocked(t *testing.T, result <-chan managedTestAcceptResult) {
	t.Helper()
	select {
	case accepted := <-result:
		if accepted.connection != nil {
			_ = accepted.connection.Close()
		}
		t.Fatalf("Accept() completed while capacity was full: %v", accepted.err)
	case <-time.After(20 * time.Millisecond):
	}
}

func managedTestAwaitSignal(t *testing.T, signal <-chan struct{}, operation string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s", operation)
	}
}
