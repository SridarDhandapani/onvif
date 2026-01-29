package onvif

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// getMedia2URL converts a device service URL to the Media2 service URL
func getMedia2URL(address string) string {
	url := strings.Replace(address, "/device_service", "/media2", 1)
	if !strings.Contains(url, "media2") {
		url = strings.Replace(address, "/onvif/device_service", "/onvif/media2", 1)
	}
	return url
}

// GetOSDs retrieves all OSD (On-Screen Display) configurations from the camera
func (c *Client) GetOSDs(camera *Camera) ([]OSDConfig, error) {
	address := getFirstAddress(camera.Address)
	media2URL := getMedia2URL(address)

	body := `<tr2:GetOSDs/>`
	resp, err := c.sendSOAPRequest(media2URL,
		"http://www.onvif.org/ver20/media/wsdl/GetOSDs", body)
	if err != nil {
		return nil, fmt.Errorf("failed to get OSDs: %v", err)
	}

	if err := parseSOAPFault(resp); err != nil {
		return nil, err
	}

	// Try structured parsing
	type OSDsResponse struct {
		OSDs []struct {
			Token            string `xml:"token,attr"`
			VideoSourceToken string `xml:"VideoSourceConfigurationToken"`
			Type             string `xml:"Type"`
		} `xml:"Body>GetOSDsResponse>OSD"`
	}

	var osdsResp OSDsResponse
	if err := xml.Unmarshal(resp, &osdsResp); err == nil && len(osdsResp.OSDs) > 0 {
		var configs []OSDConfig
		for _, osd := range osdsResp.OSDs {
			configs = append(configs, OSDConfig{
				Token:            osd.Token,
				Type:             osd.Type,
				VideoSourceToken: osd.VideoSourceToken,
			})
		}
		return configs, nil
	}

	// Fallback: extract tokens from response string
	respStr := string(resp)
	var configs []OSDConfig
	remaining := respStr
	for {
		idx := strings.Index(remaining, "token=\"")
		if idx == -1 {
			break
		}
		start := idx + len("token=\"")
		end := strings.Index(remaining[start:], "\"")
		if end == -1 {
			break
		}
		token := remaining[start : start+end]
		// Skip tokens that look like profile/video source tokens (not OSD tokens)
		if strings.Contains(token, "OSD") || strings.Contains(token, "osd") {
			configs = append(configs, OSDConfig{Token: token})
		}
		remaining = remaining[start+end:]
	}

	return configs, nil
}

// DeleteOSD removes an OSD configuration by token
func (c *Client) DeleteOSD(camera *Camera, osdToken string) error {
	address := getFirstAddress(camera.Address)
	media2URL := getMedia2URL(address)

	body := fmt.Sprintf(`<tr2:DeleteOSD>
		<tr2:OSDToken>%s</tr2:OSDToken>
	</tr2:DeleteOSD>`, osdToken)

	resp, err := c.sendSOAPRequest(media2URL,
		"http://www.onvif.org/ver20/media/wsdl/DeleteOSD", body)
	if err != nil {
		return fmt.Errorf("failed to delete OSD: %v", err)
	}

	if err := parseSOAPFault(resp); err != nil {
		return err
	}

	return nil
}
