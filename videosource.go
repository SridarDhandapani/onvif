package onvif

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// GetVideoSourceModes returns the sensor capture modes (video source modes) the
// camera supports. A mode carries the sensor resolution (hence aspect ratio),
// max framerate and supported encodings, and is selected by token.
func (c *Client) GetVideoSourceModes(camera *Camera) ([]VideoSourceMode, error) {
	token, err := c.getVideoSourceToken(camera)
	if err != nil {
		return nil, err
	}
	mediaURL := c.resolveMediaURL(camera)

	body := fmt.Sprintf(`<trt:GetVideoSourceModes>
		<trt:VideoSourceToken>%s</trt:VideoSourceToken>
	</trt:GetVideoSourceModes>`, token)

	resp, err := c.sendSOAPRequest(mediaURL,
		"http://www.onvif.org/ver10/media/wsdl/GetVideoSourceModes", body)
	if err != nil {
		return nil, fmt.Errorf("failed to get video source modes: %v", err)
	}
	if err := parseSOAPFault(resp); err != nil {
		return nil, err
	}

	var parsed struct {
		Modes []struct {
			Token         string  `xml:"token,attr"`
			Enabled       bool    `xml:"Enabled,attr"`
			MaxFramerate  float64 `xml:"MaxFramerate"`
			MaxResolution struct {
				Width  int `xml:"Width"`
				Height int `xml:"Height"`
			} `xml:"MaxResolution"`
			Encodings   string `xml:"Encodings"`
			Reboot      bool   `xml:"Reboot"`
			Description string `xml:"Description"`
		} `xml:"Body>GetVideoSourceModesResponse>VideoSourceModes"`
	}
	if err := xml.Unmarshal(resp, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse video source modes: %v", err)
	}

	modes := make([]VideoSourceMode, 0, len(parsed.Modes))
	for _, m := range parsed.Modes {
		modes = append(modes, VideoSourceMode{
			Token:        m.Token,
			Enabled:      m.Enabled,
			MaxFramerate: m.MaxFramerate,
			Width:        m.MaxResolution.Width,
			Height:       m.MaxResolution.Height,
			Encodings:    strings.TrimSpace(m.Encodings),
			Reboot:       m.Reboot,
			Description:  strings.TrimSpace(m.Description),
		})
	}
	return modes, nil
}

// SetVideoSourceMode selects the sensor capture mode with the given token. It
// returns whether the camera needs to reboot to apply the change.
func (c *Client) SetVideoSourceMode(camera *Camera, modeToken string) (bool, error) {
	token, err := c.getVideoSourceToken(camera)
	if err != nil {
		return false, err
	}
	mediaURL := c.resolveMediaURL(camera)

	body := fmt.Sprintf(`<trt:SetVideoSourceMode>
		<trt:VideoSourceToken>%s</trt:VideoSourceToken>
		<trt:VideoSourceModeToken>%s</trt:VideoSourceModeToken>
	</trt:SetVideoSourceMode>`, token, modeToken)

	resp, err := c.sendSOAPRequest(mediaURL,
		"http://www.onvif.org/ver10/media/wsdl/SetVideoSourceMode", body)
	if err != nil {
		return false, fmt.Errorf("failed to set video source mode: %v", err)
	}
	if err := parseSOAPFault(resp); err != nil {
		return false, err
	}

	var parsed struct {
		Reboot bool `xml:"Body>SetVideoSourceModeResponse>Reboot"`
	}
	_ = xml.Unmarshal(resp, &parsed)
	return parsed.Reboot, nil
}
