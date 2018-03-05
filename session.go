package muxstream

import (
	"encoding/binary"
	"io"
	"net"
	"sync"
	"time"
)

type rwResult struct {
	n   int
	err error
}

type frameRequest struct {
	f  *frame
	ch chan *rwResult
}

func newFrameRequest(f *frame, waitForReturn bool) *frameRequest {
	var ch chan *rwResult = nil
	if waitForReturn {
		ch = make(chan *rwResult, 1)
	}
	return &frameRequest{f: f, ch: ch}
}

func (fr *frameRequest) send(sess *Session, timeout time.Duration) (
	n int, err error) {
	select {
	case sess.sendChannel <- fr:
		break
	default:
		return 0, nil
	}
	if fr.ch == nil {
		return
	}

	var deadline <-chan time.Time = nil
	if timeout != 0 {
		deadline = time.After(timeout)
	}
	select {
	case r, ok := <-fr.ch:
		if ok {
			return r.n, r.err
		} else {
			return 0, ERR_CH_WAS_CLOSED
		}
	case <-sess.end:
		return 0, ERR_SESSION_WAS_CLOSED
	case <-deadline:
		return 0, ERR_CH_TIMEOUT
	}
}

type Session struct {
	rwc  io.ReadWriteCloser
	role uint8
	conf *Config

	nextStreamID uint32
	streams      map[uint32]*Stream
	streamsLock  *sync.RWMutex

	end     chan struct{}
	endLock *sync.Mutex

	sendChannel   chan *frameRequest
	streamChannel chan *Stream
}

func Server(rwc io.ReadWriteCloser, conf *Config) (*Session, error) {
	return newSession(rwc, conf, _ROLE_SERVER)
}

func Client(rwc io.ReadWriteCloser, conf *Config) (*Session, error) {
	return newSession(rwc, conf, _ROLE_CLIENT)
}

func newSession(rwc io.ReadWriteCloser, conf *Config, role uint8) (
	*Session, error) {
	s := &Session{
		rwc:  rwc,
		role: role,
		conf: conf,

		nextStreamID: 0,
		streams:      make(map[uint32]*Stream),
		streamsLock:  &sync.RWMutex{},

		end:     make(chan struct{}),
		endLock: &sync.Mutex{},

		sendChannel:   make(chan *frameRequest, 1024),
		streamChannel: make(chan *Stream),
	}
	go s.recvLoop()
	go s.sendLoop()
	return s, nil
}

func (sess *Session) IsClosed() bool {
	select {
	case <-sess.end:
		return true
	default:
		return false
	}
}

func (sess *Session) readFrameHeader(buf []byte) (
	cmd byte, streamID uint32, dataLen uint16, err error) {
	_, err = io.ReadFull(sess.rwc, buf[:_FRAME_HEADER_SIZE])
	if err != nil {
		return
	} else if buf[0] != _PROTO_VER {
		err = ERR_PROTO_VERSION
		return
	}

	cmd = buf[1]
	if cmd > _LAST_CMD {
		err = ERR_UNKNOWN_CMD
		return
	}

	streamID = binary.BigEndian.Uint32(buf[2:])
	dataLen = binary.BigEndian.Uint16(buf[6:])

	return
}

func (sess *Session) notifyNewStream(stream *Stream) {
	select {
	case sess.streamChannel <- stream:
	case <-sess.end:
	}
}

func (sess *Session) recvStream() (*Stream, error) {
	select {
	case stream := <-sess.streamChannel:
		return stream, nil
	case <-sess.end:
		return nil, ERR_SESSION_WAS_CLOSED
	}
}

func (sess *Session) recvLoop() {
	buf := make([]byte, _FRAME_HEADER_SIZE, _FRAME_HEADER_SIZE)
	for {
		cmd, streamID, dataLen, err := sess.readFrameHeader(buf)
		if err != nil {
			return
		}

		switch cmd {
		case _CMD_NEW_STREAM:
			if sess.role != _ROLE_SERVER {
				break
			}
			sess.streamsLock.Lock()
			if len(sess.streams) < _MAX_STREAMS_NUM {
				for {
					if _, exist := sess.streams[sess.nextStreamID]; !exist {
						stream := newStream(sess.nextStreamID, sess)
						sess.streams[sess.nextStreamID] = stream
						sess.nextStreamID += 1

						rf := &frame{
							version:  _PROTO_VER,
							cmd:      _CMD_NEW_STREAM_ACK,
							streamID: stream.streamID,
							data:     nil,
						}
						req := newFrameRequest(rf, false)
						req.send(sess, 0)
						go sess.notifyNewStream(stream)
						break
					}
				}
			}
			sess.streamsLock.Unlock()
		case _CMD_NEW_STREAM_ACK:
			if sess.role != _ROLE_CLIENT {
				break
			}

			sess.streamsLock.Lock()
			if _, exist := sess.streams[streamID]; !exist {
				stream := newStream(streamID, sess)
				sess.streams[streamID] = stream
				go sess.notifyNewStream(stream)
			}
			sess.streamsLock.Unlock()
		case _CMD_DATA:
			sess.streamsLock.RLock()
			if stream, exist := sess.streams[streamID]; exist {
				stream.copyToBuffer(sess.rwc, dataLen)
			}
			sess.streamsLock.RUnlock()
		case _CMD_HEARTBEAT:
		case _CMD_STREAM_CLOSE:
			sess.streamsLock.Lock()
			if stream, exist := sess.streams[streamID]; exist {
				delete(sess.streams, streamID)
				stream.terminal(nil)
			}
			sess.streamsLock.Unlock()
		default:
		}
	}
}

func (sess *Session) sendLoop() {
	buf := make([]byte, _FRAME_HEADER_SIZE+_MAX_FRAME_SIZE,
		_FRAME_HEADER_SIZE+_MAX_FRAME_SIZE)
	for {
		select {
		case <-sess.end:
			return
		case req := <-sess.sendChannel:
			buf[0] = req.f.version
			buf[1] = req.f.cmd
			binary.BigEndian.PutUint32(buf[2:], req.f.streamID)
			binary.BigEndian.PutUint16(buf[6:], uint16(len(req.f.data)))

			copy(buf[_FRAME_HEADER_SIZE:], req.f.data)
			n, err := sess.rwc.Write(buf[:_FRAME_HEADER_SIZE+len(req.f.data)])

			if req.ch != nil {
				req.ch <- &rwResult{n, err}
				close(req.ch)
			}
		}
	}
}

func (sess *Session) AcceptStream() (*Stream, error) {
	if sess.role != _ROLE_SERVER {
		return nil, ERR_NOT_SERVER
	}
	if sess.IsClosed() {
		return nil, ERR_SESSION_WAS_CLOSED
	}
	return sess.recvStream()
}

func (sess *Session) OpenStream() (*Stream, error) {
	if sess.role != _ROLE_CLIENT {
		return nil, ERR_NOT_CLIENT
	}
	if sess.IsClosed() {
		return nil, ERR_SESSION_WAS_CLOSED
	}

	req := newFrameRequest(_FRAME_NEW_STREAM, false)
	req.send(sess, 0)
	return sess.recvStream()
}

func (sess *Session) LocalAddr() net.Addr {
	if conn, ok := sess.rwc.(net.Conn); ok {
		return conn.LocalAddr()
	} else {
		return nil
	}
}

func (sess *Session) Addr() net.Addr {
	return sess.LocalAddr()
}

func (sess *Session) RemoteAddr() net.Addr {
	if conn, ok := sess.rwc.(net.Conn); ok {
		return conn.RemoteAddr()
	} else {
		return nil
	}
}

func (sess *Session) Close() error {
	sess.endLock.Lock()
	defer sess.endLock.Unlock()

	if sess.IsClosed() {
		return ERR_SESSION_WAS_CLOSED
	}
	close(sess.end)

	sess.streamsLock.Lock()
	for _, stream := range sess.streams {
		stream.terminal(nil)
	}
	sess.streamsLock.Unlock()

	return nil
}
