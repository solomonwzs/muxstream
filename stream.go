package muxstream

import (
	"bytes"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type Stream struct {
	streamID uint32
	sess     *Session

	buffer       *bytes.Buffer
	bufferLock   *sync.Mutex
	readyForRead chan struct{}

	end     chan struct{}
	endLock *sync.Mutex

	readDeadline  atomic.Value
	writeDeadline atomic.Value
}

func newStream(streamID uint32, sess *Session) *Stream {
	s := &Stream{
		streamID: streamID,
		sess:     sess,

		buffer:       new(bytes.Buffer),
		bufferLock:   &sync.Mutex{},
		readyForRead: make(chan struct{}, 1),

		end:     make(chan struct{}),
		endLock: &sync.Mutex{},
	}
	s.readDeadline.Store(time.Time{})
	s.writeDeadline.Store(time.Time{})
	return s
}

func (s *Stream) IsClosed() bool {
	select {
	case <-s.end:
		return true
	default:
		return false
	}
}

func (s *Stream) terminal(tf func() error) error {
	s.endLock.Lock()
	defer s.endLock.Unlock()

	if s.IsClosed() {
		return ERR_STREAM_WAS_CLOSED
	}
	close(s.end)

	req := frameRequest{
		f: &frame{
			version:  _PROTO_VER,
			cmd:      _CMD_STREAM_CLOSE,
			streamID: s.streamID,
			data:     nil,
		},
		ch: nil,
	}
	s.sess.sendChannel <- req

	if tf != nil {
		return tf()
	}
	return nil
}

func (s *Stream) Close() error {
	return s.terminal(func() error {
		s.sess.streamsLock.Lock()
		delete(s.sess.streams, s.streamID)
		s.sess.streamsLock.Unlock()
		return nil
	})
}

func (s *Stream) notifyRead() {
	select {
	case s.readyForRead <- struct{}{}:
	default:
	}
}

func (s *Stream) copyToBuffer(r io.Reader, dataLen uint16) {
	s.bufferLock.Lock()
	io.CopyN(s.buffer, r, int64(dataLen))
	if s.buffer.Len() > 0 {
		s.notifyRead()
	}
	s.bufferLock.Unlock()
}

func (s *Stream) Read(p []byte) (n int, err error) {
	if s.buffer.Len() == 0 {
		if s.IsClosed() {
			return 0, ERR_STREAM_WAS_CLOSED
		}
	}

	var deadline <-chan time.Time
	if d, ok := s.readDeadline.Load().(time.Time); ok && !d.IsZero() {
		timer := time.NewTimer(time.Until(d))
		defer timer.Stop()
		deadline = timer.C
	}

	for {
		s.bufferLock.Lock()
		n, err = s.buffer.Read(p)
		if s.buffer.Len() > 0 {
			s.notifyRead()
		}
		s.bufferLock.Unlock()

		if n > 0 {
			return
		}
		select {
		case <-s.readyForRead:
			break
		case <-deadline:
			return 0, ERR_STREAM_IO_TIMEOUT
		case <-s.end:
			return 0, io.EOF
		}
	}
}

func (s *Stream) Write(p []byte) (n int, err error) {
	if s.IsClosed() {
		return 0, ERR_STREAM_WAS_CLOSED
	}

	var deadline <-chan time.Time
	if d, ok := s.writeDeadline.Load().(time.Time); ok && !d.IsZero() {
		timer := time.NewTimer(time.Until(d))
		defer timer.Stop()
		deadline = timer.C
	}

	n = 0
	dataLen := len(p)
	f := frame{
		version:  _PROTO_VER,
		cmd:      _CMD_DATA,
		streamID: s.streamID,
	}
	req := frameRequest{ch: make(chan rwResult, 1)}

	for n < dataLen {
		m := n + s.sess.conf.maxFrameSize
		if m > dataLen {
			m = dataLen
		}
		f.data = p[n:m]

		req.f = &f
		s.sess.sendChannel <- req
		select {
		case r := <-req.ch:
			n += r.n - _FRAME_HEADER_SIZE
			if r.err != nil {
				return n, r.err
			}
		case <-deadline:
			return n, ERR_STREAM_IO_TIMEOUT
		case <-s.end:
			return n, ERR_STREAM_WAS_CLOSED
		}
	}
	return
}

func (s *Stream) LocalAddr() net.Addr {
	return s.sess.LocalAddr()
}

func (s *Stream) RemoteAddr() net.Addr {
	return s.sess.RemoteAddr()
}

func (s *Stream) SetReadDeadline(t time.Time) error {
	s.readDeadline.Store(t)
	return nil
}

func (s *Stream) SetWriteDeadline(t time.Time) error {
	s.writeDeadline.Store(t)
	return nil
}

func (s *Stream) SetDeadline(t time.Time) error {
	if err := s.SetReadDeadline(t); err != nil {
		return err
	}
	if err := s.SetWriteDeadline(t); err != nil {
		return err
	}
	return nil
}
