package main

import (
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/SridarDhandapani/onvif"
)

func main() {
	var username, password, cameraIP, mode string
	var insecure bool

	flag.StringVar(&username, "user", "", "ONVIF username (required)")
	flag.StringVar(&password, "pass", "", "ONVIF password (required)")
	flag.StringVar(&cameraIP, "camera", "", "Camera IP address (required)")
	flag.StringVar(&mode, "mode", "", "IR mode to set: on, off, auto (omit to show current)")
	flag.BoolVar(&insecure, "insecure", false, "Skip TLS certificate verification")
	flag.Parse()

	if username == "" || password == "" || cameraIP == "" {
		fmt.Println("Username, password, and camera IP are required")
		fmt.Println("\nUsage examples:")
		fmt.Println("  Get current IR mode:")
		fmt.Println("    go run ir_mode.go -camera 192.168.1.100 -user admin -pass secret")
		fmt.Println()
		fmt.Println("  Set IR mode:")
		fmt.Println("    go run ir_mode.go -camera 192.168.1.100 -user admin -pass secret -mode auto")
		fmt.Println("    go run ir_mode.go -camera 192.168.1.100 -user admin -pass secret -mode off")
		fmt.Println("    go run ir_mode.go -camera 192.168.1.100 -user admin -pass secret -mode on")
		flag.Usage()
		return
	}

	client := onvif.NewClient(username, password)
	client.InsecureTLS = insecure

	camera := onvif.Camera{
		Address: fmt.Sprintf("http://%s/onvif/device_service", cameraIP),
	}

	// Get current settings
	settings, err := client.GetImagingSettings(&camera)
	if err != nil {
		log.Fatalf("Failed to get imaging settings: %v", err)
	}

	fmt.Printf("Camera: %s\n", cameraIP)
	fmt.Printf("Video source: %s\n", settings.VideoSourceToken)
	fmt.Printf("IR cut filter: %s\n", settings.IrCutFilter)

	if mode == "" {
		return
	}

	// Set new mode
	var newMode onvif.IrCutFilterMode
	switch strings.ToLower(mode) {
	case "on":
		newMode = onvif.IrCutFilterOn
	case "off":
		newMode = onvif.IrCutFilterOff
	case "auto":
		newMode = onvif.IrCutFilterAuto
	default:
		log.Fatalf("Invalid mode '%s': use on, off, or auto", mode)
	}

	fmt.Printf("\nSetting IR cut filter to: %s\n", newMode)
	if err := client.SetIrCutFilter(&camera, newMode); err != nil {
		log.Fatalf("Failed to set IR cut filter: %v", err)
	}
	fmt.Println("Success!")

	// Verify
	settings, err = client.GetImagingSettings(&camera)
	if err != nil {
		log.Fatalf("Failed to verify: %v", err)
	}
	fmt.Printf("Verified IR cut filter: %s\n", settings.IrCutFilter)
}
