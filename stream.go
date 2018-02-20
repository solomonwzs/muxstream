package muxstream

import (
	"bytes"
	"net"
	"sync/atomic"
	"time"
)

type readBufferArg struct {
	p       []byte
	timeout time.Duration
}

type Stream struct {
	flagClosed
	streamID      uint32
	session       *Session
	readBuf       *bytes.Buffer
	readReqQueue  *closerQueue
	eventChannel  chan *event
	readDeadline  atomic.Value
	writeDeadline atomic.Value
}

func newStream(streamID uint32, s *Session) *Stream {
	stream := &Stream{
		streamID:     streamID,
		flagClosed:   false,
		session:      s,
		readBuf:      new(bytes.Buffer),
		readReqQueue: newCloserQueue(),
		eventChannel: make(chan *event, _CHANNEL_SIZE),
	}
	stream.readDeadline.Store(time.Time{})
	stream.writeDeadline.Store(time.Time{})
	go stream.serv()
	return stream
}

func (stream *Stream) serv() {
	for e := range stream.eventChannel {
		switch e.typ {
		case _EVENT_STREAM_CLOSE:
			goto end
		case _EVENT_STREAM_CLOSE_WAIT:
			stream.flagClosed = true
			if stream.readBuf.Len() == 0 {
				goto end
			} else {
				go newEvent(_EVENT_STREAM_CLOSE_WAIT, nil).sendToAfter(
					stream.eventChannel, 1*time.Second)
			}
		case _EVENT_STREAM_DATA_IN:
			dReq := e.data.(*channelRequest)
			data := dReq.arg.([]byte)
			i := 0
			for !stream.readReqQueue.isEmpty() && i < len(data) {
				req := stream.readReqQueue.pop().(*channelRequest)
				n := copy(req.arg.(*readBufferArg).p, data[i:])
				req.finish(n, nil)
				i += n
			}
			if i < len(data) {
				stream.readBuf.Write(data[i:])
			}
			dReq.finish(struct{}{}, nil)
		case _EVENT_STREAM_READ_REQ:
			req := e.data.(*channelRequest)
			if stream.readReqQueue.isEmpty() && stream.readBuf.Len() != 0 {
				n, err := stream.readBuf.Read(req.arg.(*readBufferArg).p)
				req.finish(n, err)
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

func (stream *Stream) newReadReq(p []byte) (*channelRequest, error) {
	var timeout time.Duration = 0
	if t, ok := stream.readDeadline.Load().(time.Time); ok && !t.IsZero() {
		timeout = time.Until(t)
		if timeout <= 0 {
			return nil, ERR_STREAM_IO_TIMEOUT
		}
	}
	return newChannelRequest(&readBufferArg{p, timeout}, true), nil
}

func (stream *Stream) Read(p []byte) (int, error) {
	if stream.session.flagClosed {
		return 0, ERR_CLOSED_SESSION
	} else if stream.flagClosed && stream.readBuf.Len() == 0 {
		return 0, ERR_CLOSED_STREAM
	}

	req, err := stream.newReadReq(p)
	if err != nil {
		return 0, err
	}
	newEvent(_EVENT_STREAM_READ_REQ, req).sendTo(stream.eventChannel)
	n0, err := req.bGetResponse(0)
	n := 0
	if n0 != nil {
		n = n0.(int)
	}
	return n, err
}

func (stream *Stream) Write(p []byte) (n int, err error) {
	if stream.session.flagClosed {
		return 0, ERR_CLOSED_SESSION
	} else if stream.flagClosed {
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
	var timeout time.Duration = 0
	t, ok := stream.writeDeadline.Load().(time.Time)
	if ok && !t.IsZero() {
		timeout = time.Until(t)
		if timeout <= 0 {
			return 0, ERR_STREAM_IO_TIMEOUT
		}
	}

	req := newChannelRequest(f, true)
	newEvent(_EVENT_SESSION_SL_SEND_FRAME, req).sendTo(
		stream.session.sendQueue)
	n0, err := req.bGetResponse(timeout)
	n := 0
	if n0 != nil {
		n = n0.(int)
	}
	if err == ERR_CH_REQ_TIMEOUT {
		err = ERR_STREAM_IO_TIMEOUT
	}
	return n, err
}

func (stream *Stream) Close() (err error) {
	if stream.flagClosed {
		return ERR_STREAM_WAS_CLOSED
	}
	newEvent(_EVENT_STREAM_CLOSE, nil).sendTo(stream.eventChannel)
	return
}

func (stream *Stream) LocalAddr() net.Addr {
	return stream.session.LocalAddr()
}

func (stream *Stream) RemoteAddr() net.Addr {
	return stream.session.RemoteAddr()
}

func (stream *Stream) SetReadDeadline(t time.Time) error {
	stream.readDeadline.Store(t)
	return nil
}

func (stream *Stream) SetWriteDeadline(t time.Time) error {
	stream.writeDeadline.Store(t)
	return nil
}

func (stream *Stream) SetDeadline(t time.Time) error {
	if err := stream.SetReadDeadline(t); err != nil {
		return err
	}
	if err := stream.SetWriteDeadline(t); err != nil {
		return err
	}
	return nil
}

func (stream *Stream) terminal() {
	stream.flagClosed = true
	newEvent(_EVENT_STREAM_TERMINAL, stream.streamID).sendTo(
		stream.session.eventChannel)

	go waitForEventChannelClean(stream.eventChannel)

	stream.readReqQueue.Close()
}
