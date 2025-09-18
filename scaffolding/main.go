package main

import (
	"encoding/xml"
	"fmt"
	"log"
	"net"
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
	MessageID    string `xml:"MessageID"`
	RelatesTo    string `xml:"RelatesTo"`
	To           string `xml:"To"`
	Action       string `xml:"Action"`
	AppSequence  AppSeq `xml:"AppSequence"`
}

type AppSeq struct {
	InstanceId   int    `xml:"InstanceId,attr"`
	MessageNumber int   `xml:"MessageNumber,attr"`
}

type Body struct {
	ProbeMatches ProbeMatches `xml:"ProbeMatches"`
}

type ProbeMatches struct {
	ProbeMatch []ProbeMatch `xml:"ProbeMatch"`
}

type ProbeMatch struct {
	EndpointReference EndpointRef `xml:"EndpointReference"`
	Types            string      `xml:"Types"`
	Scopes           string      `xml:"Scopes"`
	XAddrs           string      `xml:"XAddrs"`
	MetadataVersion  int         `xml:"MetadataVersion"`
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
		// Create a unique key based on the camera properties
		// Use EndpointReference Address or XAddrs as the unique identifier
		key := camera.Address

		// If Address is empty, use name and model as fallback
		if key == "" {
			key = fmt.Sprintf("%s_%s", camera.Name, camera.Model)
		}

		// Only add if we haven't seen this camera before
		if _, exists := cameraMap[key]; !exists {
			cameraMap[key] = camera
		}
	}

	// Convert map back to slice
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

	// Deduplicate cameras before returning
	return deduplicateCameras(cameras), nil
}

func main() {
	fmt.Println("===========================================")
	fmt.Println("      ONVIF Camera Discovery Scanner")
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
		fmt.Println(strings.Repeat("-", 40))

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
			fmt.Printf("📋 Profiles: %s\n", strings.Join(camera.Profiles, ", "))
		}

		fmt.Println()
	}
}