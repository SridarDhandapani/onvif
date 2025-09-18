// Package onvif provides a Go library for ONVIF camera discovery and management
package onvif

import (
	"fmt"
	"strings"
	"time"
)

// NewClient creates a new ONVIF client with credentials
func NewClient(username, password string) *Client {
	return &Client{
		Username: username,
		Password: password,
		Timeout:  10 * time.Second,
	}
}

// NewClientWithTimeout creates a new ONVIF client with custom timeout
func NewClientWithTimeout(username, password string, timeout time.Duration) *Client {
	return &Client{
		Username: username,
		Password: password,
		Timeout:  timeout,
	}
}

// GetCameraInfo returns a formatted string with camera information
func (camera *Camera) GetCameraInfo() string {
	info := fmt.Sprintf("Camera: %s\n", camera.GetDisplayName())
	info += fmt.Sprintf("  Address: %s\n", camera.Address)

	if camera.Manufacturer != "" {
		info += fmt.Sprintf("  Manufacturer: %s\n", camera.Manufacturer)
	}

	if camera.DeviceModel != "" {
		info += fmt.Sprintf("  Model: %s\n", camera.DeviceModel)
	} else if camera.Model != "" {
		info += fmt.Sprintf("  Model: %s\n", camera.Model)
	}

	if camera.SerialNumber != "" {
		info += fmt.Sprintf("  Serial: %s\n", camera.SerialNumber)
	}

	if camera.Hostname != "" {
		info += fmt.Sprintf("  Hostname: %s\n", camera.Hostname)
	}

	return info
}

// GetDisplayName returns the best available name for the camera
func (camera *Camera) GetDisplayName() string {
	// Priority: Manufacturer + Model > Hostname > Discovery Name > Model > Address
	if camera.Manufacturer != "" && camera.DeviceModel != "" {
		return fmt.Sprintf("%s %s", camera.Manufacturer, camera.DeviceModel)
	}

	if camera.Hostname != "" {
		return camera.Hostname
	}

	if camera.Name != "" {
		return camera.Name
	}

	if camera.Model != "" {
		return camera.Model
	}

	return getFirstAddress(camera.Address)
}

// IsMainStream checks if a stream configuration is likely the main stream
func IsMainStream(config StreamConfig) bool {
	return config.Quality == "Main" ||
		strings.Contains(strings.ToLower(config.ProfileName), "main") ||
		strings.Contains(strings.ToLower(config.ProfileName), "stream1")
}

// IsSubStream checks if a stream configuration is likely the sub stream
func IsSubStream(config StreamConfig) bool {
	return config.Quality == "Sub" ||
		strings.Contains(strings.ToLower(config.ProfileName), "sub") ||
		strings.Contains(strings.ToLower(config.ProfileName), "stream2")
}