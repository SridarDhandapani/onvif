package onvif

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
)

// GetUsers retrieves all users from the camera
func (c *Client) GetUsers(camera *Camera) ([]User, error) {
	address := getFirstAddress(camera.Address)

	body := `<tds:GetUsers/>`
	resp, err := c.sendSOAPRequest(address,
		"http://www.onvif.org/ver10/device/wsdl/GetUsers", body)
	if err != nil {
		return nil, fmt.Errorf("failed to get users: %v", err)
	}

	if err := parseSOAPFault(resp); err != nil {
		return nil, err
	}

	// Try structured parsing first
	type GetUsersResponse struct {
		Users []struct {
			Username  string `xml:"Username"`
			UserLevel string `xml:"UserLevel"`
		} `xml:"Body>GetUsersResponse>User"`
	}

	var usersResp GetUsersResponse
	var users []User

	if err := xml.Unmarshal(resp, &usersResp); err == nil && len(usersResp.Users) > 0 {
		for _, u := range usersResp.Users {
			users = append(users, User{
				Username:  u.Username,
				UserLevel: UserLevel(u.UserLevel),
			})
		}
		return users, nil
	}

	// Fallback to manual extraction
	respStr := string(resp)
	for {
		userStart := strings.Index(respStr, "<tds:User>")
		if userStart == -1 {
			userStart = strings.Index(respStr, "<tt:User>")
		}
		if userStart == -1 {
			break
		}

		userEnd := strings.Index(respStr[userStart:], "</tds:User>")
		if userEnd == -1 {
			userEnd = strings.Index(respStr[userStart:], "</tt:User>")
		}
		if userEnd == -1 {
			break
		}

		userXML := respStr[userStart : userStart+userEnd]
		var user User

		// Extract username
		if start := strings.Index(userXML, "<tds:Username>"); start != -1 {
			start += len("<tds:Username>")
			if end := strings.Index(userXML[start:], "</tds:Username>"); end != -1 {
				user.Username = userXML[start : start+end]
			}
		} else if start := strings.Index(userXML, "<tt:Username>"); start != -1 {
			start += len("<tt:Username>")
			if end := strings.Index(userXML[start:], "</tt:Username>"); end != -1 {
				user.Username = userXML[start : start+end]
			}
		}

		// Extract user level
		if start := strings.Index(userXML, "<tds:UserLevel>"); start != -1 {
			start += len("<tds:UserLevel>")
			if end := strings.Index(userXML[start:], "</tds:UserLevel>"); end != -1 {
				user.UserLevel = UserLevel(userXML[start : start+end])
			}
		} else if start := strings.Index(userXML, "<tt:UserLevel>"); start != -1 {
			start += len("<tt:UserLevel>")
			if end := strings.Index(userXML[start:], "</tt:UserLevel>"); end != -1 {
				user.UserLevel = UserLevel(userXML[start : start+end])
			}
		}

		if user.Username != "" {
			users = append(users, user)
		}

		respStr = respStr[userStart+userEnd:]
	}

	return users, nil
}

// CreateUsers creates multiple users on the camera
func (c *Client) CreateUsers(camera *Camera, users []User) error {
	address := getFirstAddress(camera.Address)

	var usersXML strings.Builder
	usersXML.WriteString("<tds:CreateUsers>")
	for _, user := range users {
		usersXML.WriteString("<tds:User>")
		usersXML.WriteString("<tt:Username xmlns:tt=\"http://www.onvif.org/ver10/schema\">")
		usersXML.WriteString(escapeXML(user.Username))
		usersXML.WriteString("</tt:Username>")
		usersXML.WriteString("<tt:Password xmlns:tt=\"http://www.onvif.org/ver10/schema\">")
		usersXML.WriteString(escapeXML(user.Password))
		usersXML.WriteString("</tt:Password>")
		usersXML.WriteString("<tt:UserLevel xmlns:tt=\"http://www.onvif.org/ver10/schema\">")
		usersXML.WriteString(string(user.UserLevel))
		usersXML.WriteString("</tt:UserLevel>")
		usersXML.WriteString("</tds:User>")
	}
	usersXML.WriteString("</tds:CreateUsers>")

	resp, err := c.sendSOAPRequest(address,
		"http://www.onvif.org/ver10/device/wsdl/CreateUsers", usersXML.String())
	if err != nil {
		return fmt.Errorf("failed to create users: %v", err)
	}

	if err := parseSOAPFault(resp); err != nil {
		return err
	}

	return nil
}

// CreateUser creates a single user on the camera (convenience wrapper)
func (c *Client) CreateUser(camera *Camera, username, password string, level UserLevel) error {
	return c.CreateUsers(camera, []User{{
		Username:  username,
		Password:  password,
		UserLevel: level,
	}})
}

// SetUser modifies an existing user's password and/or level
func (c *Client) SetUser(camera *Camera, user User) error {
	address := getFirstAddress(camera.Address)

	body := fmt.Sprintf(`<tds:SetUser>
		<tds:User>
			<tt:Username xmlns:tt="http://www.onvif.org/ver10/schema">%s</tt:Username>
			<tt:Password xmlns:tt="http://www.onvif.org/ver10/schema">%s</tt:Password>
			<tt:UserLevel xmlns:tt="http://www.onvif.org/ver10/schema">%s</tt:UserLevel>
		</tds:User>
	</tds:SetUser>`,
		escapeXML(user.Username),
		escapeXML(user.Password),
		string(user.UserLevel))

	resp, err := c.sendSOAPRequest(address,
		"http://www.onvif.org/ver10/device/wsdl/SetUser", body)
	if err != nil {
		return fmt.Errorf("failed to set user: %v", err)
	}

	if err := parseSOAPFault(resp); err != nil {
		return err
	}

	return nil
}

// SetUserPassword changes a user's password (convenience wrapper)
// Note: This requires knowing the user's current level
func (c *Client) SetUserPassword(camera *Camera, username, newPassword string) error {
	// First get the user's current level
	users, err := c.GetUsers(camera)
	if err != nil {
		return fmt.Errorf("failed to get current user info: %v", err)
	}

	var userLevel UserLevel
	found := false
	for _, u := range users {
		if u.Username == username {
			userLevel = u.UserLevel
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("user '%s' not found", username)
	}

	return c.SetUser(camera, User{
		Username:  username,
		Password:  newPassword,
		UserLevel: userLevel,
	})
}

// DeleteUsers deletes multiple users from the camera
func (c *Client) DeleteUsers(camera *Camera, usernames []string) error {
	address := getFirstAddress(camera.Address)

	var usernamesXML strings.Builder
	usernamesXML.WriteString("<tds:DeleteUsers>")
	for _, username := range usernames {
		usernamesXML.WriteString("<tds:Username>")
		usernamesXML.WriteString(escapeXML(username))
		usernamesXML.WriteString("</tds:Username>")
	}
	usernamesXML.WriteString("</tds:DeleteUsers>")

	resp, err := c.sendSOAPRequest(address,
		"http://www.onvif.org/ver10/device/wsdl/DeleteUsers", usernamesXML.String())
	if err != nil {
		return fmt.Errorf("failed to delete users: %v", err)
	}

	if err := parseSOAPFault(resp); err != nil {
		return err
	}

	return nil
}

// DeleteUser deletes a single user from the camera (convenience wrapper)
func (c *Client) DeleteUser(camera *Camera, username string) error {
	return c.DeleteUsers(camera, []string{username})
}

// escapeXML escapes special XML characters in a string
func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

// parseSOAPFault checks for SOAP faults and returns a descriptive error
func parseSOAPFault(resp []byte) error {
	if !bytes.Contains(resp, []byte("SOAP-ENV:Fault")) && !bytes.Contains(resp, []byte("s:Fault")) {
		return nil
	}

	respStr := string(resp)

	// Check for common ONVIF-specific fault codes
	if strings.Contains(respStr, "ter:UsernameClash") {
		return fmt.Errorf("username already exists")
	}
	if strings.Contains(respStr, "ter:UsernameMissing") {
		return fmt.Errorf("username not found")
	}
	if strings.Contains(respStr, "ter:TooManyUsers") {
		return fmt.Errorf("maximum number of users reached")
	}
	if strings.Contains(respStr, "ter:FixedUser") {
		return fmt.Errorf("cannot modify or delete fixed user")
	}
	if strings.Contains(respStr, "ter:Password") {
		return fmt.Errorf("password does not meet requirements")
	}
	if strings.Contains(respStr, "NotAuthorized") || strings.Contains(respStr, "ter:NotAuthorized") {
		return fmt.Errorf("not authorized")
	}

	// Try to extract fault reason
	if start := strings.Index(respStr, "<env:Reason>"); start != -1 {
		if end := strings.Index(respStr[start:], "</env:Reason>"); end != -1 {
			reason := respStr[start : start+end]
			if textStart := strings.Index(reason, "<env:Text"); textStart != -1 {
				if textEnd := strings.Index(reason[textStart:], ">"); textEnd != -1 {
					textStart = textStart + textEnd + 1
					if textEndTag := strings.Index(reason[textStart:], "</env:Text>"); textEndTag != -1 {
						return fmt.Errorf("SOAP fault: %s", reason[textStart:textStart+textEndTag])
					}
				}
			}
		}
	}

	// Try alternative fault structure
	if start := strings.Index(respStr, "<faultstring>"); start != -1 {
		start += len("<faultstring>")
		if end := strings.Index(respStr[start:], "</faultstring>"); end != -1 {
			return fmt.Errorf("SOAP fault: %s", respStr[start:start+end])
		}
	}

	return fmt.Errorf("SOAP fault in response")
}
