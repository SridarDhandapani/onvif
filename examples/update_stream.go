package main

import (
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/SridarDhandapani/onvif"
)

func main() {
	var username, password, cameraIP string
	var width, height, framerate int

	flag.StringVar(&username, "user", "", "ONVIF username (required)")
	flag.StringVar(&password, "pass", "", "ONVIF password (required)")
	flag.StringVar(&cameraIP, "ip", "", "Camera IP address (optional, updates all if not specified)")
	flag.IntVar(&width, "width", 640, "Target width")
	flag.IntVar(&height, "height", 480, "Target height")
	flag.IntVar(&framerate, "fps", 10, "Target framerate")
	flag.Parse()

	if username == "" || password == "" {
		fmt.Println("Username and password are required")
		flag.Usage()
		return
	}

	fmt.Println("===========================================")
	fmt.Println("   ONVIF Stream Update Example")
	fmt.Println("===========================================")
	fmt.Printf("Target: %dx%d @ %d fps\n\n", width, height, framerate)

	// Create client
	client := onvif.NewClient(username, password)

	// Discover cameras
	cameras, err := onvif.DiscoverCameras(nil)
	if err != nil {
		log.Fatalf("Discovery failed: %v", err)
	}

	// Filter by IP if specified
	var targetCameras []onvif.Camera
	if cameraIP != "" {
		for _, camera := range cameras {
			if strings.Contains(camera.Address, cameraIP) {
				targetCameras = append(targetCameras, camera)
				break
			}
		}
		if len(targetCameras) == 0 {
			log.Fatalf("No camera found with IP: %s", cameraIP)
		}
	} else {
		targetCameras = cameras
	}

	// Update configuration
	config := onvif.StreamUpdateConfig{
		Resolution: onvif.Resolution{Width: width, Height: height},
		Framerate:  framerate,
		Bitrate:    1024000, // 1 Mbps
		Encoding:   "H264",
	}

	// Update each camera's sub stream
	for _, camera := range targetCameras {
		fmt.Printf("Updating %s...\n", camera.GetDisplayName())

		if err := client.UpdateSubStream(&camera, config); err != nil {
			fmt.Printf("  ❌ Failed: %v\n", err)
		} else {
			fmt.Printf("  ✅ Successfully updated sub stream\n")
		}
	}
}