package onvif

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// imagingURLHeuristic derives a likely Imaging service URL from the
// device-service address. Used only when service discovery reported none.
func imagingURLHeuristic(address string) string {
	url := strings.Replace(address, "/device_service", "/imaging", 1)
	if !strings.Contains(url, "imaging") {
		url = strings.Replace(address, "/onvif/device_service", "/onvif/imaging", 1)
	}
	return url
}

// getVideoSourceToken retrieves the first video source token from media profiles
func (c *Client) getVideoSourceToken(camera *Camera) (string, error) {
	mediaURL := c.resolveMediaURL(camera)

	body := `<trt:GetVideoSources/>`
	resp, err := c.sendSOAPRequest(mediaURL,
		"http://www.onvif.org/ver10/media/wsdl/GetVideoSources", body)
	if err != nil {
		return "", fmt.Errorf("failed to get video sources: %v", err)
	}
	if err := parseSOAPFault(resp); err != nil {
		return "", err
	}

	var sourcesResp struct {
		VideoSources []struct {
			Token string `xml:"token,attr"`
		} `xml:"Body>GetVideoSourcesResponse>VideoSources"`
	}
	if err := xml.Unmarshal(resp, &sourcesResp); err != nil {
		return "", fmt.Errorf("failed to parse video sources: %v", err)
	}
	if len(sourcesResp.VideoSources) == 0 {
		return "", fmt.Errorf("no video source token found")
	}
	return sourcesResp.VideoSources[0].Token, nil
}

// GetImagingSettings retrieves the current imaging settings for the camera's video source
func (c *Client) GetImagingSettings(camera *Camera) (*ImagingSettings, error) {
	token, err := c.getVideoSourceToken(camera)
	if err != nil {
		return nil, err
	}

	imagingURL := c.resolveImagingURL(camera)

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
	if err := xml.Unmarshal(resp, &imgResp); err == nil {
		settings.IrCutFilter = IrCutFilterMode(imgResp.IrCutFilter)
	}

	return settings, nil
}

// SetIrCutFilter sets the IR cut filter (day/night) mode for the camera
func (c *Client) SetIrCutFilter(camera *Camera, mode IrCutFilterMode) error {
	token, err := c.getVideoSourceToken(camera)
	if err != nil {
		return err
	}

	imagingURL := c.resolveImagingURL(camera)

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
