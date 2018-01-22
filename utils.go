package muxstream

import "errors"

const (
	_EVENT_SESSION_HEARTBEAT = iota
	_EVENT_ERROR
	_EVENT_NEW_SESSION
	_EVENT_NEW_SESSION_ACK
)

const (
	_BUFFER_SZIE  = 0xffff
	_CHANNEL_SIZE = 100
)

var (
	_ERR_PROTO_VERSION = errors.New("muxstream: error proto version")
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
