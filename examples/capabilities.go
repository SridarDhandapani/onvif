package main

import (
	"bytes"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/SridarDhandapani/onvif"
)

// yesNo renders a boolean capability flag as a human-readable mark.
func yesNo(b bool) string {
	if b {
		return "✅ yes"
	}
	return "❌ no"
}

func main() {
	var username, password string
	var insecure, raw, verbose bool

	flag.StringVar(&username, "user", "", "ONVIF username")
	flag.StringVar(&password, "pass", "", "ONVIF password")
	flag.BoolVar(&insecure, "insecure", false, "Skip TLS certificate verification")
	flag.BoolVar(&raw, "raw", false, "Print the raw GetCapabilities XML response")
	flag.BoolVar(&verbose, "verbose", false, "Print extended parsed capabilities (network, system, events, RTP)")
	flag.Parse()

	fmt.Println("===========================================")
	fmt.Println("   ONVIF Camera Capabilities Report")
	fmt.Println("===========================================")
	fmt.Println()

	// Discover cameras on the local network.
	fmt.Println("🔍 Discovering ONVIF cameras on the network...")
	cameras, err := onvif.DiscoverCameras(nil)
	if err != nil {
		log.Fatalf("Discovery failed: %v", err)
	}
	if len(cameras) == 0 {
		fmt.Println("❌ No ONVIF cameras found on the network.")
		return
	}
	fmt.Printf("✅ Found %d camera(s)\n\n", len(cameras))

	// Capabilities are reported only over an authenticated session on most
	// cameras, so credentials are required to query GetCapabilities/GetServices.
	if username == "" || password == "" {
		fmt.Println("⚠️  No credentials provided (-user/-pass).")
		fmt.Println("    Capabilities require authentication on most cameras; only")
		fmt.Println("    discovery-advertised service profiles will be shown.")
		fmt.Println()
	}

	client := onvif.NewClient(username, password)
	client.InsecureTLS = insecure

	for i := range cameras {
		camera := cameras[i]

		fmt.Printf("Camera #%d\n", i+1)
		fmt.Println(strings.Repeat("=", 60))
		fmt.Printf("📷 Name:    %s\n", camera.GetDisplayName())
		fmt.Printf("🌐 Address: %s\n", camera.Address)

		// Service profiles advertised during WS-Discovery (no auth needed).
		if len(camera.Profiles) > 0 {
			fmt.Printf("📋 Discovery profiles: %s\n", strings.Join(camera.Profiles, ", "))
		}

		if username == "" || password == "" {
			fmt.Println()
			continue
		}

		// Pull device info; this populates the structured capability fields.
		if err := client.GetDeviceInformation(&camera); err != nil {
			fmt.Printf("⚠️  Failed to query capabilities: %v\n", err)
			fmt.Println()
			continue
		}

		if camera.Manufacturer != "" || camera.DeviceModel != "" {
			fmt.Printf("🏭 Device:  %s %s (firmware %s)\n",
				camera.Manufacturer, camera.DeviceModel, camera.FirmwareVersion)
		}

		fmt.Println("\n🧩 Reported capabilities:")
		fmt.Printf("   PTZ support:       %s\n", yesNo(camera.PTZSupport))
		fmt.Printf("   Analytics support: %s\n", yesNo(camera.AnalyticsSupport))
		fmt.Printf("   Video sources:     %d\n", camera.VideoSources)
		fmt.Printf("   Video outputs:     %d\n", camera.VideoOutputs)
		fmt.Printf("   Audio sources:     %d\n", camera.AudioSources)
		fmt.Printf("   Audio outputs:     %d\n", camera.AudioOutputs)
		fmt.Printf("   Relay outputs:     %d\n", camera.RelayOutputs)

		fmt.Println("\n🔗 Service endpoints:")
		printService("Media (ver10)", camera.MediaURL)
		printService("Media2 (ver20)", camera.Media2URL)
		printService("Imaging", camera.ImagingURL)
		printService("PTZ", camera.PTZURL)

		// -verbose and -raw both need the full GetCapabilities response, which
		// carries far more than the structured Camera fields expose.
		if verbose || raw {
			capsXML, err := client.GetCapabilitiesRaw(&camera)
			if err != nil && capsXML == nil {
				fmt.Printf("\n⚠️  Failed to fetch raw capabilities: %v\n", err)
			} else {
				if verbose {
					printVerbose(capsXML)
					printStreamProfiles(client, &camera)
				}
				if raw {
					fmt.Println("\n📄 Raw GetCapabilities response:")
					fmt.Println(prettyXML(capsXML))
				}
			}
		}

		fmt.Println()
	}
}

// printService prints a service endpoint, marking unsupported/undiscovered ones.
func printService(name, url string) {
	if url == "" {
		fmt.Printf("   %-15s —\n", name)
		return
	}
	fmt.Printf("   %-15s %s\n", name, url)
}

// capsExtended mirrors the parts of the GetCapabilities response that the
// library's structured Camera fields don't surface. Everything is best-effort:
// fields a camera omits simply stay at their zero value.
type capsExtended struct {
	Device struct {
		XAddr   string `xml:"XAddr"`
		Network struct {
			IPFilter          bool `xml:"IPFilter"`
			ZeroConfiguration bool `xml:"ZeroConfiguration"`
			IPVersion6        bool `xml:"IPVersion6"`
			DynDNS            bool `xml:"DynDNS"`
		} `xml:"Network"`
		System struct {
			DiscoveryResolve  bool `xml:"DiscoveryResolve"`
			DiscoveryBye      bool `xml:"DiscoveryBye"`
			RemoteDiscovery   bool `xml:"RemoteDiscovery"`
			SystemBackup      bool `xml:"SystemBackup"`
			SystemLogging     bool `xml:"SystemLogging"`
			FirmwareUpgrade   bool `xml:"FirmwareUpgrade"`
			SupportedVersions []struct {
				Major int `xml:"Major"`
				Minor int `xml:"Minor"`
			} `xml:"SupportedVersions"`
		} `xml:"System"`
	} `xml:"Body>GetCapabilitiesResponse>Capabilities>Device"`
	Events struct {
		XAddr                                  string `xml:"XAddr"`
		WSSubscriptionPolicySupport            bool   `xml:"WSSubscriptionPolicySupport"`
		WSPullPointSupport                     bool   `xml:"WSPullPointSupport"`
		WSPausableSubscriptionManagerInterface bool   `xml:"WSPausableSubscriptionManagerInterfaceSupport"`
	} `xml:"Body>GetCapabilitiesResponse>Capabilities>Events"`
	Media struct {
		XAddr         string `xml:"XAddr"`
		StreamingCaps struct {
			RTPMulticast bool `xml:"RTPMulticast"`
			RTP_TCP      bool `xml:"RTP_TCP"`
			RTP_RTSP_TCP bool `xml:"RTP_RTSP_TCP"`
		} `xml:"StreamingCapabilities"`
	} `xml:"Body>GetCapabilitiesResponse>Capabilities>Media"`
	Analytics struct {
		XAddr                  string `xml:"XAddr"`
		RuleSupport            bool   `xml:"RuleSupport"`
		AnalyticsModuleSupport bool   `xml:"AnalyticsModuleSupport"`
	} `xml:"Body>GetCapabilitiesResponse>Capabilities>Analytics"`
}

// printVerbose parses the raw capabilities for the extra detail the structured
// Camera fields omit and prints whatever the device reported.
func printVerbose(capsXML []byte) {
	var ext capsExtended
	if err := xml.Unmarshal(capsXML, &ext); err != nil {
		fmt.Printf("\n⚠️  Could not parse extended capabilities: %v\n", err)
		return
	}

	fmt.Println("\n🔎 Extended capabilities:")

	fmt.Println("   Device:")
	printBool("IP filtering", ext.Device.Network.IPFilter)
	printBool("Zero-config", ext.Device.Network.ZeroConfiguration)
	printBool("IPv6", ext.Device.Network.IPVersion6)
	printBool("Dynamic DNS", ext.Device.Network.DynDNS)
	printBool("Remote discovery", ext.Device.System.RemoteDiscovery)
	printBool("System backup", ext.Device.System.SystemBackup)
	printBool("System logging", ext.Device.System.SystemLogging)
	printBool("Firmware upgrade", ext.Device.System.FirmwareUpgrade)
	if vers := ext.Device.System.SupportedVersions; len(vers) > 0 {
		parts := make([]string, 0, len(vers))
		for _, v := range vers {
			parts = append(parts, fmt.Sprintf("%d.%02d", v.Major, v.Minor))
		}
		fmt.Printf("      %-18s %s\n", "ONVIF versions", strings.Join(parts, ", "))
	}

	fmt.Println("   Events:")
	printBool("WS-Subscription", ext.Events.WSSubscriptionPolicySupport)
	printBool("WS-PullPoint", ext.Events.WSPullPointSupport)
	printBool("Pausable subs", ext.Events.WSPausableSubscriptionManagerInterface)

	fmt.Println("   Media streaming:")
	printBool("RTP multicast", ext.Media.StreamingCaps.RTPMulticast)
	printBool("RTP over TCP", ext.Media.StreamingCaps.RTP_TCP)
	printBool("RTP/RTSP/TCP", ext.Media.StreamingCaps.RTP_RTSP_TCP)

	if ext.Analytics.XAddr != "" {
		fmt.Println("   Analytics:")
		printBool("Rule support", ext.Analytics.RuleSupport)
		printBool("Module support", ext.Analytics.AnalyticsModuleSupport)
	}
}

// printStreamProfiles lists the media profiles the camera exposes — a concrete
// view of its streaming capability beyond the boolean flags.
func printStreamProfiles(client *onvif.Client, camera *onvif.Camera) {
	streams, err := client.GetStreamProfiles(camera)
	if err != nil || len(streams) == 0 {
		return
	}
	fmt.Println("\n📹 Stream profiles:")
	for j, s := range streams {
		fmt.Printf("   %d. %s (%s): %s @ %dfps, %s, %dkbps\n",
			j+1, s.ProfileName, s.Quality, s.Resolution, s.Framerate, s.Encoding, s.Bitrate)
	}
}

// printBool prints an indented, aligned boolean capability line.
func printBool(name string, b bool) {
	fmt.Printf("      %-18s %s\n", name, yesNo(b))
}

// prettyXML re-indents an XML document for readable output, falling back to the
// original bytes if the response isn't well-formed enough to re-encode.
func prettyXML(b []byte) string {
	dec := xml.NewDecoder(bytes.NewReader(b))
	var out bytes.Buffer
	enc := xml.NewEncoder(&out)
	enc.Indent("", "  ")
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return string(b)
		}
		if err := enc.EncodeToken(tok); err != nil {
			return string(b)
		}
	}
	if err := enc.Flush(); err != nil {
		return string(b)
	}
	return out.String()
}
