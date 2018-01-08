package muxstream

import "time"

type Config struct {
	heartbeatDuration time.Time
}

func (conf *Config) SetHeartbeatDuration(t time.Time) *Config {
	conf.heartbeatDuration = t
	return conf
}
