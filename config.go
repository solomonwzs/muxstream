package muxstream

import "time"

const (
	_ROLE_CLIENT = 0x01
	_ROLE_SERVER = 0x02
)

type Config struct {
	role                   uint8
	heartbeatDuration      time.Duration
	networkTimeoutDuration time.Duration
}

func (conf *Config) SetHeartbeatDuration(t time.Duration) *Config {
	conf.heartbeatDuration = t
	return conf
}

func (conf *Config) SetNetworkTimeoutDuration(t time.Duration) *Config {
	conf.networkTimeoutDuration = t
	return conf
}

func (conf *Config) SetServer() *Config {
	conf.role = _ROLE_SERVER
	return conf
}

func (conf *Config) SetClient() *Config {
	conf.role = _ROLE_CLIENT
	return conf
}
