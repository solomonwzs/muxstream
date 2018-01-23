package muxstream

import (
	"encoding/binary"
	"io"
	"net"
	"sync"
	"time"
)

const (
	_ROLE_CLIENT = 0x01
	_ROLE_SERVER = 0x02
)

type sessionRW struct {
	net.Conn
	readBuffer  []byte
	writeBuffer []byte
}

func newSessionRW(conn net.Conn) *sessionRW {
	return &sessionRW{
		readBuffer:  []byte{},
		writeBuffer: []byte{},
		Conn:        conn,
	}
}

func (srw *sessionRW) Read(p []byte) (n int, err error) {
	l := len(p)
	if l < len(srw.readBuffer) {
		copy(p, srw.readBuffer[0:l])
		srw.readBuffer = srw.readBuffer[l:]
		return l, nil
	} else {
		lBuf := len(srw.readBuffer)
		if lBuf != 0 {
			copy(p, srw.readBuffer)
		}
		n, err = io.ReadFull(srw.Conn, p[lBuf:])
		n += lBuf
		return
	}
}

func (srw *sessionRW) recycleBytes(p []byte) {
	srw.readBuffer = append(p, srw.readBuffer...)
}

type Session struct {
	rw *sessionRW

	streamID     uint32
	streamIDLock *sync.Mutex

	role    uint8
	conf    *Config
	streams map[uint32]*Stream

	eventChannel   chan *event
	dataWaitToSend chan []byte
}

func NewSession(conn net.Conn, conf *Config) *Session {
	return &Session{
		rw:             newSessionRW(conn),
		streamID:       _MIN_STREAM_ID,
		streamIDLock:   &sync.Mutex{},
		conf:           conf,
		streams:        make(map[uint32]*Stream),
		eventChannel:   make(chan *event, _CHANNEL_SIZE),
		dataWaitToSend: make(chan []byte, _CHANNEL_SIZE),
	}
}

func (s *Session) serv() {
	go s.recv()
	go s.send()

	for e := range s.eventChannel {
		switch e.typ {
		case _EVENT_FRAME_IN:
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
	s.rw.SetReadDeadline(time.Now().Add(s.conf.networkTimeoutDuration))
	defer s.rw.SetReadDeadline(time.Time{})

	var (
		twoBytes = make([]byte, 2, 2)
		dataLen  uint16
	)

	_, err = io.ReadFull(s.rw, twoBytes)
	if err != nil {
		return
	} else if twoBytes[0] != _PROTO_VER {
		err = _ERR_PROTO_VERSION
		return
	}

	f = &frame{
		version:  _PROTO_VER,
		cmd:      twoBytes[1],
		streamID: 0,
		data:     nil,
	}
	switch f.cmd {
	case _CMD_NEW_STREAM_ACK:
		err = binary.Read(s.rw, binary.BigEndian, &f.streamID)
	case _CMD_DATA:
		err = binary.Read(s.rw, binary.BigEndian, &f.streamID)
		if err != nil {
			return
		}
		err = binary.Read(s.rw, binary.BigEndian, &dataLen)
		if err != nil {
			return
		}
		f.data = make([]byte, dataLen, dataLen)
		_, err = io.ReadFull(s.rw, f.data)
	case _CMD_STREAM_CLOSE:
		err = binary.Read(s.rw, binary.BigEndian, &f.streamID)
	default:
	}

	return
}

func (s *Session) recv() {
	for {
		if f, err := s.readFrame(); err != nil {
			newEvent(_EVENT_RECV_ERROR, err).sendTo(s.eventChannel)
			return
		} else {
			newEvent(_EVENT_FRAME_IN, f).sendTo(s.eventChannel)
		}
	}
}

func (s *Session) send() {
	for data := range s.dataWaitToSend {
		s.rw.SetWriteDeadline(time.Now().Add(s.conf.networkTimeoutDuration))
		dataLen := len(data)
		i := 0
		for i < dataLen {
			if n, err := s.rw.Write(data[i:]); err != nil {
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
	s.rw.Close()
	close(s.dataWaitToSend)
}
