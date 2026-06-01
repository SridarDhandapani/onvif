package onvif

import (
	"encoding/xml"
	"fmt"
	"strings"
	"time"
)

// GetDeviceInformation fetches comprehensive device information
func (c *Client) GetDeviceInformation(camera *Camera) error {
	address := getFirstAddress(camera.Address)

	// Get device information
	deviceInfoBody := `<tds:GetDeviceInformation/>`
	deviceInfoResp, err := c.sendSOAPRequest(address,
		"http://www.onvif.org/ver10/device/wsdl/GetDeviceInformation", deviceInfoBody)

	if err == nil && parseSOAPFault(deviceInfoResp) == nil {
		type DeviceInfoResponse struct {
			Manufacturer    string `xml:"Body>GetDeviceInformationResponse>Manufacturer"`
			Model           string `xml:"Body>GetDeviceInformationResponse>Model"`
			FirmwareVersion string `xml:"Body>GetDeviceInformationResponse>FirmwareVersion"`
			SerialNumber    string `xml:"Body>GetDeviceInformationResponse>SerialNumber"`
			HardwareId      string `xml:"Body>GetDeviceInformationResponse>HardwareId"`
		}

		var deviceInfo DeviceInfoResponse
		if xml.Unmarshal(deviceInfoResp, &deviceInfo) == nil {
			camera.Manufacturer = deviceInfo.Manufacturer
			camera.DeviceModel = deviceInfo.Model
			camera.FirmwareVersion = deviceInfo.FirmwareVersion
			camera.SerialNumber = deviceInfo.SerialNumber
			camera.HardwareId = deviceInfo.HardwareId
		}
	}

	// Get hostname
	c.GetHostname(camera)

	// Get system date and time
	c.GetSystemDateTime(camera)

	// Get capabilities
	c.GetCapabilities(camera)

	return nil
}

// GetHostname fetches the device hostname
func (c *Client) GetHostname(camera *Camera) error {
	address := getFirstAddress(camera.Address)

	hostnameBody := `<tds:GetHostname/>`
	hostnameResp, err := c.sendSOAPRequest(address,
		"http://www.onvif.org/ver10/device/wsdl/GetHostname", hostnameBody)

	if err == nil && parseSOAPFault(hostnameResp) == nil {
		type HostnameResponse struct {
			HostnameInfo struct {
				FromDHCP bool   `xml:"FromDHCP,attr"`
				Name     string `xml:"Name"`
			} `xml:"Body>GetHostnameResponse>HostnameInformation"`
		}

		var hostname HostnameResponse
		if xml.Unmarshal(hostnameResp, &hostname) == nil {
			camera.Hostname = hostname.HostnameInfo.Name
			if hostname.HostnameInfo.FromDHCP {
				camera.HostnameFrom = "DHCP"
			} else {
				camera.HostnameFrom = "Manual"
			}
		}
	}

	return nil
}

// GetSystemDateTime fetches the system date and time
func (c *Client) GetSystemDateTime(camera *Camera) error {
	address := getFirstAddress(camera.Address)

	dateTimeBody := `<tds:GetSystemDateAndTime/>`
	dateTimeResp, err := c.sendSOAPRequest(address,
		"http://www.onvif.org/ver10/device/wsdl/GetSystemDateAndTime", dateTimeBody)

	if err == nil {
		type DateTimeResponse struct {
			TimeZone struct {
				TZ string `xml:"TZ"`
			} `xml:"Body>GetSystemDateAndTimeResponse>SystemDateAndTime>TimeZone"`
			UTCDateTime struct {
				Time struct {
					Hour   int `xml:"Hour"`
					Minute int `xml:"Minute"`
					Second int `xml:"Second"`
				} `xml:"Time"`
				Date struct {
					Year  int `xml:"Year"`
					Month int `xml:"Month"`
					Day   int `xml:"Day"`
				} `xml:"Date"`
			} `xml:"Body>GetSystemDateAndTimeResponse>SystemDateAndTime>UTCDateTime"`
		}

		var dateTime DateTimeResponse
		if xml.Unmarshal(dateTimeResp, &dateTime) == nil {
			camera.TimeZone = dateTime.TimeZone.TZ
			if dateTime.UTCDateTime.Date.Year > 0 {
				camera.DateTime = fmt.Sprintf("%04d-%02d-%02d %02d:%02d:%02d UTC",
					dateTime.UTCDateTime.Date.Year,
					dateTime.UTCDateTime.Date.Month,
					dateTime.UTCDateTime.Date.Day,
					dateTime.UTCDateTime.Time.Hour,
					dateTime.UTCDateTime.Time.Minute,
					dateTime.UTCDateTime.Time.Second)
			}
		}
	}

	return nil
}

// SetSystemDateTime sets the system date and time to current GMT
func (c *Client) SetSystemDateTime(camera *Camera) error {
	address := getFirstAddress(camera.Address)
	now := time.Now().UTC()

	// TimeZone is intentionally omitted: per the ONVIF spec it is optional, and
	// when absent the device keeps its configured timezone and simply applies
	// the supplied UTC time — which is all this call needs (clock sync). Some
	// devices (e.g. Reolink) reject the request with ter:InvalidArgVal when sent
	// a TZ value they consider malformed, so not sending one is both correct and
	// the most widely compatible.
	setDateTimeBody := fmt.Sprintf(`
	<tds:SetSystemDateAndTime>
		<tds:DateTimeType>Manual</tds:DateTimeType>
		<tds:DaylightSavings>false</tds:DaylightSavings>
		<tds:UTCDateTime>
			<tt:Time xmlns:tt="http://www.onvif.org/ver10/schema">
				<tt:Hour>%d</tt:Hour>
				<tt:Minute>%d</tt:Minute>
				<tt:Second>%d</tt:Second>
			</tt:Time>
			<tt:Date xmlns:tt="http://www.onvif.org/ver10/schema">
				<tt:Year>%d</tt:Year>
				<tt:Month>%d</tt:Month>
				<tt:Day>%d</tt:Day>
			</tt:Date>
		</tds:UTCDateTime>
	</tds:SetSystemDateAndTime>`,
		now.Hour(), now.Minute(), now.Second(),
		now.Year(), int(now.Month()), now.Day())

	setResp, err := c.sendSOAPRequest(address,
		"http://www.onvif.org/ver10/device/wsdl/SetSystemDateAndTime", setDateTimeBody)
	if err != nil {
		return fmt.Errorf("failed to set date/time: %v", err)
	}

	if err := parseSOAPFault(setResp); err != nil {
		return fmt.Errorf("failed to set date/time: %w", err)
	}

	return nil
}

// SetHostname sets the device hostname
func (c *Client) SetHostname(camera *Camera, name string) error {
	address := getFirstAddress(camera.Address)

	body := fmt.Sprintf(`<tds:SetHostname><tds:Name>%s</tds:Name></tds:SetHostname>`, name)
	resp, err := c.sendSOAPRequest(address,
		"http://www.onvif.org/ver10/device/wsdl/SetHostname", body)
	if err != nil {
		return fmt.Errorf("failed to set hostname: %v", err)
	}

	if err := parseSOAPFault(resp); err != nil {
		return err
	}

	camera.Hostname = name
	return nil
}

// GetCapabilities fetches device capabilities
func (c *Client) GetCapabilities(camera *Camera) error {
	address := getFirstAddress(camera.Address)

	capabilitiesBody := `<tds:GetCapabilities><tds:Category>All</tds:Category></tds:GetCapabilities>`
	capabilitiesResp, err := c.sendSOAPRequest(address,
		"http://www.onvif.org/ver10/device/wsdl/GetCapabilities", capabilitiesBody)

	if err == nil {
		respStr := string(capabilitiesResp)

		// Check for PTZ support
		camera.PTZSupport = strings.Contains(respStr, "PTZ>") && strings.Contains(respStr, "XAddr")

		// Check for Analytics support
		camera.AnalyticsSupport = strings.Contains(respStr, "Analytics>") && strings.Contains(respStr, "XAddr")

		// Parse capabilities including service URLs
		type CapabilitiesResponse struct {
			Media struct {
				XAddr         string `xml:"XAddr"`
				StreamingCaps struct {
					VideoSources int `xml:"VideoSources,attr"`
					VideoOutputs int `xml:"VideoOutputs,attr"`
					AudioSources int `xml:"AudioSources,attr"`
					AudioOutputs int `xml:"AudioOutputs,attr"`
				} `xml:"StreamingCapabilities"`
			} `xml:"Body>GetCapabilitiesResponse>Capabilities>Media"`
			Imaging struct {
				XAddr string `xml:"XAddr"`
			} `xml:"Body>GetCapabilitiesResponse>Capabilities>Extension>Imaging"`
			Device struct {
				IO struct {
					RelayOutputs int `xml:"RelayOutputs,attr"`
				} `xml:"IO"`
			} `xml:"Body>GetCapabilitiesResponse>Capabilities>Device"`
		}

		var capabilities CapabilitiesResponse
		if xml.Unmarshal(capabilitiesResp, &capabilities) == nil {
			camera.VideoSources = capabilities.Media.StreamingCaps.VideoSources
			camera.VideoOutputs = capabilities.Media.StreamingCaps.VideoOutputs
			camera.AudioSources = capabilities.Media.StreamingCaps.AudioSources
			camera.AudioOutputs = capabilities.Media.StreamingCaps.AudioOutputs
			camera.RelayOutputs = capabilities.Device.IO.RelayOutputs
			if capabilities.Media.XAddr != "" {
				camera.MediaURL = capabilities.Media.XAddr
			}
			if capabilities.Imaging.XAddr != "" {
				camera.ImagingURL = capabilities.Imaging.XAddr
			}
		}
		// Service URLs that GetCapabilities does not report (notably Media2) are
		// resolved via GetServices in discoverServices().
	}

	return nil
}

// GetServices queries the device service for the endpoint (XAddr) of every
// ONVIF service it exposes and records the ones we use. Unlike GetCapabilities,
// GetServices reliably reports the Media2 service, and reports each service's
// real host/port/path — important for cameras (e.g. Reolink) that serve ONVIF
// services on a non-default port and distinct paths.
func (c *Client) GetServices(camera *Camera) error {
	address := getFirstAddress(camera.Address)

	body := `<tds:GetServices><tds:IncludeCapability>false</tds:IncludeCapability></tds:GetServices>`
	resp, err := c.sendSOAPRequest(address,
		"http://www.onvif.org/ver10/device/wsdl/GetServices", body)
	if err != nil {
		return fmt.Errorf("failed to get services: %v", err)
	}
	if err := parseSOAPFault(resp); err != nil {
		return err
	}

	var parsed struct {
		Services []struct {
			Namespace string `xml:"Namespace"`
			XAddr     string `xml:"XAddr"`
		} `xml:"Body>GetServicesResponse>Service"`
	}
	if err := xml.Unmarshal(resp, &parsed); err != nil {
		return fmt.Errorf("failed to parse services: %v", err)
	}

	for _, s := range parsed.Services {
		switch strings.TrimSpace(s.Namespace) {
		case "http://www.onvif.org/ver20/media/wsdl":
			camera.Media2URL = s.XAddr
		case "http://www.onvif.org/ver10/media/wsdl":
			if camera.MediaURL == "" {
				camera.MediaURL = s.XAddr
			}
		case "http://www.onvif.org/ver20/imaging/wsdl":
			if camera.ImagingURL == "" {
				camera.ImagingURL = s.XAddr
			}
		}
	}
	return nil
}

// discoverServices populates the camera's service URLs (Media / Media2 /
// Imaging) using the device's own advertisements: GetCapabilities first, then
// GetServices for anything still missing (notably Media2, and the real
// host/port for cameras that serve ONVIF off the default endpoint). Best-effort
// — anything still unset is left to a per-service heuristic fallback.
func (c *Client) discoverServices(camera *Camera) {
	if camera.MediaURL == "" || camera.ImagingURL == "" {
		_ = c.GetCapabilities(camera)
	}
	if camera.MediaURL == "" || camera.ImagingURL == "" || camera.Media2URL == "" {
		_ = c.GetServices(camera)
	}
}

// resolveMediaURL returns the Media (ver10) service URL, discovering it if
// needed and falling back to a heuristic rewrite of the device-service address.
func (c *Client) resolveMediaURL(camera *Camera) string {
	if camera.MediaURL == "" {
		c.discoverServices(camera)
	}
	if camera.MediaURL != "" {
		return camera.MediaURL
	}
	return mediaURLHeuristic(getFirstAddress(camera.Address))
}

// resolveImagingURL returns the Imaging service URL, discovering it if needed
// and falling back to a heuristic rewrite of the device-service address.
func (c *Client) resolveImagingURL(camera *Camera) string {
	if camera.ImagingURL == "" {
		c.discoverServices(camera)
	}
	if camera.ImagingURL != "" {
		return camera.ImagingURL
	}
	return imagingURLHeuristic(getFirstAddress(camera.Address))
}

// resolveMedia2URL returns the Media2 (ver20) service URL, discovering it if
// needed and falling back to a heuristic rewrite of the device-service address.
func (c *Client) resolveMedia2URL(camera *Camera) string {
	if camera.Media2URL == "" {
		c.discoverServices(camera)
	}
	if camera.Media2URL != "" {
		return camera.Media2URL
	}
	return getMedia2URL(getFirstAddress(camera.Address))
}
