package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/SridarDhandapani/onvif"
)

func main() {
	// Current credentials (for authentication)
	var currentUser, currentPass string
	// Camera target
	var cameraIP string
	// Operation modes
	var listUsers, initSetup bool
	// New user credentials (for create/modify)
	var newUser, newPass string
	var userLevel string
	var insecure bool

	flag.StringVar(&currentUser, "user", "", "Current ONVIF username (empty for anonymous/factory-reset)")
	flag.StringVar(&currentPass, "pass", "", "Current ONVIF password (empty for anonymous/factory-reset)")
	flag.StringVar(&cameraIP, "camera", "", "Camera IP address (required)")
	flag.BoolVar(&listUsers, "list", false, "List all users")
	flag.BoolVar(&initSetup, "init", false, "Initialize/setup admin credentials on factory-reset camera")
	flag.StringVar(&newUser, "new-user", "", "New username to create or modify")
	flag.StringVar(&newPass, "new-pass", "", "New password for the user")
	flag.StringVar(&userLevel, "level", "Administrator", "User level: Administrator, Operator, User")
	flag.BoolVar(&insecure, "insecure", false, "Skip TLS certificate verification")
	flag.Parse()

	if cameraIP == "" {
		fmt.Println("Camera IP is required")
		fmt.Println("\nUsage examples:")
		fmt.Println("  List users (with auth):")
		fmt.Println("    go run user_management.go -camera 192.168.1.100 -user admin -pass secret -list")
		fmt.Println()
		fmt.Println("  List users (factory-reset, no auth):")
		fmt.Println("    go run user_management.go -camera 192.168.1.100 -list")
		fmt.Println()
		fmt.Println("  Setup initial admin on factory-reset camera:")
		fmt.Println("    go run user_management.go -camera 192.168.1.100 -init -new-user admin -new-pass MySecurePass123")
		fmt.Println()
		fmt.Println("  Create new user (with existing admin auth):")
		fmt.Println("    go run user_management.go -camera 192.168.1.100 -user admin -pass secret -new-user operator1 -new-pass OpPass123 -level Operator")
		fmt.Println()
		fmt.Println("  Change existing user's password:")
		fmt.Println("    go run user_management.go -camera 192.168.1.100 -user admin -pass secret -new-user admin -new-pass NewAdminPass")
		flag.Usage()
		os.Exit(1)
	}

	// Create client (empty credentials for factory-reset/anonymous access)
	client := onvif.NewClient(currentUser, currentPass)
	client.InsecureTLS = insecure

	camera := onvif.Camera{
		Address: fmt.Sprintf("http://%s/onvif/device_service", cameraIP),
	}

	fmt.Printf("Camera: %s\n", cameraIP)
	fmt.Printf("Auth: %s\n", authStatus(currentUser))
	fmt.Println()

	// List users
	if listUsers || (!initSetup && newUser == "") {
		fmt.Println("--- Current Users ---")
		users, err := client.GetUsers(&camera)
		if err != nil {
			log.Fatalf("Failed to get users: %v", err)
		}

		if len(users) == 0 {
			fmt.Println("No users found (factory-reset state)")
		} else {
			for _, u := range users {
				fmt.Printf("  - %s (%s)\n", u.Username, u.UserLevel)
			}
		}

		if listUsers && newUser == "" {
			return
		}
		fmt.Println()
	}

	// Initialize admin on factory-reset camera
	if initSetup {
		if newUser == "" || newPass == "" {
			log.Fatal("-new-user and -new-pass are required for -init")
		}

		fmt.Printf("--- Initializing Admin User ---\n")
		fmt.Printf("Creating user '%s' with Administrator level...\n", newUser)

		err := client.CreateUser(&camera, newUser, newPass, onvif.UserLevelAdministrator)
		if err != nil {
			log.Fatalf("Failed to create admin user: %v", err)
		}

		// Verify by authenticating with the new credentials
		verifyClient := onvif.NewClient(newUser, newPass)
		verifyClient.InsecureTLS = insecure
		users, err := verifyClient.GetUsers(&camera)
		if err != nil {
			log.Fatalf("User creation appeared to succeed but verification failed: %v\n"+
				"The camera may not be in factory-reset state, or may already have credentials configured.", err)
		}

		fmt.Printf("Success! Admin user '%s' created.\n", newUser)
		fmt.Println("\nVerified users:")
		for _, u := range users {
			fmt.Printf("  - %s (%s)\n", u.Username, u.UserLevel)
		}
		fmt.Printf("\nYou can now use:\n")
		fmt.Printf("  -user %s -pass <your-password>\n", newUser)
		return
	}

	// Create or modify user
	if newUser != "" && newPass != "" {
		// Check if user exists
		users, err := client.GetUsers(&camera)
		if err != nil {
			log.Fatalf("Failed to get users: %v", err)
		}

		userExists := false
		for _, u := range users {
			if u.Username == newUser {
				userExists = true
				break
			}
		}

		level := parseUserLevel(userLevel)

		if userExists {
			fmt.Printf("--- Modifying User '%s' ---\n", newUser)
			err = client.SetUser(&camera, onvif.User{
				Username:  newUser,
				Password:  newPass,
				UserLevel: level,
			})
			if err != nil {
				log.Fatalf("Failed to modify user: %v", err)
			}
			fmt.Printf("User '%s' updated (level: %s)\n", newUser, level)
		} else {
			fmt.Printf("--- Creating User '%s' ---\n", newUser)
			err = client.CreateUser(&camera, newUser, newPass, level)
			if err != nil {
				log.Fatalf("Failed to create user: %v", err)
			}
			fmt.Printf("User '%s' created (level: %s)\n", newUser, level)
		}

		// Verify
		fmt.Println("\n--- Updated User List ---")
		users, _ = client.GetUsers(&camera)
		for _, u := range users {
			fmt.Printf("  - %s (%s)\n", u.Username, u.UserLevel)
		}
	}
}

func authStatus(user string) string {
	if user == "" {
		return "anonymous (no credentials)"
	}
	return fmt.Sprintf("as '%s'", user)
}

func parseUserLevel(level string) onvif.UserLevel {
	switch level {
	case "Administrator", "admin":
		return onvif.UserLevelAdministrator
	case "Operator", "operator":
		return onvif.UserLevelOperator
	case "User", "user":
		return onvif.UserLevelUser
	case "Anonymous", "anonymous":
		return onvif.UserLevelAnonymous
	default:
		return onvif.UserLevelAdministrator
	}
}
