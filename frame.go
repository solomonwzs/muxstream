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
