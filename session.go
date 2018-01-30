package muxstream

import (
	"encoding/binary"
	"io"
)

type Session struct {
	rwc io.ReadWriteCloser

	streamID uint32

	conf    *Config
	streams map[uint32]*Stream

	eventChannel     chan *event
	framesWaitToSend chan *frame
	newStreams       chan *Stream
}

func NewSession(rwc io.ReadWriteCloser, conf *Config) (*Session, error) {
	return &Session{
		rwc:              rwc,
		streamID:         _MIN_STREAM_ID,
		conf:             conf,
		streams:          make(map[uint32]*Stream),
		eventChannel:     make(chan *event, _CHANNEL_SIZE),
		framesWaitToSend: make(chan *frame, _CHANNEL_SIZE),
		newStreams:       make(chan *Stream, _CHANNEL_SIZE),
	}, nil
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

		s.streams[s.streamID] = &Stream{
			streamID: s.streamID,
			session:  s,
		}
		s.streamID += 1

		go func() {
			s.framesWaitToSend <- resFrame
		}()
	case _CMD_NEW_STREAM_ACK:
		if s.conf.role != _ROLE_CLIENT {
			return
		}
		if _, exist := s.streams[f.streamID]; !exist {
			s.streams[f.streamID] = &Stream{
				streamID: s.streamID,
				session:  s,
			}
		}
	case _CMD_DATA:
	case _CMD_HEARTBEAT:
	case _CMD_STREAM_CLOSE:
	default:
	}
	return
}

func (s *Session) serv() {
	go s.recvLoop()
	go s.sendLoop()

	for e := range s.eventChannel {
		switch e.typ {
		case _EVENT_FRAME_IN:
			f := e.data.(*frame)
			s.processFrame(f)
		case _EVENT_FRAME_OUT:
			f := e.data.(*frame)
			go func() {
				s.framesWaitToSend <- f
			}()
		case _EVENT_RECV_ERROR:
			goto end
		case _EVENT_SEND_ERROR:
			goto end
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
			newEvent(_EVENT_RECV_ERROR, err).sendTo(s.eventChannel)
			return
		} else {
			newEvent(_EVENT_FRAME_IN, f).sendTo(s.eventChannel)
		}
	}
}

func (s *Session) sendLoop() {
	for f := range s.framesWaitToSend {
		data := f.encode().Bytes()
		dataLen := len(data)
		i := 0
		for i < dataLen {
			if n, err := s.rwc.Write(data[i:]); err != nil {
				newEvent(_EVENT_SEND_ERROR, err).sendTo(s.eventChannel)
				return
			} else {
				i += n
			}
		}
	}
}

func (s *Session) heartbeat() {
}

func (s *Session) terminal() {
	s.rwc.Close()
	close(s.framesWaitToSend)
}

func (s *Session) AcceptStream() (stream *Stream, err error) {
	if s.conf.role != _ROLE_SERVER {
		return nil, ERR_NOT_SERVER
	}
	return
}

func (s *Session) NewStream() (stream *Stream, err error) {
	if s.conf.role != _ROLE_CLIENT {
		return nil, ERR_NOT_CLIENT
	}
	return
}
