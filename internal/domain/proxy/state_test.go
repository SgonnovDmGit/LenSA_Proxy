package proxy

import (
	"net/netip"
	"testing"
)

func TestStateString(t *testing.T) {
	tests := []struct {
		state State
		want  string
	}{
		{StateStopped, "stopped"},
		{StateStarting, "starting"},
		{StateRunning, "running"},
		{StateStopping, "stopping"},
		{StateError, "error"},
		{State(255), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("State(%d).String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}

func TestSnapshotAddress(t *testing.T) {
	config := Config{
		Interface: NetworkInterface{
			Name:    "Ethernet",
			Address: netip.MustParsePrefix("192.168.1.42/24"),
		},
		Port: 8080,
	}

	tests := []struct {
		name     string
		snapshot Snapshot
		want     string
	}{
		{name: "configured", snapshot: Snapshot{Config: config}, want: "192.168.1.42:8080"},
		{name: "actual", snapshot: Snapshot{Config: config, Address: "192.168.1.42:9090"}, want: "192.168.1.42:9090"},
		{name: "empty", snapshot: Snapshot{}, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.snapshot.ProxyAddress(); got != tt.want {
				t.Fatalf("ProxyAddress() = %q, want %q", got, tt.want)
			}
		})
	}
}
