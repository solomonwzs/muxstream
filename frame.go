package muxstream

import (
	"bytes"
	"encoding/binary"
)

type frame struct {
	version byte
	cmd     byte
	sid     uint32
	data    []byte
}

func newFrame(sid uint32, cmd byte) *frame {
	return &frame{
		version: _PROTO_VER,
		cmd:     cmd,
		sid:     sid,
		data:    nil,
	}
}

func (f *frame) encode() []byte {
	buf := new(bytes.Buffer)
	buf.Write([]byte{f.version, f.cmd})
	binary.Write(buf, binary.BigEndian, f.sid)
	binary.Write(buf, binary.BigEndian, uint16(len(f.data)))
	buf.Write(f.data)

	return buf.Bytes()
}
