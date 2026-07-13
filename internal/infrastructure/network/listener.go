package network

import (
	"net"
	"net/netip"
	"sync"
	"time"
)

type managedListener struct {
	inner         net.Listener
	allowedSubnet netip.Prefix
	idleTimeout   time.Duration
	capacity      chan struct{}
	done          chan struct{}
	closeOnce     sync.Once
	closeErr      error
	mutex         sync.Mutex
	closed        bool
	connections   map[*managedConn]struct{}
	clients       map[netip.Addr]int
}

type managedConn struct {
	net.Conn
	listener      *managedListener
	source        netip.Addr
	idleTimeout   time.Duration
	deadlineMutex sync.Mutex
	readDeadline  time.Time
	writeDeadline time.Time
	closeOnce     sync.Once
	closeErr      error
}

type halfCloseConn interface {
	CloseRead() error
	CloseWrite() error
}

type managedHalfCloseConn struct {
	*managedConn
	halfClose halfCloseConn
}

func newManagedListener(inner net.Listener, allowedSubnet netip.Prefix, maxConnections int, idleTimeout time.Duration) *managedListener {
	if maxConnections < 0 {
		maxConnections = 0
	}
	return &managedListener{
		inner:         inner,
		allowedSubnet: normalizePrefix(allowedSubnet),
		idleTimeout:   idleTimeout,
		capacity:      make(chan struct{}, maxConnections),
		done:          make(chan struct{}),
		connections:   make(map[*managedConn]struct{}),
		clients:       make(map[netip.Addr]int),
	}
}

func (l *managedListener) Accept() (net.Conn, error) {
	for {
		if !l.acquireCapacity() {
			return nil, net.ErrClosed
		}

		connection, err := l.inner.Accept()
		if err != nil {
			if connection != nil {
				_ = connection.Close()
			}
			l.releaseCapacity()
			if l.isClosed() {
				return nil, net.ErrClosed
			}
			return nil, err
		}

		source, valid := remoteIP(connection)
		if !valid || !l.allowedSubnet.Contains(source) {
			if connection != nil {
				_ = connection.Close()
			}
			l.releaseCapacity()
			continue
		}

		managed := &managedConn{
			Conn:        connection,
			listener:    l,
			source:      source,
			idleTimeout: l.idleTimeout,
		}
		if !l.register(managed) {
			_ = connection.Close()
			l.releaseCapacity()
			return nil, net.ErrClosed
		}
		if halfClose, ok := connection.(halfCloseConn); ok {
			return &managedHalfCloseConn{managedConn: managed, halfClose: halfClose}, nil
		}
		return managed, nil
	}
}

func (l *managedListener) Close() error {
	l.closeOnce.Do(func() {
		l.mutex.Lock()
		l.closed = true
		l.mutex.Unlock()
		close(l.done)
		l.closeErr = l.inner.Close()
	})
	return l.closeErr
}

func (l *managedListener) Addr() net.Addr {
	return l.inner.Addr()
}

func (l *managedListener) ClientCount() int {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return len(l.clients)
}

func (l *managedListener) ConnectionCount() int {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return len(l.connections)
}

func (l *managedListener) CloseConnections() {
	l.mutex.Lock()
	connections := make([]*managedConn, 0, len(l.connections))
	for connection := range l.connections {
		connections = append(connections, connection)
	}
	l.mutex.Unlock()

	for _, connection := range connections {
		_ = connection.Close()
	}
}

func (l *managedListener) acquireCapacity() bool {
	select {
	case <-l.done:
		return false
	default:
	}

	select {
	case l.capacity <- struct{}{}:
		select {
		case <-l.done:
			l.releaseCapacity()
			return false
		default:
			return true
		}
	case <-l.done:
		return false
	}
}

func (l *managedListener) releaseCapacity() {
	<-l.capacity
}

func (l *managedListener) register(connection *managedConn) bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if l.closed {
		return false
	}
	l.connections[connection] = struct{}{}
	l.clients[connection.source]++
	return true
}

func (l *managedListener) unregister(connection *managedConn) {
	l.mutex.Lock()
	if _, exists := l.connections[connection]; !exists {
		l.mutex.Unlock()
		return
	}
	delete(l.connections, connection)
	if l.clients[connection.source] == 1 {
		delete(l.clients, connection.source)
	} else {
		l.clients[connection.source]--
	}
	l.mutex.Unlock()
	l.releaseCapacity()
}

func (l *managedListener) isClosed() bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.closed
}

func (c *managedConn) Read(buffer []byte) (int, error) {
	if err := c.refreshReadDeadline(); err != nil {
		return 0, err
	}
	return c.Conn.Read(buffer)
}

func (c *managedConn) Write(buffer []byte) (int, error) {
	if err := c.refreshWriteDeadline(); err != nil {
		return 0, err
	}
	return c.Conn.Write(buffer)
}

func (c *managedConn) SetDeadline(deadline time.Time) error {
	c.deadlineMutex.Lock()
	c.readDeadline = deadline
	c.writeDeadline = deadline
	c.deadlineMutex.Unlock()
	return c.Conn.SetDeadline(deadline)
}

func (c *managedConn) SetReadDeadline(deadline time.Time) error {
	c.deadlineMutex.Lock()
	c.readDeadline = deadline
	c.deadlineMutex.Unlock()
	return c.Conn.SetReadDeadline(deadline)
}

func (c *managedConn) SetWriteDeadline(deadline time.Time) error {
	c.deadlineMutex.Lock()
	c.writeDeadline = deadline
	c.deadlineMutex.Unlock()
	return c.Conn.SetWriteDeadline(deadline)
}

func (c *managedConn) refreshReadDeadline() error {
	c.deadlineMutex.Lock()
	defer c.deadlineMutex.Unlock()
	if c.idleTimeout <= 0 || !c.readDeadline.IsZero() {
		return nil
	}
	return c.Conn.SetReadDeadline(time.Now().Add(c.idleTimeout))
}

func (c *managedConn) refreshWriteDeadline() error {
	c.deadlineMutex.Lock()
	defer c.deadlineMutex.Unlock()
	if c.idleTimeout <= 0 || !c.writeDeadline.IsZero() {
		return nil
	}
	return c.Conn.SetWriteDeadline(time.Now().Add(c.idleTimeout))
}

func (c *managedConn) Close() error {
	c.closeOnce.Do(func() {
		c.closeErr = c.Conn.Close()
		c.listener.unregister(c)
	})
	return c.closeErr
}

func (c *managedHalfCloseConn) CloseRead() error {
	return c.halfClose.CloseRead()
}

func (c *managedHalfCloseConn) CloseWrite() error {
	return c.halfClose.CloseWrite()
}

func remoteIP(connection net.Conn) (netip.Addr, bool) {
	if connection == nil {
		return netip.Addr{}, false
	}
	remoteAddress := connection.RemoteAddr()
	if remoteAddress == nil {
		return netip.Addr{}, false
	}
	if tcpAddress, ok := remoteAddress.(*net.TCPAddr); ok {
		if tcpAddress == nil {
			return netip.Addr{}, false
		}
		address, valid := netip.AddrFromSlice(tcpAddress.IP)
		if !valid {
			return netip.Addr{}, false
		}
		return normalizeAddress(address), true
	}
	addressPort, err := netip.ParseAddrPort(remoteAddress.String())
	if err != nil {
		return netip.Addr{}, false
	}
	return normalizeAddress(addressPort.Addr()), true
}

func normalizeAddress(address netip.Addr) netip.Addr {
	return address.WithZone("").Unmap()
}

func normalizePrefix(prefix netip.Prefix) netip.Prefix {
	if !prefix.IsValid() {
		return netip.Prefix{}
	}
	address := prefix.Addr()
	bits := prefix.Bits()
	if address.Is4In6() {
		if bits < 96 {
			return netip.Prefix{}
		}
		address = address.Unmap()
		bits -= 96
	}
	return netip.PrefixFrom(address.WithZone(""), bits).Masked()
}

var _ net.Listener = (*managedListener)(nil)
var _ net.Conn = (*managedConn)(nil)
