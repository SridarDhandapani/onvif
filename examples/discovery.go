package main

import (
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/SridarDhandapani/onvif"
)

func main() {
	var username, password string
	var detailed, insecure bool

	flag.StringVar(&username, "user", "", "ONVIF username")
	flag.StringVar(&password, "pass", "", "ONVIF password")
	flag.BoolVar(&detailed, "detailed", false, "Fetch detailed device information")
	flag.BoolVar(&insecure, "insecure", false, "Skip TLS certificate verification")
	flag.Parse()

	fmt.Println("===========================================")
	fmt.Println("   ONVIF Camera Discovery Example")
	fmt.Println("===========================================")
	fmt.Println()

	// Discover cameras
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

	// Create client if credentials provided
	var client *onvif.Client
	if username != "" && password != "" {
		client = onvif.NewClient(username, password)
		client.InsecureTLS = insecure
	}

	// Display camera information
	for i, camera := range cameras {
		fmt.Printf("Camera #%d\n", i+1)
		fmt.Println(strings.Repeat("=", 60))

		// Basic info from discovery
		fmt.Printf("📷 Name:     %s\n", camera.GetDisplayName())
		fmt.Printf("🌐 Address:  %s\n", camera.Address)

		if len(camera.Profiles) > 0 {
			fmt.Printf("📋 Services: %s\n", strings.Join(camera.Profiles, ", "))
		}

		// Fetch detailed information if requested and credentials provided
		if detailed && client != nil {
			fmt.Println("\nFetching detailed information...")

			if err := client.GetDeviceInformation(&camera); err != nil {
				fmt.Printf("⚠️  Failed to fetch details: %v\n", err)
			} else {
				if camera.Manufacturer != "" {
					fmt.Printf("🏭 Manufacturer: %s\n", camera.Manufacturer)
				}
				if camera.DeviceModel != "" {
					fmt.Printf("📱 Model:        %s\n", camera.DeviceModel)
				}
				if camera.SerialNumber != "" {
					fmt.Printf("🔢 Serial:       %s\n", camera.SerialNumber)
				}
				if camera.Hostname != "" {
					fmt.Printf("💻 Hostname:     %s\n", camera.Hostname)
				}
				if camera.FirmwareVersion != "" {
					fmt.Printf("📀 Firmware:     %s\n", camera.FirmwareVersion)
				}
				if camera.DateTime != "" {
					fmt.Printf("⏰ Time:         %s\n", camera.DateTime)
				}
			}

			// Get stream profiles
			streams, err := client.GetStreamProfiles(&camera)
			if err == nil && len(streams) > 0 {
				fmt.Println("\n📹 Stream Configurations:")
				for j, stream := range streams {
					fmt.Printf("   %d. %s (%s): %s @ %dfps, %s\n",
						j+1,
						stream.ProfileName,
						stream.Quality,
						stream.Resolution,
						stream.Framerate,
						stream.Encoding)

					if stream.StreamURI != "" {
						fmt.Printf("      RTSP: %s\n", stream.StreamURI)
					}
				}
			}
		}

		fmt.Println()
	}
}