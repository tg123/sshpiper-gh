package main

type upstreamConfig struct {
	Username       string `yaml:"username,omitempty"`
	Host           string `yaml:"host"`
	Password       string `yaml:"password,omitempty"`
	PrivateKeyData string `yaml:"private_key_data,omitempty"`
	KnownHostsData string `yaml:"known_hosts_data,omitempty"`
	Tags           string `yaml:"tags,omitempty"`
	Repo           string `yaml:"-"`
}

type pipeConfig struct {
	Version   string           `yaml:"version"`
	Upstreams []upstreamConfig `yaml:"upstreams,flow"`
}
