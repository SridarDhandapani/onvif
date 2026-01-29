package onvif

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// resolveImagingURL returns the imaging service URL for a camera.
func resolveImagingURL(camera *Camera) string {
	if camera.ImagingURL != "" {
		return camera.ImagingURL
	}
	address := getFirstAddress(camera.Address)
	url := strings.Replace(address, "/device_service", "/imaging", 1)
	if !strings.Contains(url, "imaging") {
		url = strings.Replace(address, "/onvif/device_service", "/onvif/imaging", 1)
	}
	return url
}

// getVideoSourceToken retrieves the first video source token from media profiles
func (c *Client) getVideoSourceToken(camera *Camera) (string, error) {
	if camera.MediaURL == "" {
		c.GetCapabilities(camera)
	}
	mediaURL := resolveMediaURL(camera)

	body := `<trt:GetVideoSources/>`
	resp, err := c.sendSOAPRequest(mediaURL,
		"http://www.onvif.org/ver10/media/wsdl/GetVideoSources", body)
	if err != nil {
		return "", fmt.Errorf("failed to get video sources: %v", err)
	}

	// Try structured parsing
	type VideoSourcesResponse struct {
		VideoSources []struct {
			Token string `xml:"token,attr"`
		} `xml:"Body>GetVideoSourcesResponse>VideoSources"`
	}

	var sourcesResp VideoSourcesResponse
	if err := xml.Unmarshal(resp, &sourcesResp); err == nil && len(sourcesResp.VideoSources) > 0 {
		return sourcesResp.VideoSources[0].Token, nil
	}

	// Fallback: extract token from response
	token := extractBetweenTags(string(resp), "token")
	if token != "" {
		return token, nil
	}

	// Look for token attribute in VideoSources element
	respStr := string(resp)
	if idx := strings.Index(respStr, "token=\""); idx != -1 {
		start := idx + len("token=\"")
		if end := strings.Index(respStr[start:], "\""); end != -1 {
			return respStr[start : start+end], nil
		}
	}

	return "", fmt.Errorf("no video source token found")
}

// GetImagingSettings retrieves the current imaging settings for the camera's video source
func (c *Client) GetImagingSettings(camera *Camera) (*ImagingSettings, error) {
	token, err := c.getVideoSourceToken(camera)
	if err != nil {
		return nil, err
	}

	if camera.ImagingURL == "" {
		c.GetCapabilities(camera)
	}
	imagingURL := resolveImagingURL(camera)

	body := fmt.Sprintf(`<timg:GetImagingSettings>
		<timg:VideoSourceToken>%s</timg:VideoSourceToken>
	</timg:GetImagingSettings>`, token)

	resp, err := c.sendSOAPRequest(imagingURL,
		"http://www.onvif.org/ver20/imaging/wsdl/GetImagingSettings", body)
	if err != nil {
		return nil, fmt.Errorf("failed to get imaging settings: %v", err)
	}

	if err := parseSOAPFault(resp); err != nil {
		return nil, err
	}

	settings := &ImagingSettings{
		VideoSourceToken: token,
	}

	// Try structured parsing
	type ImagingResponse struct {
		IrCutFilter string `xml:"Body>GetImagingSettingsResponse>ImagingSettings>IrCutFilter"`
	}

	var imgResp ImagingResponse
	if err := xml.Unmarshal(resp, &imgResp); err == nil && imgResp.IrCutFilter != "" {
		settings.IrCutFilter = IrCutFilterMode(imgResp.IrCutFilter)
		return settings, nil
	}

	// Fallback to string extraction
	if filter := extractBetweenTags(string(resp), "IrCutFilter"); filter != "" {
		settings.IrCutFilter = IrCutFilterMode(filter)
	}

	return settings, nil
}

// SetIrCutFilter sets the IR cut filter (day/night) mode for the camera
func (c *Client) SetIrCutFilter(camera *Camera, mode IrCutFilterMode) error {
	token, err := c.getVideoSourceToken(camera)
	if err != nil {
		return err
	}

	if camera.ImagingURL == "" {
		c.GetCapabilities(camera)
	}
	imagingURL := resolveImagingURL(camera)

	body := fmt.Sprintf(`<timg:SetImagingSettings>
		<timg:VideoSourceToken>%s</timg:VideoSourceToken>
		<timg:ImagingSettings>
			<tt:IrCutFilter xmlns:tt="http://www.onvif.org/ver10/schema">%s</tt:IrCutFilter>
		</timg:ImagingSettings>
	</timg:SetImagingSettings>`, token, string(mode))

	resp, err := c.sendSOAPRequest(imagingURL,
		"http://www.onvif.org/ver20/imaging/wsdl/SetImagingSettings", body)
	if err != nil {
		return fmt.Errorf("failed to set IR cut filter: %v", err)
	}

	if err := parseSOAPFault(resp); err != nil {
		return err
	}

	return nil
}
