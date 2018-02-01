package muxstream

import "errors"

const (
	_EVENT_SESSION_FRAME_IN = iota
	_EVENT_SESSION_FRAME_OUT
	_EVENT_SESSION_RECV_ERROR
	_EVENT_SESSION_SEND_ERROR
	_EVENT_SESSION_CLOSE

	_EVENT_STREAM_CLOSE
)

const (
	_BUFFER_SZIE  = 0xffff
	_CHANNEL_SIZE = 100
)

var (
	ERR_PROTO_VERSION  = errors.New("muxstream: error proto version")
	ERR_UNKNOWN_CMD    = errors.New("muxstream: unknown command")
	ERR_NOT_SERVER     = errors.New("muxstream: not a server conn")
	ERR_NOT_CLIENT     = errors.New("muxstream: not a client conn")
	ERR_CLOSED_SESSION = errors.New("muxstream: used of a closed session")
	ERR_CLOSED_STREAM  = errors.New("muxstream: used of a closed stream")
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
