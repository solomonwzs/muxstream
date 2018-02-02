package muxstream

import "time"

type readBufferReq struct {
	len          int
	closed       bool
	dataCh       chan []byte
	errCh        chan error
	timeDuration time.Duration
}

func (req *readBufferReq) finish(err error) error {
	if req.closed {
		return nil
	}

	if err != nil {
		req.errCh <- err
	}

	req.closed = true
	close(req.dataCh)
	close(req.errCh)
	return nil
}

func (req *readBufferReq) Close() error {
	return req.finish(ERR_STREAM_WAS_CLOSED)
}

type Stream struct {
	streamID     uint32
	closed       bool
	session      *Session
	readBuf      [][]byte
	readReqQueue []*readBufferReq
	eventChannel chan *event
}

func newStream(streamID uint32, s *Session) *Stream {
	stream := &Stream{
		streamID:     streamID,
		closed:       false,
		session:      s,
		readBuf:      [][]byte{},
		readReqQueue: []*readBufferReq{},
		eventChannel: make(chan *event, _CHANNEL_SIZE),
	}
	go stream.serv()
	return stream
}

func (stream *Stream) packDataFrame(p []byte) *frame {
	return &frame{
		version:  _PROTO_VER,
		cmd:      _CMD_DATA,
		streamID: stream.streamID,
		data:     p,
	}
}

func (stream *Stream) serv() {
	for e := range stream.eventChannel {
		switch e.typ {
		case _EVENT_STREAM_CLOSE:
			goto end
		case _EVENT_STREAM_DATA_IN:
			data := e.data.([]byte)
			if len(data) != 0 {
				stream.readBuf = append(stream.readBuf, data)
			}

			for !stream.isReadReqQueueEmpty() && len(stream.readBuf) != 0 {
				req := stream.readReqQueue[0]
				stream.sendBufferToReq(req)
				req.finish(nil)
				stream.readReqQueue = stream.readReqQueue[1:]
			}
		case _EVENT_STREAM_READ_REQ:
			req := e.data.(*readBufferReq)
			if stream.isReadReqQueueEmpty() {
				stream.sendBufferToReq(req)
				req.finish(nil)
			} else {
				stream.readReqQueue = append(stream.readReqQueue, req)
				if req.timeDuration > 0 {
					go func() {
						time.Sleep(req.timeDuration)
						newEvent(_EVENT_STREAM_READ_REQ_TIMEOUT, req).sendTo(
							stream.eventChannel)
					}()
				}
			}
		case _EVENT_STREAM_READ_REQ_TIMEOUT:
			req := e.data.(*readBufferReq)
			req.finish(ERR_STREAM_IO_TIMEOUT)
		}
	}
end:
	stream.terminal()
}

func (stream *Stream) sendBufferToReq(req *readBufferReq) {
	i := 0
	for i < len(stream.readBuf) && req.len > 0 {
		bufLen := len(stream.readBuf[i])
		if bufLen > req.len {
			req.dataCh <- stream.readBuf[i][:req.len]
			req.len = 0
			break
		} else {
			req.dataCh <- stream.readBuf[i]
			req.len -= bufLen
			i += 1
		}
	}
	stream.readBuf = stream.readBuf[i:]
}

func (stream *Stream) isReadReqQueueEmpty() bool {
	for i, req := range stream.readReqQueue {
		if req.closed {
			stream.readReqQueue = stream.readReqQueue[i:]
			return false
		}
	}
	return true
}

func (stream *Stream) newReadReq(len int) *readBufferReq {
	return &readBufferReq{
		len:          len,
		closed:       false,
		dataCh:       make(chan []byte, _CHANNEL_SIZE),
		errCh:        make(chan error, 1),
		timeDuration: -1,
	}
}

func (stream *Stream) Read(p []byte) (n int, err error) {
	if stream.session.closed {
		return 0, ERR_CLOSED_SESSION
	} else if stream.closed {
		return 0, ERR_CLOSED_STREAM
	}

	pLen := len(p)
	req := stream.newReadReq(pLen)
	newEvent(_EVENT_STREAM_READ_REQ, req).sendTo(stream.eventChannel)

	n = 0
	err = nil
	for {
		select {
		case buf, ok := <-req.dataCh:
			if !ok {
				goto end
			}
			copy(p[n:], buf)
			n += len(buf)
		case err = <-req.errCh:
			goto end
		}
	}

end:
	return
}

func (stream *Stream) Write(p []byte) (n int, err error) {
	if stream.session.closed {
		return 0, ERR_CLOSED_SESSION
	} else if stream.closed {
		return 0, ERR_CLOSED_STREAM
	}

	fs := newDataFrames(stream.streamID, p)
	n = 0
	err = nil
	for _, f := range fs {
		if n0, err0 := stream.writeFrame(f); err0 == nil {
			n += n0
		} else {
			return n + n0, err0
		}
	}
	return
}

func (stream *Stream) writeFrame(f *frame) (n int, err error) {
	sReq := newSendFrameReq(f, true)
	stream.session.sendQueue <- sReq
	if res, ok := <-sReq.ch; ok {
		return res.n, res.err
	} else {
		return 0, ERR_SEND_WAS_INTERED
	}
}

func (stream *Stream) Close() (err error) {
	newEvent(_EVENT_STREAM_CLOSE, nil).sendTo(stream.eventChannel)
	return
}

func (stream *Stream) terminal() {
	stream.closed = true
	newEvent(_EVENT_STREAM_TERMINAL, stream.streamID).sendTo(
		stream.session.eventChannel)

	go waitForEventChannelClean(stream.eventChannel)

	for _, req := range stream.readReqQueue {
		req.Close()
	}
}
