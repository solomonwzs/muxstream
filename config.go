package muxstream

import "time"

type Config struct {
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
