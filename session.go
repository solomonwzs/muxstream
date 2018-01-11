package muxstream

import (
	"io"
	"net"
	"sync"
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

type Session struct {
	buf []byte
	rw  *sessionRW

	streamID     uint32
	streamIDLock *sync.Mutex

	conf *Config

	eventChannel chan *event

	streams map[uint32]*Stream
}

func NewSession(conn net.Conn, conf *Config) *Session {
	return &Session{
		buf:          []byte{},
		rw:           newSessionRW(conn),
		streamID:     0,
		streamIDLock: &sync.Mutex{},
		conf:         conf,
		eventChannel: make(chan *event, _CHANNEL_SIZE),
		streams:      make(map[uint32]*Stream),
	}
}

func (s *Session) serv() {
}

func (s *Session) recv() {
}

func (s *Session) heartbeat() {
}

func (s *Session) terminal() {
}
