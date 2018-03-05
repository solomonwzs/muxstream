package muxstream

import "time"

const (
	_ROLE_CLIENT = 0x01
	_ROLE_SERVER = 0x02
)

type Config struct {
	maxFrameSize           int
	heartbeatDuration      time.Duration
	networkTimeoutDuration time.Duration
}

func DefaultConfig() *Config {
	return &Config{
		maxFrameSize:           32 * 1024,
		heartbeatDuration:      0,
		networkTimeoutDuration: 0,
	}
}

func (conf *Config) SetHeartbeatDuration(t time.Duration) *Config {
	conf.heartbeatDuration = t
	return conf
}

func (conf *Config) SetNetworkTimeoutDuration(t time.Duration) *Config {
	conf.networkTimeoutDuration = t
	return conf
}

func (conf *Config) SetMaxFrameSize(size int) *Config {
	if size <= _MAX_FRAME_SIZE {
		conf.maxFrameSize = size
	}
	return conf
}
