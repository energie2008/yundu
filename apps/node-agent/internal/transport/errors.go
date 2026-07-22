package transport

import "errors"

var (
	ErrNoChannelAvailable = errors.New("transport: no channel available")
	ErrNotConnected       = errors.New("transport: channel not connected")
	ErrSendTimeout        = errors.New("transport: send timeout")
	ErrHealthCheckFailed  = errors.New("transport: health check failed")
	ErrAuthFailed         = errors.New("transport: authentication failed")
)
