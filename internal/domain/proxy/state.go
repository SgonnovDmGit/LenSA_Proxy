package proxy

type State uint8

const (
	StateStopped State = iota
	StateStarting
	StateRunning
	StateStopping
	StateError
)

func (s State) String() string {
	switch s {
	case StateStopped:
		return "stopped"
	case StateStarting:
		return "starting"
	case StateRunning:
		return "running"
	case StateStopping:
		return "stopping"
	case StateError:
		return "error"
	default:
		return "unknown"
	}
}

type Snapshot struct {
	State        State
	Config       Config
	Address      string
	Clients      int
	ErrorMessage string
}

func (s Snapshot) ProxyAddress() string {
	if s.Address != "" {
		return s.Address
	}
	return s.Config.BindAddress()
}
