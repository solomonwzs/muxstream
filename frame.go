package muxstream

import (
	"encoding/binary"
	"io"
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

func (f *frame) writeTo(w io.Writer, buf []byte) (n int, err error) {
	// header := make([]byte, _FRAME_HEADER_SIZE, _FRAME_HEADER_SIZE)
	// header[0] = f.version
	// header[1] = f.cmd
	// binary.BigEndian.PutUint32(header[2:], f.streamID)
	// binary.BigEndian.PutUint16(header[6:], uint16(len(f.data)))

	// if _, err = w.Write(header[:_FRAME_HEADER_SIZE]); err != nil {
	// 	return
	// }
	// if n, err = w.Write(f.data); err != nil {
	// 	return _FRAME_HEADER_SIZE + n, err
	// } else {
	// 	return _FRAME_HEADER_SIZE + n, nil
	// }

	buf[0] = f.version
	buf[1] = f.cmd
	binary.BigEndian.PutUint32(buf[2:], f.streamID)
	binary.BigEndian.PutUint16(buf[6:], uint16(len(f.data)))

	copy(buf[_FRAME_HEADER_SIZE:_FRAME_HEADER_SIZE+len(f.data)], f.data)
	n, err = w.Write(buf[:_FRAME_HEADER_SIZE+len(f.data)])

	return
}
