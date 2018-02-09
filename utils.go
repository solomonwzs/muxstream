package muxstream

import (
	"errors"
	"io"
	"time"
)

const (
	_EVENT_SESSION_FRAME_IN = iota
	_EVENT_SESSION_FRAME_OUT
	_EVENT_SESSION_RECV_ERROR
	_EVENT_SESSION_SEND_ERROR
	_EVENT_SESSION_CLOSE

	_EVENT_SESSION_SL_SEND_FRAME
	_EVENT_SESSION_SL_CLOSE

	_EVENT_STREAM_CLOSE
	_EVENT_STREAM_TERMINAL
	_EVENT_STREAM_DATA_IN
	_EVENT_STREAM_READ_REQ
	_EVENT_STREAM_READ_REQ_TIMEOUT
)

const (
	_MAX_BUFFER_SIZE = 0xffff
	_CHANNEL_SIZE    = 100
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
	ERR_CLOSED_SESSION     = errors.New("muxstream: use of a closed session")
	ERR_CLOSED_STREAM      = errors.New("muxstream: use of a closed stream")
	ERR_CH_REQ_WAS_CLOSED  = errors.New("muxstream: channel request was closed")
	ERR_CH_RES_NIL         = errors.New("muxstream: channel response was nil")
	ERR_SEND_WAS_INTERED   = errors.New("muxstream: send was interrupted")
	ERR_STREAM_WAS_CLOSED  = errors.New("muxstream: stream was closed")
	ERR_SESSION_WAS_CLOSED = errors.New("muxstream: session was closed")
	ERR_STREAM_IO_TIMEOUT  = newTimeoutError("muxstream: stream i/o timeout")
)

var (
	_HEARTBEAT = []byte{_PROTO_VER, _CMD_HEARTBEAT}
)

type event struct {
	typ  uint8
	data interface{}
}

func newEvent(typ uint8, data interface{}) *event {
	return &event{
		typ:  typ,
		data: data,
	}
}

func (e *event) sendTo(ch chan *event) {
	go func() { ch <- e }()
}

func waitForEventChannelClean(ch chan *event) {
	end := time.After(_CHANNEL_TIME_WAIT)
	for {
		select {
		case e, ok := <-ch:
			if ok {
				if c, ok := e.data.(io.Closer); ok {
					c.Close()
				}
			} else {
				return
			}
		case <-end:
			return
		}
	}
}
