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
	"net/url"
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
	Name     string
	Address  string
	Profiles []string
	Model    string
	Location string
}

type VideoEncoderConfig struct {
	Token            string
	Name             string
	Encoding         string
	Width            int
	Height           int
	FrameRateLimit   int
	BitrateLimit     int
	EncodingInterval int
	Quality          float32
	ProfileToken     string
	ProfileName      string
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
		key := camera.Address
		if key == "" {
			key = fmt.Sprintf("%s_%s", camera.Name, camera.Model)
		}

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

	soapRequest := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:trt="http://www.onvif.org/ver10/media/wsdl">
	<s:Header>%s</s:Header>
	<s:Body>%s</s:Body>
</s:Envelope>`, authHeader, body)

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

func getVideoEncoderConfigurations(camera *Camera, username, password string) ([]VideoEncoderConfig, error) {
	// Get first address if multiple are provided
	address := strings.Fields(camera.Address)[0]

	// Convert device service URL to media service URL
	mediaURL := strings.Replace(address, "/device_service", "/media_service", 1)
	if !strings.Contains(mediaURL, "media_service") {
		u, err := url.Parse(address)
		if err != nil {
			return nil, fmt.Errorf("invalid camera address: %v", err)
		}
		u.Path = "/onvif/media_service"
		mediaURL = u.String()
	}

	fmt.Printf("   Using media URL: %s\n", mediaURL)

	// First, get profiles (which works in the simple scanner)
	profilesBody := `<trt:GetProfiles/>`
	profilesResp, err := sendSOAPRequest(mediaURL, username, password, "http://www.onvif.org/ver10/media/wsdl/GetProfiles", profilesBody)
	if err != nil {
		return nil, fmt.Errorf("failed to get encoder configurations: %v", err)
	}

	// Debug: print response
	fmt.Printf("   Response length: %d bytes\n", len(profilesResp))

	// Check for SOAP fault
	if strings.Contains(string(profilesResp), "SOAP-ENV:Fault") {
		if strings.Contains(string(profilesResp), "NotAuthorized") {
			return nil, fmt.Errorf("authentication failed - check credentials")
		}
		return nil, fmt.Errorf("SOAP fault in response")
	}

	// Parse profiles response (same structure as working scanner)
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
					EncodingInterval int `xml:"EncodingInterval"`
					BitrateLimit     int `xml:"BitrateLimit"`
				} `xml:"RateControl"`
				Quality float32 `xml:"Quality"`
			} `xml:"VideoEncoderConfiguration"`
		} `xml:"Body>GetProfilesResponse>Profiles"`
	}

	var profiles ProfilesResponse
	if err := xml.Unmarshal(profilesResp, &profiles); err != nil {
		return nil, fmt.Errorf("failed to parse profiles: %v", err)
	}

	// Convert to our structure using data from profiles
	var encoderConfigs []VideoEncoderConfig
	for _, profile := range profiles.Profiles {
		if profile.VideoEncoderConfiguration.Token == "" {
			continue // Skip profiles without video encoder config
		}

		ec := VideoEncoderConfig{
			Token:            profile.VideoEncoderConfiguration.Token,
			Name:             profile.VideoEncoderConfiguration.Name,
			Encoding:         profile.VideoEncoderConfiguration.Encoding,
			Width:            profile.VideoEncoderConfiguration.Resolution.Width,
			Height:           profile.VideoEncoderConfiguration.Resolution.Height,
			FrameRateLimit:   profile.VideoEncoderConfiguration.RateControl.FrameRateLimit,
			BitrateLimit:     profile.VideoEncoderConfiguration.RateControl.BitrateLimit,
			EncodingInterval: profile.VideoEncoderConfiguration.RateControl.EncodingInterval,
			Quality:          profile.VideoEncoderConfiguration.Quality,
			ProfileToken:     profile.Token,
			ProfileName:      profile.Name,
		}

		encoderConfigs = append(encoderConfigs, ec)
	}

	return encoderConfigs, nil
}

func isSubStream(configs []VideoEncoderConfig, config VideoEncoderConfig) bool {
	// Sort configs by resolution (descending)
	type resConfig struct {
		resolution int
		config     VideoEncoderConfig
	}

	var sorted []resConfig
	for _, c := range configs {
		sorted = append(sorted, resConfig{
			resolution: c.Width * c.Height,
			config:     c,
		})
	}

	// Sort by resolution
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].resolution > sorted[i].resolution {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	// If we have exactly 3 streams, the middle one is typically the sub stream
	// The largest is main, middle is sub, smallest is often mobile/tertiary
	if len(sorted) >= 2 {
		// Check if this is the second largest stream
		if sorted[1].config.Token == config.Token {
			return true
		}
	}

	// Fallback: check name patterns for stream2
	nameLower := strings.ToLower(config.Name)
	if strings.Contains(nameLower, "stream2") {
		return true
	}

	return false
}

func updateVideoEncoderConfiguration(camera *Camera, username, password string, config VideoEncoderConfig) error {
	// Get first address if multiple are provided
	address := strings.Fields(camera.Address)[0]

	// Convert device service URL to media service URL
	mediaURL := strings.Replace(address, "/device_service", "/media_service", 1)
	if !strings.Contains(mediaURL, "media_service") {
		u, err := url.Parse(address)
		if err != nil {
			return fmt.Errorf("invalid camera address: %v", err)
		}
		u.Path = "/onvif/media_service"
		mediaURL = u.String()
	}

	// Set video encoder configuration with new parameters
	setConfigBody := fmt.Sprintf(`
	<trt:SetVideoEncoderConfiguration>
		<trt:Configuration token="%s">
			<tt:Name xmlns:tt="http://www.onvif.org/ver10/schema">%s</tt:Name>
			<tt:UseCount xmlns:tt="http://www.onvif.org/ver10/schema">0</tt:UseCount>
			<tt:Encoding xmlns:tt="http://www.onvif.org/ver10/schema">H264</tt:Encoding>
			<tt:Resolution xmlns:tt="http://www.onvif.org/ver10/schema">
				<tt:Width>640</tt:Width>
				<tt:Height>480</tt:Height>
			</tt:Resolution>
			<tt:Quality xmlns:tt="http://www.onvif.org/ver10/schema">%.1f</tt:Quality>
			<tt:RateControl xmlns:tt="http://www.onvif.org/ver10/schema">
				<tt:FrameRateLimit>10</tt:FrameRateLimit>
				<tt:EncodingInterval>%d</tt:EncodingInterval>
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
		config.Token,
		config.Name,
		config.Quality,
		config.EncodingInterval,
		config.BitrateLimit)

	_, err := sendSOAPRequest(mediaURL, username, password, "http://www.onvif.org/ver10/media/wsdl/SetVideoEncoderConfiguration", setConfigBody)
	if err != nil {
		return fmt.Errorf("failed to set configuration: %v", err)
	}

	return nil
}

func main() {
	var username, password string
	var dryRun bool
	flag.StringVar(&username, "user", "", "ONVIF username (required)")
	flag.StringVar(&password, "pass", "", "ONVIF password (required)")
	flag.BoolVar(&dryRun, "dry-run", false, "Show what would be changed without making changes")
	flag.Parse()

	if username == "" || password == "" {
		fmt.Println("❌ Error: Username and password are required")
		fmt.Println("Usage: go run stream_updater.go -user USERNAME -pass PASSWORD")
		flag.PrintDefaults()
		return
	}

	fmt.Println("===========================================")
	fmt.Println("   ONVIF Sub Stream Configuration Updater")
	fmt.Println("===========================================")
	fmt.Println()
	fmt.Println("Target Configuration:")
	fmt.Println("• Resolution: 640x480")
	fmt.Println("• Framerate: 10 fps")
	fmt.Println("• Encoding: H.264")
	fmt.Println()

	cameras, err := discoverCameras()
	if err != nil {
		log.Fatalf("Discovery failed: %v", err)
	}

	if len(cameras) == 0 {
		fmt.Println("❌ No ONVIF cameras found on the network.")
		return
	}

	fmt.Printf("✅ Found %d ONVIF camera(s)\n\n", len(cameras))

	successCount := 0
	failureCount := 0

	for i, camera := range cameras {
		fmt.Printf("Camera #%d: %s (%s)\n", i+1, camera.Name, camera.Address)
		fmt.Println(strings.Repeat("-", 60))

		// Get video encoder configurations
		configs, err := getVideoEncoderConfigurations(&camera, username, password)
		if err != nil {
			fmt.Printf("❌ Failed to get configurations: %v\n\n", err)
			failureCount++
			continue
		}

		// Show all configurations found
		fmt.Printf("📊 Found %d video encoder configuration(s):\n", len(configs))
		for _, config := range configs {
			fmt.Printf("   • %s: %dx%d @ %dfps, %s\n",
				config.Name, config.Width, config.Height, config.FrameRateLimit, config.Encoding)
		}

		// Find and update sub streams
		subStreamFound := false
		for _, config := range configs {
			if isSubStream(configs, config) {
				subStreamFound = true

				fmt.Printf("📹 Identified as sub stream: %s\n", config.Name)
				fmt.Printf("   Current: %dx%d, %d fps, %s\n",
					config.Width, config.Height, config.FrameRateLimit, config.Encoding)

				if dryRun {
					fmt.Printf("   🔄 Would update to: 640x480, 10 fps, H264\n")
				} else {
					fmt.Printf("   🔄 Updating to: 640x480, 10 fps, H264...")

					err := updateVideoEncoderConfiguration(&camera, username, password, config)
					if err != nil {
						fmt.Printf(" ❌ Failed: %v\n", err)
						failureCount++
					} else {
						fmt.Printf(" ✅ Success!\n")
						successCount++
					}
				}
			}
		}

		if !subStreamFound {
			fmt.Printf("⚠️  No sub stream configuration found\n")
		}

		fmt.Println()
	}

	// Summary
	fmt.Println(strings.Repeat("=", 60))
	if dryRun {
		fmt.Println("🔍 Dry run completed - no changes were made")
	} else {
		fmt.Printf("📊 Summary: %d successful updates, %d failures\n", successCount, failureCount)

		if successCount > 0 {
			fmt.Println("\n✅ Sub streams have been updated to:")
			fmt.Println("   • Resolution: 640x480")
			fmt.Println("   • Framerate: 10 fps")
			fmt.Println("   • Encoding: H264")
		}
	}
}