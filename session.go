package muxstream

import (
	"encoding/binary"
	"io"
	"net"
	"time"
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

func (m *streamManager) size() int {
	return len(m.streams)
}

func (m *streamManager) Close() error {
	closeEvent := newEvent(_EVENT_STREAM_CLOSE, nil)
	for _, stream := range m.streams {
		closeEvent.sendTo(stream.eventChannel)
	}
	m.reqQueue.Close()
	return nil
}

type Session struct {
	flagClosed

	rwc          io.ReadWriteCloser
	nextStreamID uint32
	conf         *Config
	streamM      *streamManager

	eventChannel chan *event
	sendQueue    chan *event
}

func NewSession(rwc io.ReadWriteCloser, conf *Config) (*Session, error) {
	s := &Session{
		rwc:          rwc,
		nextStreamID: _MIN_STREAM_ID,
		conf:         conf,
		flagClosed:   false,
		streamM:      newStreamManager(),
		eventChannel: make(chan *event, _CHANNEL_SIZE),
		sendQueue:    make(chan *event, _CHANNEL_SIZE),
	}
	go s.serv()
	return s, nil
}

func (s *Session) getNextStreamID() uint32 {
	id := s.nextStreamID
	for {
		s.nextStreamID += 1
		if _, exist := s.streamM.get(s.nextStreamID); !exist {
			break
		}
	}
	return id
}

func (s *Session) processFrame(f *frame) (err error) {
	switch f.cmd {
	case _CMD_NEW_STREAM:
		if s.conf.role != _ROLE_SERVER {
			return
		}
		streamID := s.getNextStreamID()
		resFrame := &frame{
			version:  _PROTO_VER,
			cmd:      _CMD_NEW_STREAM_ACK,
			streamID: streamID,
			data:     nil,
		}
		s.streamM.streamIn(newStream(streamID, s))

		req := newChannelRequest(resFrame, false)
		go newEvent(_EVENT_SESSION_SL_SEND_FRAME, req).sendTo(s.sendQueue)
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
	if s.conf.heartbeatDuration > 0 {
		go s.heartbeat()
	}

	for e := range s.eventChannel {
		switch e.typ {
		case _EVENT_SESSION_FRAME_IN:
			f := e.data.(*frame)
			s.processFrame(f)
		case _EVENT_SESSION_RECV_ERROR:
			err := e.data.(error)
			if err != ERR_PROTO_VERSION && err != ERR_UNKNOWN_CMD {
				goto end
			}
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
			if err != ERR_PROTO_VERSION && err != ERR_UNKNOWN_CMD {
				break
			}
		} else {
			newEvent(_EVENT_SESSION_FRAME_IN, f).sendTo(s.eventChannel)
		}
	}
}

func (s *Session) sendLoop() {
	for e := range s.sendQueue {
		switch e.typ {
		case _EVENT_SESSION_SL_SEND_FRAME:
			req := e.data.(*channelRequest)

			f := req.arg.(*frame)
			n, err := f.WriteTo(s.rwc)
			if err != nil {
				newEvent(_EVENT_SESSION_SEND_ERROR, err).sendTo(
					s.eventChannel)
			}

			req.finish(int(n-_HEADER_SIZE), err)
			if err != nil {
				return
			}
		case _EVENT_SESSION_SL_CLOSE:
			return
		}
	}
}

func (s *Session) heartbeat() {
	f := &frame{
		version: _PROTO_VER,
		cmd:     _CMD_HEARTBEAT,
	}
	req := newChannelRequest(f, false)
	for !s.flagClosed {
		newEvent(_EVENT_SESSION_SL_SEND_FRAME, req).sendTo(s.sendQueue)
		time.Sleep(s.conf.heartbeatDuration)
	}
}

func (s *Session) terminal() {
	s.flagClosed = true
	newEvent(_EVENT_SESSION_SL_CLOSE, nil).sendTo(s.sendQueue)

	go waitForEventChannelClean(s.eventChannel)
	go waitForEventChannelClean(s.sendQueue)

	s.streamM.Close()
	s.rwc.Close()
}

func (s *Session) AcceptStream() (*Stream, error) {
	if s.conf.role != _ROLE_SERVER {
		return nil, ERR_NOT_SERVER
	}

	req := newChannelRequest(nil, true)
	newEvent(_EVENT_SESSION_ACC_STREAM, req).sendTo(s.eventChannel)
	stream, err := req.bGetResponse(0)
	if err != nil {
		return nil, err
	} else {
		return stream.(*Stream), nil
	}
}

func (s *Session) Accept() (*Stream, error) {
	return s.AcceptStream()
}

func (s *Session) OpenStream() (*Stream, error) {
	if s.conf.role != _ROLE_CLIENT {
		return nil, ERR_NOT_CLIENT
	}

	if s.streamM.size() > _MAX_STREAMS_NUM {
		return nil, ERR_STREAMS_TOO_MUCH
	}

	req := newChannelRequest(nil, true)
	newEvent(_EVENT_SESSION_NEW_STREAM, req).sendTo(s.eventChannel)
	stream, err := req.bGetResponse(0)
	if err != nil {
		return nil, err
	} else {
		return stream.(*Stream), nil
	}
}

func (s *Session) LocalAddr() net.Addr {
	if conn, ok := s.rwc.(net.Conn); ok {
		return conn.LocalAddr()
	} else {
		return nil
	}
}

func (s *Session) Addr() net.Addr {
	return s.LocalAddr()
}

func (s *Session) RemoteAddr() net.Addr {
	if conn, ok := s.rwc.(net.Conn); ok {
		return conn.RemoteAddr()
	} else {
		return nil
	}
}

func (s *Session) Close() error {
	if s.flagClosed {
		return ERR_SESSION_WAS_CLOSED
	}
	newEvent(_EVENT_SESSION_CLOSE, nil).sendTo(s.eventChannel)
	return nil
}
