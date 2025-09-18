package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/SridarDhandapani/onvif"
)

func main() {
	var username, password string

	flag.StringVar(&username, "user", "", "ONVIF username (required)")
	flag.StringVar(&password, "pass", "", "ONVIF password (required)")
	flag.Parse()

	if username == "" || password == "" {
		fmt.Println("Username and password are required")
		flag.Usage()
		return
	}

	fmt.Println("===========================================")
	fmt.Println("   ONVIF Set Date/Time Example")
	fmt.Println("===========================================")
	fmt.Printf("Setting all cameras to: %s GMT\n\n", time.Now().UTC().Format("2006-01-02 15:04:05"))

	// Create client
	client := onvif.NewClient(username, password)

	// Discover cameras
	cameras, err := onvif.DiscoverCameras(nil)
	if err != nil {
		log.Fatalf("Discovery failed: %v", err)
	}

	if len(cameras) == 0 {
		fmt.Println("No cameras found")
		return
	}

	// Update each camera
	successCount := 0
	for _, camera := range cameras {
		// Get current time first
		client.GetSystemDateTime(&camera)
		fmt.Printf("%s\n", camera.GetDisplayName())
		fmt.Printf("  Current: %s (TZ: %s)\n", camera.DateTime, camera.TimeZone)

		// Set new time
		fmt.Printf("  Setting to GMT...")
		if err := client.SetSystemDateTime(&camera); err != nil {
			fmt.Printf(" ❌ Failed: %v\n", err)
		} else {
			fmt.Printf(" ✅ Success\n")
			successCount++

			// Verify
			time.Sleep(1 * time.Second)
			client.GetSystemDateTime(&camera)
			fmt.Printf("  Verified: %s (TZ: %s)\n", camera.DateTime, camera.TimeZone)
		}
		fmt.Println()
	}

	fmt.Printf("Successfully updated %d/%d cameras\n", successCount, len(cameras))
}