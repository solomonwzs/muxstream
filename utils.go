package muxstream

const (
	_EVENT_SESSION_HEARTBEAT = iota
)

const (
	_BUFFER_SZIE  = 0xffff
	_CHANNEL_SIZE = 100
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
