package muxstream

import (
	"time"
)

type dataWithErr struct {
	data interface{}
	err  error
}

type channelRequest struct {
	arg        interface{}
	closed     bool
	responseCh chan *dataWithErr
}

func newChannelRequest(arg interface{}, waitForReturn bool) *channelRequest {
	var ch chan *dataWithErr = nil
	if waitForReturn {
		ch = make(chan *dataWithErr, 1)
	}
	return &channelRequest{
		arg:        arg,
		closed:     false,
		responseCh: ch,
	}
}

func (req *channelRequest) finish(res interface{}, err error) error {
	if req.closed {
		return nil
	}
	req.closed = true
	if req.responseCh != nil {
		req.responseCh <- &dataWithErr{res, err}
		close(req.responseCh)
	}
	return nil
}

func (req *channelRequest) IsClosed() bool {
	return req.closed
}

func (req *channelRequest) Close() error {
	return req.finish(nil, ERR_CH_REQ_WAS_CLOSED)
}

func (req *channelRequest) bGetResponse(timeout time.Duration) (interface{}, error) {
	var deadline <-chan time.Time = nil
	if timeout != 0 {
		deadline = time.After(timeout)
	}

	select {
	case d, ok := <-req.responseCh:
		if ok {
			if d == nil {
				return nil, ERR_CH_RES_NIL
			} else {
				return d.data, d.err
			}
		} else {
			return nil, ERR_CH_REQ_WAS_CLOSED
		}
	case <-deadline:
		return nil, ERR_CH_REQ_TIMEOUT
	}
}
