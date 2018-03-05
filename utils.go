package muxstream

import (
	"errors"
	"time"
)

const (
	_MAX_FRAME_SIZE  = 0xffff
	_MAX_STREAMS_NUM = 0xffffffff
	_CHANNEL_SIZE    = 0xff
)

var (
	_CHANNEL_TIME_WAIT = 16 * time.Second
)

type timeoutError struct {
	message string
}

func newTimeoutError(m string) *timeoutError { return &timeoutError{m} }
func (err *timeoutError) Error() string      { return err.message }
func (err *timeoutError) Timeout() bool      { return true }
func (err *timeoutError) Temporary() bool    { return true }

var (
	ERR_PROTO_VERSION      = errors.New("muxstream: error proto version")
	ERR_UNKNOWN_CMD        = errors.New("muxstream: unknown command")
	ERR_NOT_SERVER         = errors.New("muxstream: not a server conn")
	ERR_NOT_CLIENT         = errors.New("muxstream: not a client conn")
	ERR_CH_WAS_CLOSED      = errors.New("muxstream: channel was closed")
	ERR_CH_TIMEOUT         = errors.New("muxstream: channel timeout")
	ERR_STREAM_WAS_CLOSED  = errors.New("muxstream: stream was closed")
	ERR_SESSION_WAS_CLOSED = errors.New("muxstream: session was closed")
	ERR_STREAMS_TOO_MUCH   = errors.New("muxstream: streams too much")
	ERR_STREAM_IO_TIMEOUT  = newTimeoutError("muxstream: stream i/o timeout")
	ERR_SESSION_IO_BUSY    = errors.New("muxstream: session i/o was busy")
)

var (
	_BYTES_HEARTBEAT  = []byte{_PROTO_VER, _CMD_HEARTBEAT}
	_BYTES_NEW_STREAM = []byte{_PROTO_VER, _CMD_NEW_STREAM}
)

var (
	_FRAME_NEW_STREAM = &frame{_PROTO_VER, _CMD_NEW_STREAM, 0, nil}
)
