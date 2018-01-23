package muxstream

import "errors"

const (
	_EVENT_FRAME_IN = iota
	_EVENT_RECV_ERROR
	_EVENT_SEND_ERROR
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
