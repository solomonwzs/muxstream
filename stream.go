package muxstream

type Stream struct {
	streamID     uint32
	session      *Session
	readBuf      [][]byte
	eventChannel chan *event
}

func newStream(streamID uint32, s *Session) *Stream {
	stream := &Stream{
		streamID:     streamID,
		session:      s,
		readBuf:      [][]byte{},
		eventChannel: make(chan *event, _CHANNEL_SIZE),
	}
	go stream.serv()
	return stream
}

func (stream *Stream) packDataFrame(p []byte) *frame {
	return &frame{
		version:  _PROTO_VER,
		cmd:      _CMD_DATA,
		streamID: stream.streamID,
		data:     p,
	}
}

func (stream *Stream) serv() {
	for e := range stream.eventChannel {
		switch e.typ {
		case _EVENT_STREAM_CLOSE:
			goto end
		}
	}
end:
	stream.terminal()
}

func (stream *Stream) Read(p []byte) (n int, err error) {
	if stream.session.closed {
		return 0, ERR_CLOSED_SESSION
	}
	return
}

func (stream *Stream) Write(p []byte) (n int, err error) {
	if stream.session.closed {
		return 0, ERR_CLOSED_SESSION
	}
	return
}

func (stream *Stream) Close() (err error) {
	return
}

func (stream *Stream) terminal() {
}
