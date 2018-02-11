package muxstream

import (
	"encoding/binary"
	"io"
)

type streamManager struct {
	streams     map[uint32]*Stream
	reqQueue    *closerQueue
	idleStreams *closerQueue
}

func newStreamManager() *streamManager {
	return &streamManager{
		streams:     make(map[uint32]*Stream),
		reqQueue:    newCloserQueue(),
		idleStreams: newCloserQueue(),
	}
}

func (m *streamManager) streamIn(stream *Stream) {
	m.streams[stream.streamID] = stream
	if m.reqQueue.isEmpty() {
		m.idleStreams.push(stream)
	} else {
		req := m.reqQueue.pop().(*channelRequest)
		req.finish(stream, nil)
	}
}

func (m *streamManager) requestIn(req *channelRequest) {
	if m.idleStreams.isEmpty() {
		m.reqQueue.push(req)
	} else {
		stream := m.idleStreams.pop().(*Stream)
		req.finish(stream, nil)
	}
}

func (m *streamManager) get(id uint32) (*Stream, bool) {
	stream, exist := m.streams[id]
	return stream, exist
}

func (m *streamManager) del(id uint32) {
	if _, exist := m.streams[id]; exist {
		delete(m.streams, id)
	}
}

func (m *streamManager) Close() error {
	closeEvent := newEvent(_EVENT_STREAM_CLOSE, nil)
	for _, stream := range m.streams {
		closeEvent.sendTo(stream.eventChannel)
	}
	return nil
}

type Session struct {
	rwc io.ReadWriteCloser

	streamID uint32

	conf    *Config
	closed  bool
	streamM *streamManager

	accStreamQueue []*channelRequest
	newStreamQueue []*channelRequest

	eventChannel chan *event
	sendQueue    chan *event
}

func NewSession(rwc io.ReadWriteCloser, conf *Config) (*Session, error) {
	s := &Session{
		rwc:            rwc,
		streamID:       _MIN_STREAM_ID,
		conf:           conf,
		closed:         false,
		streamM:        newStreamManager(),
		accStreamQueue: []*channelRequest{},
		newStreamQueue: []*channelRequest{},
		eventChannel:   make(chan *event, _CHANNEL_SIZE),
		sendQueue:      make(chan *event, _CHANNEL_SIZE),
	}
	go s.serv()
	return s, nil
}

func (s *Session) processFrame(f *frame) (err error) {
	switch f.cmd {
	case _CMD_NEW_STREAM:
		if s.conf.role != _ROLE_SERVER {
			return
		}
		resFrame := &frame{
			version:  _PROTO_VER,
			cmd:      _CMD_NEW_STREAM_ACK,
			streamID: s.streamID,
			data:     nil,
		}

		s.streamM.streamIn(newStream(s.streamID, s))
		s.streamID += 1

		req := newChannelRequest(resFrame, false)
		newEvent(_EVENT_SESSION_SL_SEND_FRAME, req).sendTo(s.sendQueue)
	case _CMD_NEW_STREAM_ACK:
		if s.conf.role != _ROLE_CLIENT {
			return
		}
		if _, exist := s.streamM.get(f.streamID); !exist {
			s.streamM.streamIn(newStream(f.streamID, s))
		}
	case _CMD_DATA:
		if stream, exist := s.streamM.get(f.streamID); exist {
			newEvent(_EVENT_STREAM_DATA_IN, f.data).sendTo(
				stream.eventChannel)
		}
	case _CMD_HEARTBEAT:
	case _CMD_STREAM_CLOSE:
		if stream, exist := s.streamM.get(f.streamID); exist {
			newEvent(_EVENT_STREAM_CLOSE_WAIT, nil).sendTo(
				stream.eventChannel)
		}
	default:
	}
	return
}

func (s *Session) serv() {
	go s.recvLoop()
	go s.sendLoop()

	for e := range s.eventChannel {
		switch e.typ {
		case _EVENT_SESSION_FRAME_IN:
			f := e.data.(*frame)
			s.processFrame(f)
		case _EVENT_SESSION_RECV_ERROR:
			goto end
		case _EVENT_SESSION_SEND_ERROR:
			goto end
		case _EVENT_SESSION_CLOSE:
			goto end
		case _EVENT_SESSION_ACC_STREAM:
			req := e.data.(*channelRequest)
			s.streamM.requestIn(req)
		case _EVENT_SESSION_NEW_STREAM:
			req := e.data.(*channelRequest)
			s.streamM.requestIn(req)

			f := &frame{
				version: _PROTO_VER,
				cmd:     _CMD_NEW_STREAM,
			}
			req = newChannelRequest(f, false)
			newEvent(_EVENT_SESSION_SL_SEND_FRAME, req).sendTo(s.sendQueue)
		case _EVENT_STREAM_TERMINAL:
			streamID := e.data.(uint32)
			s.streamM.del(streamID)

			f := &frame{
				version:  _PROTO_VER,
				cmd:      _CMD_STREAM_CLOSE,
				streamID: streamID,
				data:     nil,
			}
			req := newChannelRequest(f, false)
			newEvent(_EVENT_SESSION_SL_SEND_FRAME, req).sendTo(s.sendQueue)
		}
	}

end:
	s.terminal()
}

func (s *Session) readFrame() (f *frame, err error) {
	var (
		twoBytes = make([]byte, 2, 2)
		dataLen  uint16
	)

	_, err = io.ReadFull(s.rwc, twoBytes)
	if err != nil {
		return
	} else if twoBytes[0] != _PROTO_VER {
		err = ERR_PROTO_VERSION
		return
	}

	f = &frame{
		version:  _PROTO_VER,
		cmd:      twoBytes[1],
		streamID: 0,
		data:     nil,
	}
	switch f.cmd {
	case _CMD_NEW_STREAM:
	case _CMD_NEW_STREAM_ACK:
		err = binary.Read(s.rwc, binary.BigEndian, &f.streamID)
	case _CMD_DATA:
		err = binary.Read(s.rwc, binary.BigEndian, &f.streamID)
		if err != nil {
			return
		}
		err = binary.Read(s.rwc, binary.BigEndian, &dataLen)
		if err != nil {
			return
		}
		f.data = make([]byte, dataLen, dataLen)
		_, err = io.ReadFull(s.rwc, f.data)
	case _CMD_STREAM_CLOSE:
		err = binary.Read(s.rwc, binary.BigEndian, &f.streamID)
	case _CMD_HEARTBEAT:
	default:
		err = ERR_UNKNOWN_CMD
	}

	return
}

func (s *Session) recvLoop() {
	for {
		if f, err := s.readFrame(); err != nil {
			newEvent(_EVENT_SESSION_RECV_ERROR, err).sendTo(s.eventChannel)
			return
		} else {
			newEvent(_EVENT_SESSION_FRAME_IN, f).sendTo(s.eventChannel)
		}
	}
}

func (s *Session) sendLoop() {
	var (
		n   int
		err error
	)

	for e := range s.sendQueue {
		switch e.typ {
		case _EVENT_SESSION_SL_SEND_FRAME:
			req := e.data.(*channelRequest)
			data := req.arg.(*frame).encode().Bytes()
			dataLen := len(data)
			i := 0
			for i < dataLen {
				if n, err = s.rwc.Write(data[i:]); err != nil {
					newEvent(_EVENT_SESSION_SEND_ERROR, err).sendTo(
						s.eventChannel)
					break
				} else {
					i += n
				}
			}
			req.finish(n-_HEADER_SIZE, err)
			if err != nil {
				return
			}
		case _EVENT_SESSION_SL_CLOSE:
			return
		}
	}
}

func (s *Session) heartbeat() {
}

func (s *Session) terminal() {
	s.closed = true
	newEvent(_EVENT_SESSION_SL_CLOSE, nil).sendTo(s.sendQueue)

	go waitForEventChannelClean(s.eventChannel)
	go waitForEventChannelClean(s.sendQueue)

	for _, req := range s.accStreamQueue {
		req.Close()
	}
	for _, req := range s.newStreamQueue {
		req.Close()
	}

	s.streamM.Close()
	s.rwc.Close()
}

func (s *Session) AcceptStream() (*Stream, error) {
	if s.conf.role != _ROLE_SERVER {
		return nil, ERR_NOT_SERVER
	}

	req := newChannelRequest(nil, true)
	newEvent(_EVENT_SESSION_ACC_STREAM, req).sendTo(s.eventChannel)
	stream, err := req.bGetResponse()
	if err != nil {
		return nil, err
	} else {
		return stream.(*Stream), nil
	}
}

func (s *Session) OpenStream() (*Stream, error) {
	if s.conf.role != _ROLE_CLIENT {
		return nil, ERR_NOT_CLIENT
	}

	req := newChannelRequest(nil, true)
	newEvent(_EVENT_SESSION_NEW_STREAM, req).sendTo(s.eventChannel)
	stream, err := req.bGetResponse()
	if err != nil {
		return nil, err
	} else {
		return stream.(*Stream), nil
	}
}

func (s *Session) IsClosed() bool {
	return s.closed
}

func (s *Session) Close() error {
	newEvent(_EVENT_SESSION_CLOSE, nil).sendTo(s.eventChannel)
	return nil
}
