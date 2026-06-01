package onvif

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// ptzURLHeuristic derives a likely PTZ service URL from the device-service
// address. Used only when service discovery reported none.
func ptzURLHeuristic(address string) string {
	url := strings.Replace(address, "/device_service", "/ptz", 1)
	if !strings.Contains(url, "ptz") {
		url = strings.Replace(address, "/onvif/device_service", "/onvif/ptz", 1)
	}
	return url
}

// resolvePTZURL returns the PTZ service URL, discovering it if needed and
// falling back to a heuristic rewrite of the device-service address.
func (c *Client) resolvePTZURL(camera *Camera) string {
	if camera.PTZURL == "" {
		c.discoverServices(camera)
	}
	if camera.PTZURL != "" {
		return camera.PTZURL
	}
	return ptzURLHeuristic(getFirstAddress(camera.Address))
}

// HasPTZ reports whether the camera advertises ONVIF PTZ support, querying
// capabilities/services first if not yet known.
func (c *Client) HasPTZ(camera *Camera) bool {
	if !camera.PTZSupport && camera.PTZURL == "" {
		c.discoverServices(camera)
	}
	return camera.PTZSupport || camera.PTZURL != ""
}

// PTZDiagnostics returns the raw GetNodes, GetConfigurations and GetStatus PTZ
// responses. GetNodes' SupportedPTZSpaces reveal which move operations the
// camera supports (Absolute / Relative / Continuous pan-tilt and zoom), which
// determines how the tool should drive it.
func (c *Client) PTZDiagnostics(camera *Camera, profileToken string) string {
	ptzURL := c.resolvePTZURL(camera)
	var b strings.Builder
	dump := func(action, body string) {
		resp, err := c.sendSOAPRequest(ptzURL, "http://www.onvif.org/ver20/ptz/wsdl/"+action, body)
		if err != nil {
			fmt.Fprintf(&b, "=== PTZ %s error: %v ===\n\n", action, err)
			return
		}
		fmt.Fprintf(&b, "=== PTZ %s ===\n%s\n\n", action, string(resp))
	}
	dump("GetNodes", `<tptz:GetNodes/>`)
	dump("GetConfigurations", `<tptz:GetConfigurations/>`)
	dump("GetStatus", fmt.Sprintf(`<tptz:GetStatus><tptz:ProfileToken>%s</tptz:ProfileToken></tptz:GetStatus>`, profileToken))
	return b.String()
}

// GetPTZConfigurations returns the PTZ configurations the device exposes.
func (c *Client) GetPTZConfigurations(camera *Camera) ([]PTZConfig, error) {
	ptzURL := c.resolvePTZURL(camera)
	resp, err := c.sendSOAPRequest(ptzURL,
		"http://www.onvif.org/ver20/ptz/wsdl/GetConfigurations", `<tptz:GetConfigurations/>`)
	if err != nil {
		return nil, fmt.Errorf("failed to get PTZ configurations: %v", err)
	}
	if err := parseSOAPFault(resp); err != nil {
		return nil, err
	}

	var parsed struct {
		Configs []struct {
			Token     string `xml:"token,attr"`
			Name      string `xml:"Name"`
			NodeToken string `xml:"NodeToken"`
		} `xml:"Body>GetConfigurationsResponse>PTZConfiguration"`
	}
	if err := xml.Unmarshal(resp, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse PTZ configurations: %v", err)
	}

	configs := make([]PTZConfig, 0, len(parsed.Configs))
	for _, p := range parsed.Configs {
		configs = append(configs, PTZConfig{Token: p.Token, Name: p.Name, NodeToken: p.NodeToken})
	}
	return configs, nil
}

// GetPTZStatus returns the current PTZ position (including the zoom level) and
// movement state for the given media profile.
func (c *Client) GetPTZStatus(camera *Camera, profileToken string) (*PTZStatus, error) {
	ptzURL := c.resolvePTZURL(camera)
	body := fmt.Sprintf(`<tptz:GetStatus><tptz:ProfileToken>%s</tptz:ProfileToken></tptz:GetStatus>`, profileToken)
	resp, err := c.sendSOAPRequest(ptzURL,
		"http://www.onvif.org/ver20/ptz/wsdl/GetStatus", body)
	if err != nil {
		return nil, fmt.Errorf("failed to get PTZ status: %v", err)
	}
	if err := parseSOAPFault(resp); err != nil {
		return nil, err
	}

	var parsed struct {
		PanTilt struct {
			X float64 `xml:"x,attr"`
			Y float64 `xml:"y,attr"`
		} `xml:"Body>GetStatusResponse>PTZStatus>Position>PanTilt"`
		Zoom struct {
			X float64 `xml:"x,attr"`
		} `xml:"Body>GetStatusResponse>PTZStatus>Position>Zoom"`
		MoveState string `xml:"Body>GetStatusResponse>PTZStatus>MoveStatus>PanTilt"`
		UTCTime   string `xml:"Body>GetStatusResponse>PTZStatus>UtcTime"`
	}
	if err := xml.Unmarshal(resp, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse PTZ status: %v", err)
	}

	return &PTZStatus{
		Position:  PTZVector{Pan: parsed.PanTilt.X, Tilt: parsed.PanTilt.Y, Zoom: parsed.Zoom.X},
		MoveState: parsed.MoveState,
		UTCTime:   parsed.UTCTime,
	}, nil
}

// buildPTZVectorXML renders a PanTilt/Zoom vector wrapped in the given element
// (e.g. "tptz:Velocity", "tptz:Translation", "tptz:Position"). PanTilt and Zoom
// are each emitted only when requested. This is the unit-test seam for the move
// builders.
func buildPTZVectorXML(elem string, v PTZVector, includePanTilt, includeZoom bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<%s>", elem)
	if includePanTilt {
		fmt.Fprintf(&b, `<tt:PanTilt xmlns:tt="http://www.onvif.org/ver10/schema" x="%g" y="%g"/>`, v.Pan, v.Tilt)
	}
	if includeZoom {
		fmt.Fprintf(&b, `<tt:Zoom xmlns:tt="http://www.onvif.org/ver10/schema" x="%g"/>`, v.Zoom)
	}
	fmt.Fprintf(&b, "</%s>", elem)
	return b.String()
}

// ContinuousMove starts a continuous PTZ move at the given velocity (each
// component normalized -1..1). The caller must Stop. Pan/tilt and zoom are
// included only when non-zero.
func (c *Client) ContinuousMove(camera *Camera, profileToken string, v PTZVector) error {
	body := fmt.Sprintf(`<tptz:ContinuousMove><tptz:ProfileToken>%s</tptz:ProfileToken>%s</tptz:ContinuousMove>`,
		profileToken, buildPTZVectorXML("tptz:Velocity", v, v.Pan != 0 || v.Tilt != 0, v.Zoom != 0))
	return c.ptzCall(camera, "ContinuousMove", body)
}

// RelativeMove performs a relative PTZ move by the given translation.
func (c *Client) RelativeMove(camera *Camera, profileToken string, v PTZVector) error {
	body := fmt.Sprintf(`<tptz:RelativeMove><tptz:ProfileToken>%s</tptz:ProfileToken>%s</tptz:RelativeMove>`,
		profileToken, buildPTZVectorXML("tptz:Translation", v, v.Pan != 0 || v.Tilt != 0, v.Zoom != 0))
	return c.ptzCall(camera, "RelativeMove", body)
}

// AbsoluteMove moves to an absolute PTZ position.
func (c *Client) AbsoluteMove(camera *Camera, profileToken string, v PTZVector) error {
	body := fmt.Sprintf(`<tptz:AbsoluteMove><tptz:ProfileToken>%s</tptz:ProfileToken>%s</tptz:AbsoluteMove>`,
		profileToken, buildPTZVectorXML("tptz:Position", v, v.Pan != 0 || v.Tilt != 0, v.Zoom != 0))
	return c.ptzCall(camera, "AbsoluteMove", body)
}

// Stop stops PTZ movement; panTilt and zoom select which axes to halt.
func (c *Client) Stop(camera *Camera, profileToken string, panTilt, zoom bool) error {
	body := fmt.Sprintf(`<tptz:Stop><tptz:ProfileToken>%s</tptz:ProfileToken><tptz:PanTilt>%t</tptz:PanTilt><tptz:Zoom>%t</tptz:Zoom></tptz:Stop>`,
		profileToken, panTilt, zoom)
	return c.ptzCall(camera, "Stop", body)
}

func (c *Client) ptzCall(camera *Camera, op, body string) error {
	ptzURL := c.resolvePTZURL(camera)
	resp, err := c.sendSOAPRequest(ptzURL, "http://www.onvif.org/ver20/ptz/wsdl/"+op, body)
	if err != nil {
		return fmt.Errorf("PTZ %s failed: %v", op, err)
	}
	if err := parseSOAPFault(resp); err != nil {
		return fmt.Errorf("PTZ %s failed: %w", op, err)
	}
	return nil
}

// ZoomIn starts a continuous zoom in at the given speed (0..1); caller ZoomStops.
func (c *Client) ZoomIn(camera *Camera, profileToken string, speed float64) error {
	return c.ContinuousMove(camera, profileToken, PTZVector{Zoom: speed})
}

// ZoomStop stops zoom movement.
func (c *Client) ZoomStop(camera *Camera, profileToken string) error {
	return c.Stop(camera, profileToken, false, true)
}

// ZoomTo moves to an absolute zoom level (0..1).
func (c *Client) ZoomTo(camera *Camera, profileToken string, level float64) error {
	return c.AbsoluteMove(camera, profileToken, PTZVector{Zoom: level})
}
