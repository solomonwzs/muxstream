package muxstream

const _PROTO_VER = 0x01

// Frame struct
// +-----+-----+-----+-----+----------+
// | VER | CMD | SID | LEN |   DATA   |
// +-----+-----+-----+-----+----------+
// |  1  |  1  |  4  |  2  | Var(LEN) |
// +-----+-----+-----+-----+----------+

// Command
const (
	_CMD_NEW_SESSION = iota
	_CMD_DATA
	_CMD_SESSION_CLOSE

	_CMD_UNKNOWN = 0xff
)
