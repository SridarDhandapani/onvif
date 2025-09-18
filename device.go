package onvif

import (
	"bytes"
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

	if err == nil && !bytes.Contains(deviceInfoResp, []byte("SOAP-ENV:Fault")) {
		// Try structured parsing first
		type DeviceInfoResponse struct {
			Manufacturer    string `xml:"Body>GetDeviceInformationResponse>Manufacturer"`
			Model           string `xml:"Body>GetDeviceInformationResponse>Model"`
			FirmwareVersion string `xml:"Body>GetDeviceInformationResponse>FirmwareVersion"`
			SerialNumber    string `xml:"Body>GetDeviceInformationResponse>SerialNumber"`
			HardwareId      string `xml:"Body>GetDeviceInformationResponse>HardwareId"`
		}

		var deviceInfo DeviceInfoResponse
		if err := xml.Unmarshal(deviceInfoResp, &deviceInfo); err != nil {
			// Try manual extraction if structured parsing fails
			respStr := string(deviceInfoResp)

			if start := strings.Index(respStr, "<tds:Manufacturer>"); start != -1 {
				start += len("<tds:Manufacturer>")
				if end := strings.Index(respStr[start:], "</tds:Manufacturer>"); end != -1 {
					deviceInfo.Manufacturer = respStr[start:start+end]
				}
			}

			if start := strings.Index(respStr, "<tds:Model>"); start != -1 {
				start += len("<tds:Model>")
				if end := strings.Index(respStr[start:], "</tds:Model>"); end != -1 {
					deviceInfo.Model = respStr[start:start+end]
				}
			}

			if start := strings.Index(respStr, "<tds:SerialNumber>"); start != -1 {
				start += len("<tds:SerialNumber>")
				if end := strings.Index(respStr[start:], "</tds:SerialNumber>"); end != -1 {
					deviceInfo.SerialNumber = respStr[start:start+end]
				}
			}

			if start := strings.Index(respStr, "<tds:FirmwareVersion>"); start != -1 {
				start += len("<tds:FirmwareVersion>")
				if end := strings.Index(respStr[start:], "</tds:FirmwareVersion>"); end != -1 {
					deviceInfo.FirmwareVersion = respStr[start:start+end]
				}
			}

			if start := strings.Index(respStr, "<tds:HardwareId>"); start != -1 {
				start += len("<tds:HardwareId>")
				if end := strings.Index(respStr[start:], "</tds:HardwareId>"); end != -1 {
					deviceInfo.HardwareId = respStr[start:start+end]
				}
			}
		}

		camera.Manufacturer = deviceInfo.Manufacturer
		camera.DeviceModel = deviceInfo.Model
		camera.FirmwareVersion = deviceInfo.FirmwareVersion
		camera.SerialNumber = deviceInfo.SerialNumber
		camera.HardwareId = deviceInfo.HardwareId
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

	if err == nil && !bytes.Contains(hostnameResp, []byte("SOAP-ENV:Fault")) {
		type HostnameResponse struct {
			HostnameInfo struct {
				FromDHCP bool   `xml:"FromDHCP,attr"`
				Name     string `xml:"Name"`
			} `xml:"Body>GetHostnameResponse>HostnameInformation"`
		}

		var hostname HostnameResponse
		if err := xml.Unmarshal(hostnameResp, &hostname); err == nil {
			camera.Hostname = hostname.HostnameInfo.Name
			if hostname.HostnameInfo.FromDHCP {
				camera.HostnameFrom = "DHCP"
			} else {
				camera.HostnameFrom = "Manual"
			}
		} else {
			// Try manual extraction
			respStr := string(hostnameResp)
			if start := strings.Index(respStr, "<tds:Name>"); start != -1 {
				start += len("<tds:Name>")
				if end := strings.Index(respStr[start:], "</tds:Name>"); end != -1 {
					camera.Hostname = respStr[start:start+end]
				}
			} else if start := strings.Index(respStr, "<tt:Name>"); start != -1 {
				start += len("<tt:Name>")
				if end := strings.Index(respStr[start:], "</tt:Name>"); end != -1 {
					camera.Hostname = respStr[start:start+end]
				}
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

	setDateTimeBody := fmt.Sprintf(`
	<tds:SetSystemDateAndTime>
		<tds:DateTimeType>Manual</tds:DateTimeType>
		<tds:DaylightSavings>false</tds:DaylightSavings>
		<tds:TimeZone>
			<tt:TZ xmlns:tt="http://www.onvif.org/ver10/schema">GMT0</tt:TZ>
		</tds:TimeZone>
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

	if bytes.Contains(setResp, []byte("SOAP-ENV:Fault")) {
		if bytes.Contains(setResp, []byte("NotAuthorized")) {
			return fmt.Errorf("not authorized to set date/time")
		}
		return fmt.Errorf("SOAP fault in response")
	}

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

		// Parse media capabilities
		type CapabilitiesResponse struct {
			Media struct {
				VideoSources int `xml:"VideoSources,attr"`
				VideoOutputs int `xml:"VideoOutputs,attr"`
				AudioSources int `xml:"AudioSources,attr"`
				AudioOutputs int `xml:"AudioOutputs,attr"`
			} `xml:"Body>GetCapabilitiesResponse>Capabilities>Media>StreamingCapabilities"`
			Device struct {
				IO struct {
					RelayOutputs int `xml:"RelayOutputs,attr"`
				} `xml:"IO"`
			} `xml:"Body>GetCapabilitiesResponse>Capabilities>Device"`
		}

		var capabilities CapabilitiesResponse
		if xml.Unmarshal(capabilitiesResp, &capabilities) == nil {
			camera.VideoSources = capabilities.Media.VideoSources
			camera.VideoOutputs = capabilities.Media.VideoOutputs
			camera.AudioSources = capabilities.Media.AudioSources
			camera.AudioOutputs = capabilities.Media.AudioOutputs
			camera.RelayOutputs = capabilities.Device.IO.RelayOutputs
		}
	}

	return nil
}