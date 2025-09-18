# ONVIF Go Library

A comprehensive Go package for ONVIF camera discovery and management.

## Features

- 🔍 **Camera Discovery** - Automatic discovery of ONVIF cameras on the network
- 📊 **Device Information** - Fetch manufacturer, model, serial number, hostname
- ⏰ **Date/Time Management** - Get and set system date/time
- 📹 **Stream Management** - Get stream configurations and update encoder settings
- 🔐 **WS-Security** - Secure authentication with digest passwords
- 🎛️ **Capabilities** - Detect PTZ, Analytics, and other device capabilities

## Installation

```bash
go get github.com/SridarDhandapani/onvif
```

## Quick Start

### Discovery

```go
package main

import (
    "fmt"
    "log"
    "github.com/SridarDhandapani/onvif"
)

func main() {
    // Discover cameras on the network
    cameras, err := onvif.DiscoverCameras(nil)
    if err != nil {
        log.Fatal(err)
    }

    for _, camera := range cameras {
        fmt.Printf("Found: %s at %s\n", camera.GetDisplayName(), camera.Address)
    }
}
```

### With Authentication

```go
// Create authenticated client
client := onvif.NewClient("admin", "password")

// Discover cameras
cameras, err := onvif.DiscoverCameras(nil)
if err != nil {
    log.Fatal(err)
}

// Get detailed information for each camera
for i := range cameras {
    client.GetDeviceInformation(&cameras[i])
    fmt.Printf("Camera: %s %s (S/N: %s)\n",
        cameras[i].Manufacturer,
        cameras[i].DeviceModel,
        cameras[i].SerialNumber)
}
```

## Core Types

### Camera

```go
type Camera struct {
    // Basic discovery info
    Name     string
    Address  string
    Model    string
    Location string

    // Device information
    Manufacturer    string
    DeviceModel     string
    SerialNumber    string
    FirmwareVersion string
    Hostname        string

    // System info
    DateTime string
    TimeZone string

    // Capabilities
    PTZSupport       bool
    AnalyticsSupport bool
    VideoSources     int
    AudioSources     int
}
```

### StreamConfig

```go
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
```

## API Reference

### Discovery

```go
// Basic discovery
cameras, err := onvif.DiscoverCameras(nil)

// With options
options := &onvif.DiscoveryOptions{
    Timeout:       10 * time.Second,
    MulticastAddr: "239.255.255.250:3702",
}
cameras, err := onvif.DiscoverCameras(options)
```

### Client Methods

```go
client := onvif.NewClient("username", "password")

// Device information
err := client.GetDeviceInformation(&camera)
err := client.GetHostname(&camera)
err := client.GetSystemDateTime(&camera)
err := client.GetCapabilities(&camera)

// Date/Time management
err := client.SetSystemDateTime(&camera) // Sets to current GMT

// Stream management
streams, err := client.GetStreamProfiles(&camera)
err := client.UpdateSubStream(&camera, config)
```

### Stream Updates

```go
// Update sub stream to 640x480 @ 10fps
config := onvif.StreamUpdateConfig{
    Resolution: onvif.Resolution640x480,
    Framerate:  10,
    Bitrate:    1024000, // 1 Mbps
    Encoding:   "H264",
}

err := client.UpdateSubStream(&camera, config)
```

## Examples

Check the [examples](./examples/) directory for complete usage examples:

- `discovery.go` - Camera discovery and information retrieval
- `update_stream.go` - Update stream configurations
- `set_datetime.go` - Set camera date/time to GMT

Run examples:

```bash
go run examples/discovery.go -user admin -pass password -detailed
go run examples/update_stream.go -user admin -pass password
go run examples/set_datetime.go -user admin -pass password
```

### Get All Stream Configurations

```go
client := onvif.NewClient("admin", "password")
cameras, _ := onvif.DiscoverCameras(nil)

for _, camera := range cameras {
    streams, err := client.GetStreamProfiles(&camera)
    if err != nil {
        continue
    }

    for _, stream := range streams {
        fmt.Printf("%s: %s @ %dfps (%s)\n",
            stream.ProfileName,
            stream.Resolution,
            stream.Framerate,
            stream.Quality)
        fmt.Printf("  RTSP: %s\n", stream.StreamURI)
    }
}
```

### Set All Cameras to GMT

```go
client := onvif.NewClient("admin", "password")
cameras, _ := onvif.DiscoverCameras(nil)

for _, camera := range cameras {
    if err := client.SetSystemDateTime(&camera); err != nil {
        fmt.Printf("Failed to update %s: %v\n", camera.GetDisplayName(), err)
    } else {
        fmt.Printf("Updated %s to GMT\n", camera.GetDisplayName())
    }
}
```

### Update All Sub Streams

```go
client := onvif.NewClient("admin", "password")
cameras, _ := onvif.DiscoverCameras(nil)

config := onvif.StreamUpdateConfig{
    Resolution: onvif.Resolution640x480,
    Framerate:  10,
    Bitrate:    1024000,
    Encoding:   "H264",
}

for _, camera := range cameras {
    if err := client.UpdateSubStream(&camera, config); err != nil {
        fmt.Printf("Failed: %v\n", err)
    }
}
```

## Helper Functions

```go
// Get best display name for camera
name := camera.GetDisplayName()

// Check stream type
if onvif.IsMainStream(stream) {
    // Handle main stream
}

if onvif.IsSubStream(stream) {
    // Handle sub stream
}

// Get formatted camera info
info := camera.GetCameraInfo()
fmt.Println(info)
```

## Common Resolutions

```go
onvif.Resolution640x480   // VGA
onvif.Resolution1280x720  // HD
onvif.Resolution1920x1080 // Full HD
onvif.Resolution2560x1920 // 5MP
```

## Error Handling

The library returns standard Go errors. Common issues:

- `SOAP fault`: Camera doesn't support the operation or authentication failed
- `timeout`: Network timeout, increase client.Timeout if needed
- `not authorized`: Wrong credentials or insufficient permissions

## Limitations

- Some cameras may not support all ONVIF operations
- GetDeviceInformation may return SOAP faults on some devices
- Stream updates require appropriate permissions

## License

MIT