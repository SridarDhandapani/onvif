# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Go library for ONVIF (Open Network Video Interface Forum) IP camera discovery and management. Pure Go implementation with zero external dependencies.

## Build and Run Commands

```bash
# Build/verify the library compiles
go build .

# Run examples (each example is a standalone program)
go run examples/discovery.go
go run examples/set_datetime.go -user <username> -pass <password>
go run examples/update_stream.go -user <username> -pass <password>
go run examples/user_management.go -camera <ip> -init -new-user admin -new-pass <password>

# Manage dependencies
go mod tidy
```

Note: No tests currently exist in the codebase.

## Architecture

**Single-package library** (`github.com/SridarDhandapani/onvif`) implementing ONVIF Profile S over SOAP/HTTP.

### Protocol Flow

```
DiscoverCameras() → UDP Multicast (239.255.255.250:3702) → WS-Discovery Probe
                                     ↓
                            Parse XML Responses → Deduplicate → []Camera

Client.Method() → WS-Security Header (SHA-1 digest auth) → SOAP Envelope → HTTP POST
                                     ↓
                            Parse XML Response → Structured Data
```

### Core Files

| File | Responsibility |
|------|----------------|
| `discovery.go` | WS-Discovery multicast probing, camera enumeration |
| `soap.go` | SOAP envelope construction, WS-Security password digest, HTTP transport |
| `device.go` | Device queries (info, hostname, time, capabilities) |
| `media.go` | Stream profile retrieval, encoder configuration updates |
| `user.go` | User management (GetUsers, CreateUsers, SetUser, DeleteUsers) |
| `client.go` | Client initialization, Camera display helpers |
| `types.go` | Camera, StreamConfig, VideoEncoderConfig, Client, User, UserLevel structs |

### Key Implementation Details

- **WS-Security Authentication**: Password digest (not plaintext) using SHA-1 with nonce and timestamp
- **Dual XML Parsing**: Structured unmarshaling with fallback to string extraction when schemas vary
- **Service URL Conversion**: Automatically maps device service URLs to media service URLs (`/device_service` → `/Media`)
- **Timeout Configuration**: Per-client configurable (default 10 seconds)

### Public API Entry Points

- `DiscoverCameras(options)` - Find cameras on network via WS-Discovery
- `NewClient(username, password)` / `NewClientWithTimeout(...)` - Create authenticated client (empty credentials for anonymous access)
- Device methods: `GetDeviceInformation()`, `GetHostname()`, `GetSystemDateTime()`, `SetSystemDateTime()`, `GetCapabilities()`
- Media methods: `GetStreamProfiles()`, `UpdateStreamConfiguration()`, `UpdateSubStream()`
- User methods: `GetUsers()`, `CreateUsers()`, `CreateUser()`, `SetUser()`, `SetUserPassword()`, `DeleteUsers()`, `DeleteUser()`
