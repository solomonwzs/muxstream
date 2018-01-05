package muxstream

const PROTO_VER = 0x01

// Frame struct
// +-----+-----+-----+-----+----------+
// | VER | SID | CMD | LEN |   DATA   |
// +-----+-----+-----+-----+----------+
// |  1  |  4  |  1  |  2  | Var(LEN) |
// +-----+-----+-----+-----+----------+

// Command
const (
	CMD_NEW_SESSION = iota
	CMD_NEW_SESSION_ACK
	CMD_DATA

	CMD_UNKNOWN = 0xff
)
