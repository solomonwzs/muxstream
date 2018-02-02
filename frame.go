package muxstream

import (
	"bytes"
	"encoding/binary"
)

type frame struct {
	version  byte
	cmd      byte
	streamID uint32
	data     []byte
}

func newDataFrames(streamID uint32, data []byte) []*frame {
	dataLen := len(data)
	if dataLen == 0 {
		return nil
	}

	n := dataLen / _MAX_BUFFER_SIZE
	if dataLen%_MAX_BUFFER_SIZE != 0 {
		n += 1
	}
	fs := make([]*frame, n, n)
	for i := 0; i < n; i++ {
		var p []byte
		if i < n-1 {
			p = data[i*_MAX_BUFFER_SIZE : (i+1)*_MAX_BUFFER_SIZE]
		} else {
			p = data[i*_MAX_BUFFER_SIZE:]
		}
		fs[i] = &frame{
			version:  _PROTO_VER,
			cmd:      _CMD_DATA,
			streamID: streamID,
			data:     p,
		}
	}
	return fs
}

func (f *frame) encode() (buf *bytes.Buffer) {
	buf = new(bytes.Buffer)
	buf.Write([]byte{f.version, f.cmd})

	if f.streamID < _MIN_STREAM_ID {
		return
	}
	binary.Write(buf, binary.BigEndian, f.streamID)

	if len(f.data) == 0 {
		return
	}

	binary.Write(buf, binary.BigEndian, uint16(len(f.data)))
	buf.Write(f.data)

	return
}
