package muxstream

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

func (req *channelRequest) Close() error {
	return req.finish(nil, ERR_CH_REQ_WAS_CLOSED)
}

func (req *channelRequest) bGetResponse() (interface{}, error) {
	if d, ok := <-req.responseCh; ok {
		if d == nil {
			return nil, ERR_CH_RES_NIL
		} else {
			return d.data, d.err
		}
	} else {
		return nil, ERR_CH_REQ_WAS_CLOSED
	}
}
