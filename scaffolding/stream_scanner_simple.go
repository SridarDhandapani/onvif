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
	Name          string
	Address       string
	Profiles      []string
	Model         string
	Location      string
	StreamConfigs []StreamConfig
}

type StreamConfig struct {
	ProfileName  string
	ProfileToken string
	Resolution   string
	Framerate    int
	Bitrate      int
	Encoding     string
	StreamURI    string
	Quality      string
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

func fetchStreamProfiles(camera *Camera, username, password string) error {
	// Convert device service URL to media service URL
	mediaURL := strings.Replace(camera.Address, "/device_service", "/media_service", 1)
	if !strings.Contains(mediaURL, "media_service") {
		// Try alternative pattern
		u, err := url.Parse(camera.Address)
		if err != nil {
			return fmt.Errorf("invalid camera address: %v", err)
		}
		u.Path = "/onvif/media_service"
		mediaURL = u.String()
	}

	// Get profiles
	profilesBody := `<trt:GetProfiles/>`
	profilesResp, err := sendSOAPRequest(mediaURL, username, password, "http://www.onvif.org/ver10/media/wsdl/GetProfiles", profilesBody)
	if err != nil {
		return fmt.Errorf("failed to get profiles: %v", err)
	}

	// Parse profiles response
	type ProfilesResponse struct {
		Profiles []struct {
			Token string `xml:"token,attr"`
			Name  string `xml:"Name"`
			VideoEncoderConfiguration struct {
				Encoding   string `xml:"Encoding"`
				Resolution struct {
					Width  int `xml:"Width"`
					Height int `xml:"Height"`
				} `xml:"Resolution"`
				RateControl struct {
					FrameRateLimit int `xml:"FrameRateLimit"`
					BitrateLimit   int `xml:"BitrateLimit"`
				} `xml:"RateControl"`
			} `xml:"VideoEncoderConfiguration"`
		} `xml:"Body>GetProfilesResponse>Profiles"`
	}

	var profiles ProfilesResponse
	if err := xml.Unmarshal(profilesResp, &profiles); err != nil {
		return fmt.Errorf("failed to parse profiles: %v", err)
	}

	// Process each profile
	for _, profile := range profiles.Profiles {
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

		streamResp, err := sendSOAPRequest(mediaURL, username, password, "http://www.onvif.org/ver10/media/wsdl/GetStreamUri", streamBody)
		if err != nil {
			log.Printf("Failed to get stream URI for profile %s: %v", profile.Name, err)
			continue
		}

		// Parse stream URI response
		type StreamUriResponse struct {
			MediaUri struct {
				Uri string `xml:"Uri"`
			} `xml:"Body>GetStreamUriResponse>MediaUri"`
		}

		var streamUri StreamUriResponse
		if err := xml.Unmarshal(streamResp, &streamUri); err != nil {
			log.Printf("Failed to parse stream URI for profile %s: %v", profile.Name, err)
			continue
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
			StreamURI: streamUri.MediaUri.Uri,
			Quality:   quality,
		}

		camera.StreamConfigs = append(camera.StreamConfigs, config)
	}

	return nil
}

func main() {
	var username, password string
	flag.StringVar(&username, "user", "", "ONVIF username")
	flag.StringVar(&password, "pass", "", "ONVIF password")
	flag.Parse()

	fmt.Println("===========================================")
	fmt.Println("   ONVIF Camera Stream Configuration Scanner")
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

	fmt.Printf("✅ Found %d ONVIF camera(s) on the network:\n\n", len(cameras))

	for i, camera := range cameras {
		fmt.Printf("Camera #%d\n", i+1)
		fmt.Println(strings.Repeat("=", 60))

		if camera.Name != "" {
			fmt.Printf("📷 Name:     %s\n", camera.Name)
		}

		if camera.Model != "" {
			fmt.Printf("🔧 Model:    %s\n", camera.Model)
		}

		if camera.Location != "" {
			fmt.Printf("📍 Location: %s\n", camera.Location)
		}

		fmt.Printf("🌐 Address:  %s\n", camera.Address)

		if len(camera.Profiles) > 0 {
			fmt.Printf("📋 Services: %s\n", strings.Join(camera.Profiles, ", "))
		}

		if username != "" && password != "" {
			fmt.Println("\n📹 Stream Configurations:")
			fmt.Println(strings.Repeat("-", 60))

			if err := fetchStreamProfiles(&cameras[i], username, password); err != nil {
				fmt.Printf("   ⚠️  Failed to fetch profiles: %v\n", err)
			} else if len(cameras[i].StreamConfigs) == 0 {
				fmt.Println("   No stream profiles found")
			} else {
				for j, config := range cameras[i].StreamConfigs {
					fmt.Printf("\n   Stream #%d (%s Stream - %s):\n", j+1, config.Quality, config.ProfileName)
					fmt.Printf("   • Resolution : %s\n", config.Resolution)
					fmt.Printf("   • Encoding   : %s\n", config.Encoding)
					if config.Framerate > 0 {
						fmt.Printf("   • Framerate  : %d fps\n", config.Framerate)
					}
					if config.Bitrate > 0 {
						fmt.Printf("   • Bitrate    : %d kbps\n", config.Bitrate/1000)
					}
					if config.StreamURI != "" {
						fmt.Printf("   • RTSP URI   : %s\n", config.StreamURI)
					}
				}
			}
		} else {
			fmt.Println("\n💡 Tip: Use -user and -pass flags to fetch stream configurations")
		}

		fmt.Println()
	}
}