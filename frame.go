package muxstream

import (
	"encoding/binary"
)

const (
	_FRAME_HEADER_SIZE = 8
)

type frame struct {
	version  byte
	cmd      byte
	streamID uint32
	data     []byte
}

func (f *frame) raw(p []byte) int {
	p[0] = f.version
	p[1] = f.cmd
	binary.LittleEndian.PutUint32(p[2:], f.streamID)
	binary.LittleEndian.PutUint16(p[6:], uint16(len(f.data)))
	copy(p[_FRAME_HEADER_SIZE:], f.data)

	return _FRAME_HEADER_SIZE + len(f.data)
}

type frameRaw []byte

func (raw frameRaw) version() byte {
	return raw[0]
}

func (raw frameRaw) cmd() byte {
	return raw[1]
}

func (raw frameRaw) streamID() uint32 {
	return binary.LittleEndian.Uint32(raw[2:])
}

func (raw frameRaw) dataLen() uint16 {
	return binary.LittleEndian.Uint16(raw[6:])
}
