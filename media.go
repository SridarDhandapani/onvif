package onvif

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
)

// GetStreamProfiles fetches all stream profiles for a camera
func (c *Client) GetStreamProfiles(camera *Camera) ([]StreamConfig, error) {
	// Discover service URLs if not already cached
	if camera.MediaURL == "" {
		c.GetCapabilities(camera)
	}
	mediaURL := resolveMediaURL(camera)

	// Get profiles
	profilesBody := `<trt:GetProfiles/>`
	profilesResp, err := c.sendSOAPRequest(mediaURL,
		"http://www.onvif.org/ver10/media/wsdl/GetProfiles", profilesBody)
	if err != nil {
		return nil, fmt.Errorf("failed to get profiles: %v", err)
	}

	// Parse profiles response
	type ProfilesResponse struct {
		Profiles []struct {
			Token string `xml:"token,attr"`
			Name  string `xml:"Name"`
			VideoEncoderConfiguration struct {
				Token    string `xml:"token,attr"`
				Name     string `xml:"Name"`
				Encoding string `xml:"Encoding"`
				Resolution struct {
					Width  int `xml:"Width"`
					Height int `xml:"Height"`
				} `xml:"Resolution"`
				RateControl struct {
					FrameRateLimit   int `xml:"FrameRateLimit"`
					BitrateLimit     int `xml:"BitrateLimit"`
					EncodingInterval int `xml:"EncodingInterval"`
				} `xml:"RateControl"`
				Quality float32 `xml:"Quality"`
			} `xml:"VideoEncoderConfiguration"`
		} `xml:"Body>GetProfilesResponse>Profiles"`
	}

	var profiles ProfilesResponse
	if err := xml.Unmarshal(profilesResp, &profiles); err != nil {
		return nil, fmt.Errorf("failed to parse profiles: %v", err)
	}

	var streamConfigs []StreamConfig

	// Process each profile
	for _, profile := range profiles.Profiles {
		if profile.VideoEncoderConfiguration.Token == "" {
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
		if profile.VideoEncoderConfiguration.Resolution.Width >= 1920 {
			quality = "Main"
		} else if profile.VideoEncoderConfiguration.Resolution.Width >= 1280 {
			quality = "Main"
		}

		config := StreamConfig{
			ProfileName:  profile.Name,
			ProfileToken: profile.Token,
			Resolution: fmt.Sprintf("%dx%d",
				profile.VideoEncoderConfiguration.Resolution.Width,
				profile.VideoEncoderConfiguration.Resolution.Height),
			Framerate: profile.VideoEncoderConfiguration.RateControl.FrameRateLimit,
			Bitrate:   profile.VideoEncoderConfiguration.RateControl.BitrateLimit,
			Encoding:  profile.VideoEncoderConfiguration.Encoding,
			StreamURI: streamURI,
			Quality:   quality,
		}

		streamConfigs = append(streamConfigs, config)
	}

	return streamConfigs, nil
}

// resolveMediaURL returns the media service URL for a camera.
// Uses the discovered URL from GetCapabilities if available, otherwise
// falls back to path replacement on the device service address.
func resolveMediaURL(camera *Camera) string {
	if camera.MediaURL != "" {
		return camera.MediaURL
	}
	address := getFirstAddress(camera.Address)
	mediaURL := strings.Replace(address, "/device_service", "/media_service", 1)
	if !strings.Contains(mediaURL, "media_service") {
		mediaURL = strings.Replace(address, "/onvif/device_service", "/onvif/media_service", 1)
	}
	return mediaURL
}

// GetStreamUri retrieves the RTSP stream URI for a given profile token
func (c *Client) GetStreamUri(camera *Camera, profileToken string) (string, error) {
	if camera.MediaURL == "" {
		c.GetCapabilities(camera)
	}
	mediaURL := resolveMediaURL(camera)

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

	// Fallback to string extraction
	if uri := extractBetweenTags(string(resp), "Uri"); uri != "" {
		return uri, nil
	}

	return "", fmt.Errorf("no stream URI found in response")
}

// UpdateStreamConfiguration updates a stream's video encoder configuration
func (c *Client) UpdateStreamConfiguration(camera *Camera, encoderToken string, config StreamUpdateConfig) error {
	mediaURL := resolveMediaURL(camera)

	// Set video encoder configuration
	setConfigBody := fmt.Sprintf(`
	<trt:SetVideoEncoderConfiguration>
		<trt:Configuration token="%s">
			<tt:Name xmlns:tt="http://www.onvif.org/ver10/schema">Updated Configuration</tt:Name>
			<tt:UseCount xmlns:tt="http://www.onvif.org/ver10/schema">0</tt:UseCount>
			<tt:Encoding xmlns:tt="http://www.onvif.org/ver10/schema">%s</tt:Encoding>
			<tt:Resolution xmlns:tt="http://www.onvif.org/ver10/schema">
				<tt:Width>%d</tt:Width>
				<tt:Height>%d</tt:Height>
			</tt:Resolution>
			<tt:Quality xmlns:tt="http://www.onvif.org/ver10/schema">3.0</tt:Quality>
			<tt:RateControl xmlns:tt="http://www.onvif.org/ver10/schema">
				<tt:FrameRateLimit>%d</tt:FrameRateLimit>
				<tt:EncodingInterval>1</tt:EncodingInterval>
				<tt:BitrateLimit>%d</tt:BitrateLimit>
			</tt:RateControl>
			<tt:H264 xmlns:tt="http://www.onvif.org/ver10/schema">
				<tt:GovLength>30</tt:GovLength>
				<tt:H264Profile>Baseline</tt:H264Profile>
			</tt:H264>
			<tt:Multicast xmlns:tt="http://www.onvif.org/ver10/schema">
				<tt:Address>
					<tt:Type>IPv4</tt:Type>
					<tt:IPv4Address>0.0.0.0</tt:IPv4Address>
				</tt:Address>
				<tt:Port>0</tt:Port>
				<tt:TTL>0</tt:TTL>
				<tt:AutoStart>false</tt:AutoStart>
			</tt:Multicast>
		</trt:Configuration>
		<trt:ForcePersistence>true</trt:ForcePersistence>
	</trt:SetVideoEncoderConfiguration>`,
		encoderToken,
		config.Encoding,
		config.Resolution.Width,
		config.Resolution.Height,
		config.Framerate,
		config.Bitrate)

	updateResp, err := c.sendSOAPRequest(mediaURL,
		"http://www.onvif.org/ver10/media/wsdl/SetVideoEncoderConfiguration", setConfigBody)
	if err != nil {
		return fmt.Errorf("failed to update configuration: %v", err)
	}

	if bytes.Contains(updateResp, []byte("SOAP-ENV:Fault")) {
		return fmt.Errorf("SOAP fault in response")
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

	// Get the encoder token (may need to fetch it from profile)
	encoderToken := strings.Replace(subStream.ProfileToken, "profile", "videoencoder_config", 1)

	return c.UpdateStreamConfiguration(camera, encoderToken, config)
}