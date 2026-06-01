package onvif

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// profileXML is the parsed form of a single <Profiles> element from a
// GetProfiles response, including the full video encoder configuration.
type profileXML struct {
	Token string          `xml:"token,attr"`
	Name  string          `xml:"Name"`
	VEC   videoEncoderXML `xml:"VideoEncoderConfiguration"`
}

// videoEncoderXML captures every VideoEncoderConfiguration field needed to round
// trip a SetVideoEncoderConfiguration request. SetVideoEncoderConfiguration
// replaces the whole configuration, so to change a few fields we must read the
// existing one and send it back intact — partial or fabricated configurations
// are rejected (HTTP 400) by stricter cameras such as Panasonic.
type videoEncoderXML struct {
	Token      string `xml:"token,attr"`
	Name       string `xml:"Name"`
	UseCount   int    `xml:"UseCount"`
	Encoding   string `xml:"Encoding"`
	Resolution struct {
		Width  int `xml:"Width"`
		Height int `xml:"Height"`
	} `xml:"Resolution"`
	Quality     float32 `xml:"Quality"`
	RateControl struct {
		FrameRateLimit   int `xml:"FrameRateLimit"`
		EncodingInterval int `xml:"EncodingInterval"`
		BitrateLimit     int `xml:"BitrateLimit"`
	} `xml:"RateControl"`
	H264 struct {
		GovLength   int    `xml:"GovLength"`
		H264Profile string `xml:"H264Profile"`
	} `xml:"H264"`
	Multicast struct {
		Address struct {
			Type        string `xml:"Type"`
			IPv4Address string `xml:"IPv4Address"`
		} `xml:"Address"`
		Port      int  `xml:"Port"`
		TTL       int  `xml:"TTL"`
		AutoStart bool `xml:"AutoStart"`
	} `xml:"Multicast"`
	SessionTimeout string `xml:"SessionTimeout"`
}

// getProfiles fetches and parses the media profiles, including each profile's
// full video encoder configuration.
func (c *Client) getProfiles(camera *Camera) ([]profileXML, error) {
	mediaURL := c.resolveMediaURL(camera)

	profilesResp, err := c.sendSOAPRequest(mediaURL,
		"http://www.onvif.org/ver10/media/wsdl/GetProfiles", `<trt:GetProfiles/>`)
	if err != nil {
		return nil, fmt.Errorf("failed to get profiles: %v", err)
	}

	var parsed struct {
		Profiles []profileXML `xml:"Body>GetProfilesResponse>Profiles"`
	}
	if err := xml.Unmarshal(profilesResp, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse profiles: %v", err)
	}
	return parsed.Profiles, nil
}

// GetStreamProfiles fetches all stream profiles for a camera
func (c *Client) GetStreamProfiles(camera *Camera) ([]StreamConfig, error) {
	profiles, err := c.getProfiles(camera)
	if err != nil {
		return nil, err
	}
	mediaURL := c.resolveMediaURL(camera)

	var streamConfigs []StreamConfig

	// Process each profile
	for _, profile := range profiles {
		if profile.VEC.Token == "" {
			continue // Skip profiles without video configuration
		}

		// Get stream URI
		streamBody := fmt.Sprintf(`
		<trt:GetStreamUri>
			<trt:StreamSetup>
				<tt:Stream xmlns:tt="http://www.onvif.org/ver10/schema">RTP-Unicast</tt:Stream>
				<tt:Transport xmlns:tt="http://www.onvif.org/ver10/schema">
					<tt:Protocol>RTSP</tt:Protocol>
				</tt:Transport>
			</trt:StreamSetup>
			<trt:ProfileToken>%s</trt:ProfileToken>
		</trt:GetStreamUri>`, profile.Token)

		streamResp, err := c.sendSOAPRequest(mediaURL,
			"http://www.onvif.org/ver10/media/wsdl/GetStreamUri", streamBody)

		streamURI := ""
		if err == nil {
			type StreamUriResponse struct {
				MediaUri struct {
					Uri string `xml:"Uri"`
				} `xml:"Body>GetStreamUriResponse>MediaUri"`
			}

			var streamUri StreamUriResponse
			if xml.Unmarshal(streamResp, &streamUri) == nil {
				streamURI = streamUri.MediaUri.Uri
			}
		}

		// Determine quality based on resolution
		quality := "Sub"
		if profile.VEC.Resolution.Width >= 1280 {
			quality = "Main"
		}

		config := StreamConfig{
			ProfileName:  profile.Name,
			ProfileToken: profile.Token,
			EncoderToken: profile.VEC.Token,
			Resolution: fmt.Sprintf("%dx%d",
				profile.VEC.Resolution.Width,
				profile.VEC.Resolution.Height),
			Framerate: profile.VEC.RateControl.FrameRateLimit,
			Bitrate:   profile.VEC.RateControl.BitrateLimit,
			Encoding:  profile.VEC.Encoding,
			StreamURI: streamURI,
			Quality:   quality,
		}

		streamConfigs = append(streamConfigs, config)
	}

	return streamConfigs, nil
}

// mediaURLHeuristic derives a likely Media (ver10) service URL from the
// device-service address. Used only when service discovery reported none.
func mediaURLHeuristic(address string) string {
	url := strings.Replace(address, "/device_service", "/media_service", 1)
	if !strings.Contains(url, "media_service") {
		url = strings.Replace(address, "/onvif/device_service", "/onvif/media_service", 1)
	}
	return url
}

// GetStreamUri retrieves the RTSP stream URI for a given profile token
func (c *Client) GetStreamUri(camera *Camera, profileToken string) (string, error) {
	mediaURL := c.resolveMediaURL(camera)

	body := fmt.Sprintf(`<trt:GetStreamUri>
		<trt:StreamSetup>
			<tt:Stream xmlns:tt="http://www.onvif.org/ver10/schema">RTP-Unicast</tt:Stream>
			<tt:Transport xmlns:tt="http://www.onvif.org/ver10/schema">
				<tt:Protocol>RTSP</tt:Protocol>
			</tt:Transport>
		</trt:StreamSetup>
		<trt:ProfileToken>%s</trt:ProfileToken>
	</trt:GetStreamUri>`, profileToken)

	resp, err := c.sendSOAPRequest(mediaURL,
		"http://www.onvif.org/ver10/media/wsdl/GetStreamUri", body)
	if err != nil {
		return "", fmt.Errorf("failed to get stream URI: %v", err)
	}

	if err := parseSOAPFault(resp); err != nil {
		return "", err
	}

	// Try structured parsing
	type StreamUriResponse struct {
		MediaUri struct {
			Uri string `xml:"Uri"`
		} `xml:"Body>GetStreamUriResponse>MediaUri"`
	}

	var streamUri StreamUriResponse
	if err := xml.Unmarshal(resp, &streamUri); err == nil && streamUri.MediaUri.Uri != "" {
		return streamUri.MediaUri.Uri, nil
	}

	return "", fmt.Errorf("no stream URI found in response")
}

// findVideoEncoderConfig returns the current video encoder configuration with
// the given token, read from the device's profiles.
func (c *Client) findVideoEncoderConfig(camera *Camera, encoderToken string) (videoEncoderXML, error) {
	profiles, err := c.getProfiles(camera)
	if err != nil {
		return videoEncoderXML{}, err
	}
	for _, p := range profiles {
		if p.VEC.Token == encoderToken {
			return p.VEC, nil
		}
	}
	return videoEncoderXML{}, fmt.Errorf("video encoder configuration %q not found", encoderToken)
}

// buildSetVideoEncoderBody renders a SetVideoEncoderConfiguration request body
// from a full video encoder configuration. The element order follows the ONVIF
// schema (Name, UseCount, Encoding, Resolution, Quality, RateControl, codec,
// Multicast, SessionTimeout).
func buildSetVideoEncoderBody(v videoEncoderXML) string {
	encInterval := v.RateControl.EncodingInterval
	if encInterval <= 0 {
		encInterval = 1
	}

	// Codec-specific block (H264). Omitted for other encodings so we don't send
	// an H264 block for an H265/MJPEG stream.
	codecBlock := ""
	if strings.EqualFold(v.Encoding, "H264") {
		gov := v.H264.GovLength
		if gov <= 0 {
			gov = 30
		}
		profile := v.H264.H264Profile
		if profile == "" {
			profile = "Main"
		}
		codecBlock = fmt.Sprintf(`
			<tt:H264 xmlns:tt="http://www.onvif.org/ver10/schema">
				<tt:GovLength>%d</tt:GovLength>
				<tt:H264Profile>%s</tt:H264Profile>
			</tt:H264>`, gov, profile)
	}

	mcastType := v.Multicast.Address.Type
	if mcastType == "" {
		mcastType = "IPv4"
	}
	mcastAddr := v.Multicast.Address.IPv4Address
	if mcastAddr == "" {
		mcastAddr = "0.0.0.0"
	}
	sessionTimeout := v.SessionTimeout
	if sessionTimeout == "" {
		sessionTimeout = "PT60S"
	}
	name := v.Name
	if name == "" {
		name = "Configuration"
	}

	return fmt.Sprintf(`
	<trt:SetVideoEncoderConfiguration>
		<trt:Configuration token="%s">
			<tt:Name xmlns:tt="http://www.onvif.org/ver10/schema">%s</tt:Name>
			<tt:UseCount xmlns:tt="http://www.onvif.org/ver10/schema">%d</tt:UseCount>
			<tt:Encoding xmlns:tt="http://www.onvif.org/ver10/schema">%s</tt:Encoding>
			<tt:Resolution xmlns:tt="http://www.onvif.org/ver10/schema">
				<tt:Width>%d</tt:Width>
				<tt:Height>%d</tt:Height>
			</tt:Resolution>
			<tt:Quality xmlns:tt="http://www.onvif.org/ver10/schema">%g</tt:Quality>
			<tt:RateControl xmlns:tt="http://www.onvif.org/ver10/schema">
				<tt:FrameRateLimit>%d</tt:FrameRateLimit>
				<tt:EncodingInterval>%d</tt:EncodingInterval>
				<tt:BitrateLimit>%d</tt:BitrateLimit>
			</tt:RateControl>%s
			<tt:Multicast xmlns:tt="http://www.onvif.org/ver10/schema">
				<tt:Address>
					<tt:Type>%s</tt:Type>
					<tt:IPv4Address>%s</tt:IPv4Address>
				</tt:Address>
				<tt:Port>%d</tt:Port>
				<tt:TTL>%d</tt:TTL>
				<tt:AutoStart>%t</tt:AutoStart>
			</tt:Multicast>
			<tt:SessionTimeout xmlns:tt="http://www.onvif.org/ver10/schema">%s</tt:SessionTimeout>
		</trt:Configuration>
		<trt:ForcePersistence>true</trt:ForcePersistence>
	</trt:SetVideoEncoderConfiguration>`,
		v.Token, name, v.UseCount, v.Encoding,
		v.Resolution.Width, v.Resolution.Height,
		v.Quality,
		v.RateControl.FrameRateLimit, encInterval, v.RateControl.BitrateLimit,
		codecBlock,
		mcastType, mcastAddr, v.Multicast.Port, v.Multicast.TTL, v.Multicast.AutoStart,
		sessionTimeout)
}

// UpdateStreamConfiguration updates a stream's video encoder configuration.
//
// It reads the existing configuration for encoderToken and overrides only the
// fields supplied in config (encoding, resolution, framerate, bitrate),
// preserving everything else the camera reported (quality, codec profile/GOV,
// multicast, session timeout). This round-trip is required because
// SetVideoEncoderConfiguration replaces the whole configuration and stricter
// cameras reject partial or invented values.
func (c *Client) UpdateStreamConfiguration(camera *Camera, encoderToken string, config StreamUpdateConfig) error {
	mediaURL := c.resolveMediaURL(camera)

	existing, err := c.findVideoEncoderConfig(camera, encoderToken)
	if err != nil {
		return err
	}

	// Apply overrides (only non-zero fields).
	if config.Encoding != "" {
		existing.Encoding = config.Encoding
	}
	if config.Resolution.Width > 0 && config.Resolution.Height > 0 {
		existing.Resolution.Width = config.Resolution.Width
		existing.Resolution.Height = config.Resolution.Height
	}
	if config.Framerate > 0 {
		existing.RateControl.FrameRateLimit = config.Framerate
	}
	if config.Bitrate > 0 {
		existing.RateControl.BitrateLimit = config.Bitrate
	}

	updateResp, err := c.sendSOAPRequest(mediaURL,
		"http://www.onvif.org/ver10/media/wsdl/SetVideoEncoderConfiguration",
		buildSetVideoEncoderBody(existing))
	if err != nil {
		return fmt.Errorf("failed to update configuration: %v", err)
	}

	if err := parseSOAPFault(updateResp); err != nil {
		return err
	}

	return nil
}

// UpdateSubStream finds and updates the sub stream to specified configuration
func (c *Client) UpdateSubStream(camera *Camera, config StreamUpdateConfig) error {
	streams, err := c.GetStreamProfiles(camera)
	if err != nil {
		return fmt.Errorf("failed to get stream profiles: %v", err)
	}

	// Find the sub stream (usually the second stream or one with lower resolution)
	var subStream *StreamConfig
	for i := range streams {
		if streams[i].Quality == "Sub" ||
			(len(streams) >= 2 && i == 1) ||
			strings.Contains(strings.ToLower(streams[i].ProfileName), "stream2") {
			subStream = &streams[i]
			break
		}
	}

	if subStream == nil {
		return fmt.Errorf("no sub stream found")
	}
	if subStream.EncoderToken == "" {
		return fmt.Errorf("sub stream %q has no encoder configuration token", subStream.ProfileName)
	}

	return c.UpdateStreamConfiguration(camera, subStream.EncoderToken, config)
}
