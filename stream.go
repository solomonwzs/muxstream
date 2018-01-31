package muxstream

type Stream struct {
	streamID uint32
	session  *Session
}

func newStream(streamID uint32, s *Session) *Stream {
	return &Stream{
		streamID: streamID,
		session:  s,
	}
}

func (stream *Stream) packDataFrame(p []byte) *frame {
	return &frame{
		version:  _PROTO_VER,
		cmd:      _CMD_DATA,
		streamID: stream.streamID,
		data:     p,
	}
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
