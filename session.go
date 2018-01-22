package muxstream

import (
	"encoding/binary"
	"io"
	"net"
	"sync"
)

const (
	_ROLE_CLIENT = 0x01
	_ROLE_SERVER = 0x02
)

type sessionRW struct {
	readBuffer  []byte
	writeBuffer []byte
	conn        net.Conn
}

func newSessionRW(conn net.Conn) *sessionRW {
	return &sessionRW{
		readBuffer:  []byte{},
		writeBuffer: []byte{},
		conn:        conn,
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
		n, err = io.ReadFull(srw.conn, p[lBuf:])
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
	eCh     chan *event
	streams map[uint32]*Stream
}

func NewSession(conn net.Conn, conf *Config) *Session {
	return &Session{
		rw:           newSessionRW(conn),
		streamID:     0,
		streamIDLock: &sync.Mutex{},
		conf:         conf,
		eCh:          make(chan *event, _CHANNEL_SIZE),
		streams:      make(map[uint32]*Stream),
	}
}

func (s *Session) serv() {
	for {
	}
}

func (s *Session) recv() {
	var (
		twoBytes = make([]byte, 2, 2)
		streamID uint32
		dataLen  uint16
		err      error
	)

	for {
		_, err = io.ReadFull(s.rw, twoBytes)
		if err == nil && twoBytes[0] == _PROTO_VER {
			switch twoBytes[1] {
			case _CMD_NEW_STREAM:
				newEvent(_EVENT_NEW_SESSION, nil).sendTo(s.eCh)
			case _CMD_NEW_STREAM_ACK:
				err = binary.Read(s.rw, binary.BigEndian, &streamID)
				if err != nil {
					newEvent(_EVENT_ERROR, err).sendTo(s.eCh)
					goto end
				}
				newEvent(_EVENT_NEW_SESSION_ACK, streamID).sendTo(s.eCh)
			case _CMD_DATA:
				err = binary.Read(s.rw, binary.BigEndian, &dataLen)
			}
		} else if err != nil {
			newEvent(_EVENT_ERROR, err).sendTo(s.eCh)
			goto end
		} else {
			newEvent(_EVENT_ERROR, _ERR_PROTO_VERSION).sendTo(s.eCh)
			goto end
		}
	}
end:
	s.terminal()
}

func (s *Session) heartbeat() {
}

func (s *Session) terminal() {
}
