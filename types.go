// Package onvif provides ONVIF camera discovery and management functionality
package onvif

import (
	"time"
)

// Camera represents an ONVIF-compliant camera with all its properties
type Camera struct {
	// From discovery
	Name     string
	Address  string
	Profiles []string
	Model    string
	Location string

	// From GetHostname
	Hostname     string
	HostnameFrom string // "DHCP" or "Manual"

	// From GetDeviceInformation
	Manufacturer    string
	DeviceModel     string
	FirmwareVersion string
	SerialNumber    string
	HardwareId      string

	// From GetSystemDateAndTime
	TimeZone string
	DateTime string

	// From GetCapabilities
	VideoSources     int
	VideoOutputs     int
	AudioSources     int
	AudioOutputs     int
	RelayOutputs     int
	PTZSupport       bool
	AnalyticsSupport bool
}

// StreamConfig represents a video stream configuration
type StreamConfig struct {
	ProfileName  string
	ProfileToken string
	Resolution   string
	Framerate    int
	Bitrate      int
	Encoding     string
	StreamURI    string
	Quality      string // "Main" or "Sub"
}

// VideoEncoderConfig represents video encoder configuration
type VideoEncoderConfig struct {
	Token            string
	Name             string
	Encoding         string
	Width            int
	Height           int
	FrameRateLimit   int
	BitrateLimit     int
	EncodingInterval int
	Quality          float32
	ProfileToken     string
	ProfileName      string
}

// Client represents an ONVIF client with authentication
type Client struct {
	Username string
	Password string
	Timeout  time.Duration
}

// DiscoveryOptions provides options for camera discovery
type DiscoveryOptions struct {
	Timeout         time.Duration
	MulticastAddr   string
	FetchDetails    bool // Whether to fetch device information during discovery
	FetchHostname   bool
	FetchCapabilities bool
}

// StreamUpdateConfig specifies target configuration for stream updates
type StreamUpdateConfig struct {
	Resolution Resolution
	Framerate  int
	Bitrate    int
	Encoding   string
}

// Resolution represents video resolution
type Resolution struct {
	Width  int
	Height int
}

// Common resolutions
var (
	Resolution640x480   = Resolution{640, 480}
	Resolution1280x720  = Resolution{1280, 720}
	Resolution1920x1080 = Resolution{1920, 1080}
	Resolution2560x1920 = Resolution{2560, 1920}
)

// Default configuration
const (
	DefaultMulticastAddr = "239.255.255.250:3702"
	DefaultTimeout       = 5 * time.Second
)