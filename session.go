package muxstream

import (
	"net"
	"sync"
)

type Session struct {
	buf  []byte
	conn net.Conn

	streamID     uint32
	streamIDLock *sync.Mutex

	conf *Config

	eventChannel chan *event
}

func NewSession(conn net.Conn, conf *Config) *Session {
	return &Session{
		buf:          []byte{},
		conn:         conn,
		streamID:     0,
		streamIDLock: &sync.Mutex{},
		conf:         conf,
		eventChannel: make(chan *event, _CHANNEL_SIZE),
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
