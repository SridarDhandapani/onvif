package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	multicastAddr = "239.255.255.250:3702"
	probeMessage  = `<?xml version="1.0" encoding="UTF-8"?>
<Envelope xmlns="http://www.w3.org/2003/05/soap-envelope"
          xmlns:a="http://schemas.xmlsoap.org/ws/2004/08/addressing"
          xmlns:d="http://schemas.xmlsoap.org/ws/2005/04/discovery"
          xmlns:dn="http://www.onvif.org/ver10/network/wsdl">
    <Header>
        <a:Action>http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe</a:Action>
        <a:MessageID>uuid:probe-message-1</a:MessageID>
        <a:To>urn:schemas-xmlsoap-org:ws:2005:04:discovery</a:To>
    </Header>
    <Body>
        <d:Probe>
            <d:Types>dn:NetworkVideoTransmitter</d:Types>
        </d:Probe>
    </Body>
</Envelope>`
)

type Envelope struct {
	XMLName xml.Name `xml:"Envelope"`
	Header  Header   `xml:"Header"`
	Body    Body     `xml:"Body"`
}

type Header struct {
	MessageID   string `xml:"MessageID"`
	RelatesTo   string `xml:"RelatesTo"`
	To          string `xml:"To"`
	Action      string `xml:"Action"`
	AppSequence AppSeq `xml:"AppSequence"`
}

type AppSeq struct {
	InstanceId    int `xml:"InstanceId,attr"`
	MessageNumber int `xml:"MessageNumber,attr"`
}

type Body struct {
	ProbeMatches ProbeMatches `xml:"ProbeMatches"`
}

type ProbeMatches struct {
	ProbeMatch []ProbeMatch `xml:"ProbeMatch"`
}

type ProbeMatch struct {
	EndpointReference EndpointRef `xml:"EndpointReference"`
	Types             string      `xml:"Types"`
	Scopes            string      `xml:"Scopes"`
	XAddrs            string      `xml:"XAddrs"`
	MetadataVersion   int         `xml:"MetadataVersion"`
}

type EndpointRef struct {
	Address string `xml:"Address"`
}

type Camera struct {
	// From discovery
	Name     string
	Address  string
	Profiles []string
	Model    string
	Location string

	// From GetHostname
	Hostname     string
	HostnameFrom string // "DHCP" or "Manual"

	// From GetDeviceInformation
	Manufacturer    string
	DeviceModel     string
	FirmwareVersion string
	SerialNumber    string
	HardwareId      string

	// From GetSystemDateAndTime
	TimeZone string
	DateTime string

	// From GetCapabilities
	VideoSources    int
	VideoOutputs    int
	AudioSources    int
	AudioOutputs    int
	RelayOutputs    int
	PTZSupport      bool
	AnalyticsSupport bool
}

func parseScopes(scopes string) (name, location, model string) {
	scopeList := strings.Split(scopes, " ")
	for _, scope := range scopeList {
		scope = strings.TrimSpace(scope)
		if strings.Contains(scope, "onvif://www.onvif.org/name/") {
			name = strings.Replace(scope, "onvif://www.onvif.org/name/", "", 1)
			name = strings.Replace(name, "_", " ", -1)
		} else if strings.Contains(scope, "onvif://www.onvif.org/location/") {
			location = strings.Replace(scope, "onvif://www.onvif.org/location/", "", 1)
			location = strings.Replace(location, "_", " ", -1)
		} else if strings.Contains(scope, "onvif://www.onvif.org/hardware/") {
			model = strings.Replace(scope, "onvif://www.onvif.org/hardware/", "", 1)
			model = strings.Replace(model, "_", " ", -1)
		}
	}
	return
}

func parseProfiles(types string) []string {
	var profiles []string
	typeList := strings.Split(types, " ")

	for _, t := range typeList {
		t = strings.TrimSpace(t)
		switch {
		case strings.Contains(t, "NetworkVideoTransmitter"):
			profiles = append(profiles, "Network Video Transmitter")
		case strings.Contains(t, "Device"):
			profiles = append(profiles, "Device")
		case strings.Contains(t, "Media"):
			profiles = append(profiles, "Media")
		case strings.Contains(t, "PTZ"):
			profiles = append(profiles, "PTZ")
		case strings.Contains(t, "Analytics"):
			profiles = append(profiles, "Analytics")
		case strings.Contains(t, "Events"):
			profiles = append(profiles, "Events")
		case strings.Contains(t, "Imaging"):
			profiles = append(profiles, "Imaging")
		case strings.Contains(t, "Recording"):
			profiles = append(profiles, "Recording")
		case strings.Contains(t, "Replay"):
			profiles = append(profiles, "Replay")
		}
	}

	return profiles
}

func deduplicateCameras(cameras []Camera) []Camera {
	cameraMap := make(map[string]Camera)

	for _, camera := range cameras {
		// Get first address if multiple
		addresses := strings.Fields(camera.Address)
		key := addresses[0]

		if _, exists := cameraMap[key]; !exists {
			cameraMap[key] = camera
		}
	}

	var uniqueCameras []Camera
	for _, camera := range cameraMap {
		uniqueCameras = append(uniqueCameras, camera)
	}

	return uniqueCameras
}

func discoverCameras() ([]Camera, error) {
	addr, err := net.ResolveUDPAddr("udp4", multicastAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve multicast address: %v", err)
	}

	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, fmt.Errorf("failed to create UDP connection: %v", err)
	}
	defer conn.Close()

	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return nil, fmt.Errorf("failed to set read deadline: %v", err)
	}

	fmt.Println("🔍 Sending ONVIF discovery probe to network...")

	if _, err := conn.WriteToUDP([]byte(probeMessage), addr); err != nil {
		return nil, fmt.Errorf("failed to send probe message: %v", err)
	}

	var cameras []Camera
	buffer := make([]byte, 65536)

	for {
		n, src, err := conn.ReadFromUDP(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				break
			}
			log.Printf("Error reading response: %v", err)
			continue
		}

		response := string(buffer[:n])

		var envelope Envelope
		if err := xml.Unmarshal([]byte(response), &envelope); err != nil {
			log.Printf("Error parsing response from %s: %v", src, err)
			continue
		}

		for _, match := range envelope.Body.ProbeMatches.ProbeMatch {
			name, location, model := parseScopes(match.Scopes)
			profiles := parseProfiles(match.Types)

			camera := Camera{
				Name:     name,
				Address:  match.XAddrs,
				Profiles: profiles,
				Model:    model,
				Location: location,
			}

			cameras = append(cameras, camera)
		}
	}

	return deduplicateCameras(cameras), nil
}

func generatePasswordDigest(username, password string) (string, string, string) {
	created := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	nonce := fmt.Sprintf("%d", time.Now().UnixNano())
	nonceBytes := []byte(nonce)
	nonceB64 := base64.StdEncoding.EncodeToString(nonceBytes)

	h := sha1.New()
	h.Write(nonceBytes)
	h.Write([]byte(created))
	h.Write([]byte(password))
	digest := base64.StdEncoding.EncodeToString(h.Sum(nil))

	return digest, nonceB64, created
}

func sendSOAPRequest(endpoint, username, password, action, body string) ([]byte, error) {
	digest, nonce, created := generatePasswordDigest(username, password)

	authHeader := ""
	if username != "" {
		authHeader = fmt.Sprintf(`
		<Security xmlns="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd">
			<UsernameToken>
				<Username>%s</Username>
				<Password Type="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-username-token-profile-1.0#PasswordDigest">%s</Password>
				<Nonce EncodingType="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-soap-message-security-1.0#Base64Binary">%s</Nonce>
				<Created xmlns="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd">%s</Created>
			</UsernameToken>
		</Security>`, username, digest, nonce, created)
	}

	// Use appropriate namespace based on endpoint
	nsPrefix := "trt"
	if strings.Contains(endpoint, "device_service") {
		nsPrefix = "tds"
	}

	soapRequest := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:%s="http://www.onvif.org/ver10/device/wsdl">
	<s:Header>%s</s:Header>
	<s:Body>%s</s:Body>
</s:Envelope>`, nsPrefix, authHeader, body)

	req, err := http.NewRequest("POST", endpoint, bytes.NewBufferString(soapRequest))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/soap+xml; charset=utf-8")
	req.Header.Set("SOAPAction", action)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

func getDeviceInformation(camera *Camera, username, password string) error {
	// Get first address if multiple are provided
	address := strings.Fields(camera.Address)[0]

	// Use device service URL directly
	deviceURL := address

	// Get device information
	deviceInfoBody := `<tds:GetDeviceInformation/>`
	deviceInfoResp, err := sendSOAPRequest(deviceURL, username, password,
		"http://www.onvif.org/ver10/device/wsdl/GetDeviceInformation", deviceInfoBody)
	if err != nil {
		return fmt.Errorf("failed to get device information: %v", err)
	}

	// Check for SOAP fault
	if bytes.Contains(deviceInfoResp, []byte("SOAP-ENV:Fault")) {
		return fmt.Errorf("SOAP fault in device information response")
	}

	// Parse device information response
	type DeviceInfoResponse struct {
		Manufacturer    string `xml:"Body>GetDeviceInformationResponse>Manufacturer"`
		Model           string `xml:"Body>GetDeviceInformationResponse>Model"`
		FirmwareVersion string `xml:"Body>GetDeviceInformationResponse>FirmwareVersion"`
		SerialNumber    string `xml:"Body>GetDeviceInformationResponse>SerialNumber"`
		HardwareId      string `xml:"Body>GetDeviceInformationResponse>HardwareId"`
	}

	var deviceInfo DeviceInfoResponse
	if err := xml.Unmarshal(deviceInfoResp, &deviceInfo); err != nil {
		// Try to extract values manually if structured parsing fails
		respStr := string(deviceInfoResp)

		// Extract Manufacturer
		if start := strings.Index(respStr, "<tds:Manufacturer>"); start != -1 {
			start += len("<tds:Manufacturer>")
			if end := strings.Index(respStr[start:], "</tds:Manufacturer>"); end != -1 {
				deviceInfo.Manufacturer = respStr[start:start+end]
			}
		}

		// Extract Model
		if start := strings.Index(respStr, "<tds:Model>"); start != -1 {
			start += len("<tds:Model>")
			if end := strings.Index(respStr[start:], "</tds:Model>"); end != -1 {
				deviceInfo.Model = respStr[start:start+end]
			}
		}

		// Extract SerialNumber
		if start := strings.Index(respStr, "<tds:SerialNumber>"); start != -1 {
			start += len("<tds:SerialNumber>")
			if end := strings.Index(respStr[start:], "</tds:SerialNumber>"); end != -1 {
				deviceInfo.SerialNumber = respStr[start:start+end]
			}
		}

		// Extract FirmwareVersion
		if start := strings.Index(respStr, "<tds:FirmwareVersion>"); start != -1 {
			start += len("<tds:FirmwareVersion>")
			if end := strings.Index(respStr[start:], "</tds:FirmwareVersion>"); end != -1 {
				deviceInfo.FirmwareVersion = respStr[start:start+end]
			}
		}

		// Extract HardwareId
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

	// Get hostname
	hostnameBody := `<tds:GetHostname/>`
	hostnameResp, err := sendSOAPRequest(deviceURL, username, password,
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

	// Get system date and time
	dateTimeBody := `<tds:GetSystemDateAndTime/>`
	dateTimeResp, err := sendSOAPRequest(deviceURL, username, password,
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

	// Get capabilities
	capabilitiesBody := `<tds:GetCapabilities><tds:Category>All</tds:Category></tds:GetCapabilities>`
	capabilitiesResp, err := sendSOAPRequest(deviceURL, username, password,
		"http://www.onvif.org/ver10/device/wsdl/GetCapabilities", capabilitiesBody)
	if err == nil {
		// Simple parsing for key capabilities
		respStr := string(capabilitiesResp)

		// Check for PTZ support
		camera.PTZSupport = strings.Contains(respStr, "PTZ>") && strings.Contains(respStr, "XAddr")

		// Check for Analytics support
		camera.AnalyticsSupport = strings.Contains(respStr, "Analytics>") && strings.Contains(respStr, "XAddr")

		// Parse media capabilities for source counts
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

func main() {
	var username, password string
	flag.StringVar(&username, "user", "", "ONVIF username")
	flag.StringVar(&password, "pass", "", "ONVIF password")
	flag.Parse()

	fmt.Println("===========================================")
	fmt.Println("   Enhanced ONVIF Camera Discovery Scanner")
	fmt.Println("===========================================")
	fmt.Println()

	cameras, err := discoverCameras()
	if err != nil {
		log.Fatalf("Discovery failed: %v", err)
	}

	if len(cameras) == 0 {
		fmt.Println("❌ No ONVIF cameras found on the network.")
		fmt.Println("\nPlease ensure:")
		fmt.Println("  • Cameras are powered on and connected to the network")
		fmt.Println("  • ONVIF is enabled on the cameras")
		fmt.Println("  • This computer is on the same network segment")
		return
	}

	fmt.Printf("✅ Found %d ONVIF camera(s) on the network\n\n", len(cameras))

	// Fetch additional information if credentials provided
	if username != "" && password != "" {
		fmt.Println("📊 Fetching detailed device information...")
		fmt.Println()
	}

	for i, camera := range cameras {
		fmt.Printf("Camera #%d\n", i+1)
		fmt.Println(strings.Repeat("=", 60))

		// Basic information from discovery
		if camera.Name != "" {
			fmt.Printf("📷 Name:         %s\n", camera.Name)
		}

		if camera.Model != "" {
			fmt.Printf("🔧 Model:        %s (from discovery)\n", camera.Model)
		}

		if camera.Location != "" {
			fmt.Printf("📍 Location:     %s\n", camera.Location)
		}

		fmt.Printf("🌐 Address:      %s\n", camera.Address)

		if len(camera.Profiles) > 0 {
			fmt.Printf("📋 Services:     %s\n", strings.Join(camera.Profiles, ", "))
		}

		// Fetch additional information if credentials provided
		if username != "" && password != "" {
			if err := getDeviceInformation(&cameras[i], username, password); err != nil {
				fmt.Printf("⚠️  Failed to fetch device details: %v\n", err)
			} else {
				fmt.Println("\n📊 Device Information:")
				fmt.Println(strings.Repeat("-", 40))

				if cameras[i].Manufacturer != "" {
					fmt.Printf("   Manufacturer:    %s\n", cameras[i].Manufacturer)
				}
				if cameras[i].DeviceModel != "" {
					fmt.Printf("   Model:           %s\n", cameras[i].DeviceModel)
				}
				if cameras[i].SerialNumber != "" {
					fmt.Printf("   Serial Number:   %s\n", cameras[i].SerialNumber)
				}
				if cameras[i].FirmwareVersion != "" {
					fmt.Printf("   Firmware:        %s\n", cameras[i].FirmwareVersion)
				}
				if cameras[i].HardwareId != "" {
					fmt.Printf("   Hardware ID:     %s\n", cameras[i].HardwareId)
				}
				if cameras[i].Hostname != "" {
					fmt.Printf("   Hostname:        %s", cameras[i].Hostname)
					if cameras[i].HostnameFrom != "" {
						fmt.Printf(" (from %s)", cameras[i].HostnameFrom)
					}
					fmt.Println()
				}

				if cameras[i].DateTime != "" {
					fmt.Printf("\n⏰ System Time:      %s\n", cameras[i].DateTime)
				}
				if cameras[i].TimeZone != "" && cameras[i].TimeZone != "UTC" {
					fmt.Printf("   Time Zone:       %s\n", cameras[i].TimeZone)
				}

				// Show capabilities if any are non-zero
				if cameras[i].VideoSources > 0 || cameras[i].AudioSources > 0 ||
				   cameras[i].PTZSupport || cameras[i].AnalyticsSupport {
					fmt.Println("\n🎛️  Capabilities:")
					if cameras[i].VideoSources > 0 {
						fmt.Printf("   Video Sources:   %d\n", cameras[i].VideoSources)
					}
					if cameras[i].VideoOutputs > 0 {
						fmt.Printf("   Video Outputs:   %d\n", cameras[i].VideoOutputs)
					}
					if cameras[i].AudioSources > 0 {
						fmt.Printf("   Audio Sources:   %d\n", cameras[i].AudioSources)
					}
					if cameras[i].AudioOutputs > 0 {
						fmt.Printf("   Audio Outputs:   %d\n", cameras[i].AudioOutputs)
					}
					if cameras[i].RelayOutputs > 0 {
						fmt.Printf("   Relay Outputs:   %d\n", cameras[i].RelayOutputs)
					}
					if cameras[i].PTZSupport {
						fmt.Printf("   PTZ Support:     ✅\n")
					}
					if cameras[i].AnalyticsSupport {
						fmt.Printf("   Analytics:       ✅\n")
					}
				}
			}
		} else {
			fmt.Println("\n💡 Tip: Use -user and -pass flags for detailed device information")
		}

		fmt.Println()
	}
}