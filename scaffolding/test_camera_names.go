package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

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

func testCameraNames(address, username, password, cameraName string) {
	fmt.Printf("\n📷 Testing camera: %s\n", cameraName)
	fmt.Printf("   Address: %s\n", address)
	fmt.Println(strings.Repeat("-", 60))

	// 1. Try GetHostname
	fmt.Println("\n1. GetHostname:")
	hostnameBody := `<tds:GetHostname/>`
	hostnameResp, err := sendSOAPRequest(address, username, password,
		"http://www.onvif.org/ver10/device/wsdl/GetHostname", hostnameBody)

	if err != nil {
		fmt.Printf("   ❌ Error: %v\n", err)
	} else if bytes.Contains(hostnameResp, []byte("SOAP-ENV:Fault")) {
		fmt.Println("   ❌ SOAP Fault")
	} else {
		// Parse hostname
		type HostnameResponse struct {
			HostnameInfo struct {
				FromDHCP bool   `xml:"FromDHCP,attr"`
				Name     string `xml:"Name"`
			} `xml:"Body>GetHostnameResponse>HostnameInformation"`
		}

		var hostname HostnameResponse
		if err := xml.Unmarshal(hostnameResp, &hostname); err == nil {
			fmt.Printf("   ✅ Hostname: %s (FromDHCP: %v)\n", hostname.HostnameInfo.Name, hostname.HostnameInfo.FromDHCP)
		} else {
			// Try manual extraction
			respStr := string(hostnameResp)
			if start := strings.Index(respStr, "<tds:Name>"); start != -1 {
				start += len("<tds:Name>")
				if end := strings.Index(respStr[start:], "</tds:Name>"); end != -1 {
					fmt.Printf("   ✅ Hostname: %s\n", respStr[start:start+end])
				}
			} else if start := strings.Index(respStr, "<tt:Name>"); start != -1 {
				start += len("<tt:Name>")
				if end := strings.Index(respStr[start:], "</tt:Name>"); end != -1 {
					fmt.Printf("   ✅ Hostname: %s\n", respStr[start:start+end])
				}
			} else {
				fmt.Printf("   ⚠️  Could not parse hostname from response\n")
			}
		}
	}

	// 2. Try GetScopes
	fmt.Println("\n2. GetScopes:")
	scopesBody := `<tds:GetScopes/>`
	scopesResp, err := sendSOAPRequest(address, username, password,
		"http://www.onvif.org/ver10/device/wsdl/GetScopes", scopesBody)

	if err != nil {
		fmt.Printf("   ❌ Error: %v\n", err)
	} else if bytes.Contains(scopesResp, []byte("SOAP-ENV:Fault")) {
		fmt.Println("   ❌ SOAP Fault")
	} else {
		// Parse scopes
		type ScopesResponse struct {
			Scopes []struct {
				ScopeDef string `xml:"ScopeDef,attr"`
				ScopeItem string `xml:"ScopeItem,attr"`
			} `xml:"Body>GetScopesResponse>Scopes"`
		}

		var scopes ScopesResponse
		if err := xml.Unmarshal(scopesResp, &scopes); err == nil {
			for _, scope := range scopes.Scopes {
				// Look for name-related scopes
				if strings.Contains(scope.ScopeItem, "name/") {
					name := scope.ScopeItem[strings.Index(scope.ScopeItem, "name/")+5:]
					name = strings.Replace(name, "_", " ", -1)
					fmt.Printf("   ✅ Name Scope: %s\n", name)
				}
				// Look for hostname
				if strings.Contains(scope.ScopeItem, "hostname/") {
					hostname := scope.ScopeItem[strings.Index(scope.ScopeItem, "hostname/")+9:]
					fmt.Printf("   ✅ Hostname Scope: %s\n", hostname)
				}
			}
		} else {
			fmt.Printf("   ⚠️  Could not parse scopes\n")
		}
	}

	// 3. Try GetDeviceInformation
	fmt.Println("\n3. GetDeviceInformation:")
	deviceInfoBody := `<tds:GetDeviceInformation/>`
	deviceInfoResp, err := sendSOAPRequest(address, username, password,
		"http://www.onvif.org/ver10/device/wsdl/GetDeviceInformation", deviceInfoBody)

	if err != nil {
		fmt.Printf("   ❌ Error: %v\n", err)
	} else if bytes.Contains(deviceInfoResp, []byte("SOAP-ENV:Fault")) {
		fmt.Println("   ❌ SOAP Fault")
	} else {
		respStr := string(deviceInfoResp)

		// Extract Manufacturer
		if start := strings.Index(respStr, "<tds:Manufacturer>"); start != -1 {
			start += len("<tds:Manufacturer>")
			if end := strings.Index(respStr[start:], "</tds:Manufacturer>"); end != -1 {
				fmt.Printf("   ✅ Manufacturer: %s\n", respStr[start:start+end])
			}
		}

		// Extract Model
		if start := strings.Index(respStr, "<tds:Model>"); start != -1 {
			start += len("<tds:Model>")
			if end := strings.Index(respStr[start:], "</tds:Model>"); end != -1 {
				fmt.Printf("   ✅ Model: %s\n", respStr[start:start+end])
			}
		}

		// Sometimes the camera name is in FirmwareVersion or HardwareId
		if start := strings.Index(respStr, "<tds:HardwareId>"); start != -1 {
			start += len("<tds:HardwareId>")
			if end := strings.Index(respStr[start:], "</tds:HardwareId>"); end != -1 {
				fmt.Printf("   ✅ HardwareId: %s\n", respStr[start:start+end])
			}
		}
	}

	// 4. Try GetNetworkInterfaces to get network name
	fmt.Println("\n4. GetNetworkInterfaces:")
	netInterfacesBody := `<tds:GetNetworkInterfaces/>`
	netInterfacesResp, err := sendSOAPRequest(address, username, password,
		"http://www.onvif.org/ver10/device/wsdl/GetNetworkInterfaces", netInterfacesBody)

	if err != nil {
		fmt.Printf("   ❌ Error: %v\n", err)
	} else if bytes.Contains(netInterfacesResp, []byte("SOAP-ENV:Fault")) {
		fmt.Println("   ❌ SOAP Fault")
	} else {
		respStr := string(netInterfacesResp)

		// Look for Info/Name field
		if start := strings.Index(respStr, "<tt:Name>"); start != -1 {
			start += len("<tt:Name>")
			if end := strings.Index(respStr[start:], "</tt:Name>"); end != -1 {
				fmt.Printf("   ✅ Interface Name: %s\n", respStr[start:start+end])
			}
		}
	}

	// 5. Check OSD (On-Screen Display) text
	fmt.Println("\n5. GetOSDs (On-Screen Display):")
	// First need to get a video source token
	mediaURL := strings.Replace(address, "/device_service", "/media_service", 1)

	// Get profiles first
	profilesBody := `<trt:GetProfiles xmlns:trt="http://www.onvif.org/ver10/media/wsdl"/>`
	soapRequest := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:trt="http://www.onvif.org/ver10/media/wsdl">
	<s:Body>%s</s:Body>
</s:Envelope>`, profilesBody)

	req, _ := http.NewRequest("POST", mediaURL, bytes.NewBufferString(soapRequest))
	req.Header.Set("Content-Type", "application/soap+xml; charset=utf-8")
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)

	if err == nil {
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		respStr := string(respBody)

		// Try to get OSD info
		if strings.Contains(respStr, "OSD") {
			fmt.Println("   ✅ OSD configuration found")
		} else {
			fmt.Println("   ⚠️  No OSD configuration found")
		}
	}
}

func main() {
	username := "admin"
	password := "H0xt0nA!"

	fmt.Println("===========================================")
	fmt.Println("   ONVIF Camera Name Discovery Test")
	fmt.Println("===========================================")

	// Test each camera
	cameras := []struct {
		name    string
		address string
	}{
		{"HC35W45R3", "http://192.168.50.162/onvif/device_service"},
		{"i-PRO WV-U2130LA", "http://192.168.50.109/onvif/device_service"},
		{"RLC-520A", "http://192.168.50.161:8000/onvif/device_service"},
	}

	for _, camera := range cameras {
		testCameraNames(camera.address, username, password, camera.name)
		fmt.Println()
	}
}