package main

import (
	"errors"
	"log"
	"math"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	DEFAULT_LISTENER_PORT       uint16        = 43383
	DEFAULT_STATS_FILENAME      string        = "./stats.json"
	DEFAULT_HEARTBEAT_INTERVAL  time.Duration = 30 * time.Second
	DEFAULT_INACTIVITY_INTERVAL time.Duration = 5 * time.Minute

	ENVVAR_PREFIX string = "ANCHOR"

	LISTENER_ADDRESS_ENVVAR      string = "LISTENER_ADDRESS"
	LISTENER_PORT_ENVVAR         string = "LISTENER_PORT"
	STATS_FILENAME_ENVVAR        string = "STATS_FILENAME"
	HEARTBEAT_INTERVAL_S_ENVVAR  string = "HEARTBEAT_INTERVAL_S"
	INACTIVITY_INTERVAL_S_ENVVAR string = "INACTIVITY_INTERVAL_S"
)

type Configuration struct {
	ListenerAddress    net.IP
	ListenerPort       uint16
	StatsFilename      string
	HeartbeatInterval  time.Duration
	InactivityInterval time.Duration
}

func passthroughString(value string) (string, error) {
	return value, nil
}

func toIP(value string) (net.IP, error) {
	return net.ParseIP(value), nil
}

func toUint16(value string) (uint16, error) {
	intValue, err := strconv.ParseUint(value, 10, 16)

	if err != nil {
		return 0, err
	}

	if intValue > math.MaxUint16 {
		return 0, errors.New("value is greater than 65355")
	}

	return uint16(intValue), nil
}

func toDurationSeconds(value string) (time.Duration, error) {
	seconds, err := strconv.ParseUint(value, 10, 64)

	if err != nil {
		return 0, err
	}

	return time.Duration(seconds) * time.Second, nil
}

func resolveConfigurationValue[T any](envvarKey string, envvarValueAdapter func(string) (T, error), fallbackValue T) T {
	fullEnvvarKey := strings.Join([]string{ENVVAR_PREFIX, envvarKey}, "_")
	envvarValue, isSet := os.LookupEnv(fullEnvvarKey)

	if isSet {
		value, err := envvarValueAdapter(envvarValue)

		if err == nil {
			return value
		}

		log.Printf("Falling back to default value due to failed parse of value for the environment variable '%s': %e", fullEnvvarKey, err)
	}

	return fallbackValue
}

func NewConfiguration() (*Configuration, error) {
	statsFilename, err := filepath.Abs(
		resolveConfigurationValue(STATS_FILENAME_ENVVAR, passthroughString, DEFAULT_STATS_FILENAME),
	)

	if err != nil {
		return nil, err
	}

	return &Configuration{
		ListenerAddress:    resolveConfigurationValue(LISTENER_ADDRESS_ENVVAR, toIP, net.IPv6unspecified), // Listen on all IPv4 and IPv6 by default
		ListenerPort:       resolveConfigurationValue(LISTENER_PORT_ENVVAR, toUint16, DEFAULT_LISTENER_PORT),
		StatsFilename:      statsFilename,
		HeartbeatInterval:  resolveConfigurationValue(HEARTBEAT_INTERVAL_S_ENVVAR, toDurationSeconds, DEFAULT_HEARTBEAT_INTERVAL),
		InactivityInterval: resolveConfigurationValue(INACTIVITY_INTERVAL_S_ENVVAR, toDurationSeconds, DEFAULT_INACTIVITY_INTERVAL),
	}, nil
}

func (c *Configuration) Print() {
	log.Println("Server configuration:")
	log.Printf(" - Listener Address: %v", c.ListenerAddress)
	log.Printf(" - Listener Port: %d", c.ListenerPort)
	log.Printf(" - Stats Filename: %s", c.StatsFilename)
	log.Printf(" - Heartbeat Interval: %v", c.HeartbeatInterval)
	log.Printf(" - Inactivity Interval: %v", c.InactivityInterval)
}

func (c *Configuration) NewTCPAddress() *net.TCPAddr {
	return &net.TCPAddr{
		IP:   c.ListenerAddress,
		Port: int(c.ListenerPort),
	}
}
