package muxstream

import (
	"bytes"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type tempWriter struct {
	p  []byte
	ch chan int
}

func newTempWriter(p []byte) *tempWriter {
	return &tempWriter{
		p:  p,
		ch: make(chan int, 1),
	}
}

func (t *tempWriter) Write(p []byte) (n int, err error) {
	n = copy(t.p, p)
	t.ch <- n
	return
}

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

	req := newFrameRequest(&frame{
		version:  _PROTO_VER,
		cmd:      _CMD_STREAM_CLOSE,
		streamID: s.streamID,
		data:     nil,
	}, false)
	req.send(s.sess, 0)

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
		if s.sess.IsClosed() {
			return 0, ERR_SESSION_WAS_CLOSED
		}
	}
	if len(p) == 0 {
		return 0, nil
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
		case <-s.end:
			return 0, io.EOF
		}
	}
}

func (s *Stream) Write(p []byte) (n int, err error) {
	if s.IsClosed() {
		return 0, ERR_STREAM_WAS_CLOSED
	}
	if s.sess.IsClosed() {
		return 0, ERR_SESSION_WAS_CLOSED
	}
	if len(p) == 0 {
		return 0, nil
	}

	n = 0
	dataLen := len(p)
	f := &frame{
		version:  _PROTO_VER,
		cmd:      _CMD_DATA,
		streamID: s.streamID,
	}
	for n < dataLen {
		m := n + s.sess.conf.maxFrameSize
		if m > dataLen {
			m = dataLen
		}
		f.data = p[n:m]

		req := newFrameRequest(f, true)
		n0, err0 := req.send(s.sess, 0)
		if err0 != nil {
			return n, err0
		} else {
			n += n0 - _FRAME_HEADER_SIZE
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
