package siptcp

import "sync/atomic"

type ConnectionMetrics struct {
	currentConnections  atomic.Int64
	acceptedConnections atomic.Uint64
	closedConnections   atomic.Uint64
	readTimeouts        atomic.Uint64
	writeTimeouts       atomic.Uint64
	connectionErrors    atomic.Uint64
}

type Snapshot struct {
	CurrentConnections       int64  `json:"current_connections"`
	AcceptedConnectionsTotal uint64 `json:"accepted_connections_total"`
	ClosedConnectionsTotal   uint64 `json:"closed_connections_total"`
	ReadTimeoutTotal         uint64 `json:"read_timeout_total"`
	WriteTimeoutTotal        uint64 `json:"write_timeout_total"`
	ConnectionErrorTotal     uint64 `json:"connection_error_total"`
}

func (m *ConnectionMetrics) OnAccepted() {
	m.currentConnections.Add(1)
	m.acceptedConnections.Add(1)
}

func (m *ConnectionMetrics) OnClosed() {
	m.currentConnections.Add(-1)
	m.closedConnections.Add(1)
}

func (m *ConnectionMetrics) OnReadTimeout() {
	m.readTimeouts.Add(1)
}

func (m *ConnectionMetrics) OnWriteTimeout() {
	m.writeTimeouts.Add(1)
}

func (m *ConnectionMetrics) OnConnectionError() {
	m.connectionErrors.Add(1)
}

func (m *ConnectionMetrics) Snapshot() Snapshot {
	return Snapshot{
		CurrentConnections:       m.currentConnections.Load(),
		AcceptedConnectionsTotal: m.acceptedConnections.Load(),
		ClosedConnectionsTotal:   m.closedConnections.Load(),
		ReadTimeoutTotal:         m.readTimeouts.Load(),
		WriteTimeoutTotal:        m.writeTimeouts.Load(),
		ConnectionErrorTotal:     m.connectionErrors.Load(),
	}
}
