package muxstream

const _PROTO_VER = 0x01

// Frame struct
// +-----+-----+----------+-----+----------+
// | VER | CMD | STREAMID | LEN |   DATA   |
// +-----+-----+----------+-----+----------+
// |  1  |  1  |    4     |  2  | Var(LEN) |
// +-----+-----+----------+-----+----------+

// Command
const (
	_CMD_NEW_STREAM = iota
	_CMD_NEW_STREAM_ACK
	_CMD_DATA
	_CMD_STREAM_CLOSE
	_CMD_HEARTBEAT

	_CMD_UNKNOWN = 0xff
)

const _MIN_STREAM_ID = 1
