package connection

import (
	"errors"
	"net/url"
	"strings"
)

var ErrServerRequired = errors.New("NATS server URL is required")

type Profile struct {
	name   string
	server string
}

func NewProfile(name, server string) (Profile, error) {
	server = strings.TrimSpace(server)
	if server == "" {
		return Profile{}, ErrServerRequired
	}

	if strings.TrimSpace(name) == "" {
		name = profileName(server)
	}

	return Profile{name: name, server: server}, nil
}

func (p Profile) Name() string {
	return p.name
}

func (p Profile) Server() string {
	return p.server
}

func profileName(server string) string {
	parsed, err := url.Parse(server)
	if err == nil && parsed.Host != "" {
		return parsed.Host
	}
	return server
}
