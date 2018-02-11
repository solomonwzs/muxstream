package muxstream

import (
	"time"
)

type readBufferArg struct {
	p       []byte
	timeout time.Duration
}

type Stream struct {
	streamID     uint32
	closed       bool
	session      *Session
	readBuf      [][]byte
	readReqQueue *closerQueue
	eventChannel chan *event
}

func newStream(streamID uint32, s *Session) *Stream {
	stream := &Stream{
		streamID:     streamID,
		closed:       false,
		session:      s,
		readBuf:      [][]byte{},
		readReqQueue: newCloserQueue(),
		eventChannel: make(chan *event, _CHANNEL_SIZE),
	}
	go stream.serv()
	return stream
}

func (stream *Stream) IsClosed() bool {
	return stream.closed
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
		case _EVENT_STREAM_CLOSE_WAIT:
			stream.closed = true
			if len(stream.readBuf) == 0 {
				goto end
			} else {
				newEvent(_EVENT_STREAM_CLOSE_WAIT, nil).sendToAfter(
					stream.eventChannel, 1*time.Second)
			}
		case _EVENT_STREAM_DATA_IN:
			data := e.data.([]byte)
			if len(data) != 0 {
				stream.readBuf = append(stream.readBuf, data)
			}

			for !stream.readReqQueue.isEmpty() && len(stream.readBuf) != 0 {
				req := stream.readReqQueue.pop().(*channelRequest)
				n := stream.writeBufferToReq(req)
				req.finish(n, nil)
			}
		case _EVENT_STREAM_READ_REQ:
			req := e.data.(*channelRequest)
			if stream.readReqQueue.isEmpty() && len(stream.readBuf) != 0 {
				n := stream.writeBufferToReq(req)
				req.finish(n, nil)
			} else {
				stream.readReqQueue.push(req)
				arg := req.arg.(*readBufferArg)
				if arg.timeout > 0 {
					go func() {
						time.Sleep(arg.timeout)
						newEvent(_EVENT_STREAM_READ_REQ_TIMEOUT, req).sendTo(
							stream.eventChannel)
					}()
				}
			}
		case _EVENT_STREAM_READ_REQ_TIMEOUT:
			req := e.data.(*channelRequest)
			req.finish(0, ERR_STREAM_IO_TIMEOUT)
		}
	}
end:
	stream.terminal()
}

func (stream *Stream) writeBufferToReq(req *channelRequest) int {
	i := 0
	arg := req.arg.(*readBufferArg)
	pLen := len(arg.p)
	n := 0
	for i < len(stream.readBuf) && n < pLen {
		bufLen := len(stream.readBuf[i])
		if bufLen > pLen-n {
			copy(arg.p[n:], stream.readBuf[i][:pLen-n])
			n = pLen
			break
		} else {
			copy(arg.p[n:], stream.readBuf[i])
			n += bufLen
			i += 1
		}
	}
	stream.readBuf = stream.readBuf[i:]
	return n
}

func (stream *Stream) newReadReq(p []byte) *channelRequest {
	return newChannelRequest(&readBufferArg{p, -1}, true)
}

func (stream *Stream) Read(p []byte) (int, error) {
	if stream.session.closed {
		return 0, ERR_CLOSED_SESSION
	} else if stream.closed && len(stream.readBuf) == 0 {
		return 0, ERR_CLOSED_STREAM
	}

	req := stream.newReadReq(p)
	newEvent(_EVENT_STREAM_READ_REQ, req).sendTo(stream.eventChannel)
	n0, err := req.bGetResponse()
	n := 0
	if n0 != nil {
		n = n0.(int)
	}
	return n, err
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

func (stream *Stream) writeFrame(f *frame) (int, error) {
	req := newChannelRequest(f, true)
	newEvent(_EVENT_SESSION_SL_SEND_FRAME, req).sendTo(
		stream.session.sendQueue)
	n0, err := req.bGetResponse()
	n := 0
	if n0 != nil {
		n = n0.(int)
	}
	return n, err
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

	stream.readReqQueue.Close()
}
