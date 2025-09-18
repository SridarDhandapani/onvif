package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
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

func main() {
	username := "admin"
	password := "H0xt0nA!"
	cameraIP := "192.168.50.162"
	mediaURL := fmt.Sprintf("http://%s/onvif/media_service", cameraIP)

	fmt.Println("===========================================")
	fmt.Println("   HC35W45R3 Sub Stream Configuration Update")
	fmt.Println("===========================================")
	fmt.Println()
	fmt.Printf("Target Camera: HC35W45R3 @ %s\n", cameraIP)
	fmt.Println("Target Configuration:")
	fmt.Println("• Resolution: 640x480")
	fmt.Println("• Framerate: 10 fps")
	fmt.Println("• Encoding: H.264")
	fmt.Println()

	// Step 1: Get current profiles
	fmt.Println("Step 1: Fetching current profiles...")
	profilesBody := `<trt:GetProfiles/>`
	profilesResp, err := sendSOAPRequest(mediaURL, username, password, "http://www.onvif.org/ver10/media/wsdl/GetProfiles", profilesBody)
	if err != nil {
		log.Fatalf("Failed to get profiles: %v", err)
	}

	// Parse profiles response
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
					BitrateLimit     int `xml:"BitrateLimit"`
					EncodingInterval int `xml:"EncodingInterval"`
				} `xml:"RateControl"`
				Quality float32 `xml:"Quality"`
				H264 struct {
					GovLength    int    `xml:"GovLength"`
					H264Profile  string `xml:"H264Profile"`
				} `xml:"H264"`
			} `xml:"VideoEncoderConfiguration"`
		} `xml:"Body>GetProfilesResponse>Profiles"`
	}

	var profiles ProfilesResponse
	if err := xml.Unmarshal(profilesResp, &profiles); err != nil {
		log.Fatalf("Failed to parse profiles: %v", err)
	}

	fmt.Printf("Found %d profiles\n\n", len(profiles.Profiles))

	// Step 2: Find and update stream2 (the sub stream)
	for _, profile := range profiles.Profiles {
		if profile.Name == "profile_cam1_stream2" {
			fmt.Printf("Found sub stream profile: %s\n", profile.Name)
			fmt.Printf("Current configuration:\n")
			fmt.Printf("  • Token: %s\n", profile.VideoEncoderConfiguration.Token)
			fmt.Printf("  • Resolution: %dx%d\n",
				profile.VideoEncoderConfiguration.Resolution.Width,
				profile.VideoEncoderConfiguration.Resolution.Height)
			fmt.Printf("  • Framerate: %d fps\n", profile.VideoEncoderConfiguration.RateControl.FrameRateLimit)
			fmt.Printf("  • Encoding: %s\n", profile.VideoEncoderConfiguration.Encoding)
			fmt.Println()

			// Step 3: Update the configuration
			fmt.Println("Step 2: Updating sub stream configuration...")

			encoderToken := profile.VideoEncoderConfiguration.Token
			encoderName := profile.VideoEncoderConfiguration.Name

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
					<tt:Quality xmlns:tt="http://www.onvif.org/ver10/schema">3.0</tt:Quality>
					<tt:RateControl xmlns:tt="http://www.onvif.org/ver10/schema">
						<tt:FrameRateLimit>10</tt:FrameRateLimit>
						<tt:EncodingInterval>1</tt:EncodingInterval>
						<tt:BitrateLimit>1024</tt:BitrateLimit>
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
				encoderToken, encoderName)

			updateResp, err := sendSOAPRequest(mediaURL, username, password,
				"http://www.onvif.org/ver10/media/wsdl/SetVideoEncoderConfiguration", setConfigBody)

			if err != nil {
				fmt.Printf("❌ Failed to update configuration: %v\n", err)
			} else {
				// Check for SOAP fault in response
				if bytes.Contains(updateResp, []byte("SOAP-ENV:Fault")) {
					fmt.Printf("❌ SOAP Fault in response:\n%s\n", string(updateResp))
				} else {
					fmt.Println("✅ Configuration updated successfully!")

					// Step 4: Verify the change
					fmt.Println("\nStep 3: Verifying update...")
					time.Sleep(2 * time.Second) // Give camera time to apply changes

					verifyResp, err := sendSOAPRequest(mediaURL, username, password,
						"http://www.onvif.org/ver10/media/wsdl/GetProfiles", profilesBody)
					if err == nil {
						var verifyProfiles ProfilesResponse
						if xml.Unmarshal(verifyResp, &verifyProfiles) == nil {
							for _, vProfile := range verifyProfiles.Profiles {
								if vProfile.Name == "profile_cam1_stream2" {
									fmt.Println("New configuration:")
									fmt.Printf("  • Resolution: %dx%d\n",
										vProfile.VideoEncoderConfiguration.Resolution.Width,
										vProfile.VideoEncoderConfiguration.Resolution.Height)
									fmt.Printf("  • Framerate: %d fps\n",
										vProfile.VideoEncoderConfiguration.RateControl.FrameRateLimit)
									fmt.Printf("  • Encoding: %s\n", vProfile.VideoEncoderConfiguration.Encoding)

									if vProfile.VideoEncoderConfiguration.Resolution.Width == 640 &&
									   vProfile.VideoEncoderConfiguration.Resolution.Height == 480 &&
									   vProfile.VideoEncoderConfiguration.RateControl.FrameRateLimit == 10 {
										fmt.Println("\n✅ Sub stream successfully updated to 640x480 @ 10fps!")
									}
									break
								}
							}
						}
					}
				}
			}
			break
		}
	}
}