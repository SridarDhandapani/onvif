package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/xml"
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
		}
	}

	return profiles
}

func deduplicateCameras(cameras []Camera) []Camera {
	cameraMap := make(map[string]Camera)

	for _, camera := range cameras {
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

	soapRequest := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:tds="http://www.onvif.org/ver10/device/wsdl">
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

func getCurrentDateTime(camera *Camera, username, password string) (string, string, error) {
	// Get first address if multiple are provided
	address := strings.Fields(camera.Address)[0]

	// Get current date/time from camera
	dateTimeBody := `<tds:GetSystemDateAndTime/>`
	dateTimeResp, err := sendSOAPRequest(address, username, password,
		"http://www.onvif.org/ver10/device/wsdl/GetSystemDateAndTime", dateTimeBody)
	if err != nil {
		return "", "", fmt.Errorf("failed to get current date/time: %v", err)
	}

	// Parse response to get current timezone
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
	if err := xml.Unmarshal(dateTimeResp, &dateTime); err != nil {
		return "", "", fmt.Errorf("failed to parse date/time response: %v", err)
	}

	currentTime := ""
	if dateTime.UTCDateTime.Date.Year > 0 {
		currentTime = fmt.Sprintf("%04d-%02d-%02d %02d:%02d:%02d UTC",
			dateTime.UTCDateTime.Date.Year,
			dateTime.UTCDateTime.Date.Month,
			dateTime.UTCDateTime.Date.Day,
			dateTime.UTCDateTime.Time.Hour,
			dateTime.UTCDateTime.Time.Minute,
			dateTime.UTCDateTime.Time.Second)
	}

	return currentTime, dateTime.TimeZone.TZ, nil
}

func setSystemDateTime(camera *Camera, username, password string) error {
	// Get first address if multiple are provided
	address := strings.Fields(camera.Address)[0]

	// Get current GMT time
	now := time.Now().UTC()

	// Create SetSystemDateAndTime request with GMT timezone
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

	setResp, err := sendSOAPRequest(address, username, password,
		"http://www.onvif.org/ver10/device/wsdl/SetSystemDateAndTime", setDateTimeBody)
	if err != nil {
		return fmt.Errorf("failed to set date/time: %v", err)
	}

	// Check for SOAP fault
	if bytes.Contains(setResp, []byte("SOAP-ENV:Fault")) {
		if bytes.Contains(setResp, []byte("NotAuthorized")) {
			return fmt.Errorf("not authorized to set date/time")
		}
		return fmt.Errorf("SOAP fault in response")
	}

	return nil
}

func main() {
	username := "admin"
	password := "H0xt0nA!"

	fmt.Println("===========================================")
	fmt.Println("   ONVIF Camera Date/Time Configuration")
	fmt.Println("===========================================")
	fmt.Println()
	fmt.Println("Target Configuration:")
	fmt.Printf("• Date/Time: Current GMT (%s)\n", time.Now().UTC().Format("2006-01-02 15:04:05"))
	fmt.Println("• Timezone:  GMT")
	fmt.Println()

	cameras, err := discoverCameras()
	if err != nil {
		log.Fatalf("Discovery failed: %v", err)
	}

	if len(cameras) == 0 {
		fmt.Println("❌ No ONVIF cameras found on the network.")
		return
	}

	fmt.Printf("✅ Found %d ONVIF camera(s) on the network\n\n", len(cameras))

	successCount := 0
	failureCount := 0

	for i, camera := range cameras {
		fmt.Printf("Camera #%d: %s (%s)\n", i+1, camera.Name, camera.Model)
		fmt.Println(strings.Repeat("-", 60))

		// Get current date/time
		currentTime, currentTZ, err := getCurrentDateTime(&camera, username, password)
		if err != nil {
			fmt.Printf("   ❌ Failed to get current time: %v\n", err)
			failureCount++
		} else {
			fmt.Printf("   Current:  %s (TZ: %s)\n", currentTime, currentTZ)

			// Set new date/time
			fmt.Printf("   Setting:  %s (TZ: GMT)...", time.Now().UTC().Format("2006-01-02 15:04:05"))

			if err := setSystemDateTime(&camera, username, password); err != nil {
				fmt.Printf(" ❌ Failed: %v\n", err)
				failureCount++
			} else {
				fmt.Printf(" ✅ Success!\n")
				successCount++

				// Verify the change
				time.Sleep(1 * time.Second)
				newTime, newTZ, err := getCurrentDateTime(&camera, username, password)
				if err == nil {
					fmt.Printf("   Verified: %s (TZ: %s)\n", newTime, newTZ)
				}
			}
		}

		fmt.Println()
	}

	// Summary
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("📊 Summary: %d successful updates, %d failures\n", successCount, failureCount)

	if successCount > 0 {
		fmt.Println("\n✅ Camera date/time has been updated to:")
		fmt.Printf("   • Date/Time: %s\n", time.Now().UTC().Format("2006-01-02 15:04:05 UTC"))
		fmt.Println("   • Timezone:  GMT")
	}
}