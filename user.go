package onvif

import (
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

	var usersResp struct {
		Users []struct {
			Username  string `xml:"Username"`
			UserLevel string `xml:"UserLevel"`
		} `xml:"Body>GetUsersResponse>User"`
	}
	if err := xml.Unmarshal(resp, &usersResp); err != nil {
		return nil, fmt.Errorf("failed to parse GetUsers response: %v", err)
	}

	var users []User
	for _, u := range usersResp.Users {
		users = append(users, User{
			Username:  u.Username,
			UserLevel: UserLevel(u.UserLevel),
		})
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

// soapFault is the structured form of a SOAP 1.1/1.2 fault. Element names are
// matched by local name, so any namespace prefix (s:, env:, SOAP-ENV:, …) works.
type soapFault struct {
	// SOAP 1.2
	Code struct {
		Value   string `xml:"Value"`
		Subcode struct {
			Value string `xml:"Value"`
		} `xml:"Subcode"`
	} `xml:"Code"`
	Reason struct {
		Text string `xml:"Text"`
	} `xml:"Reason"`
	// SOAP 1.1
	FaultCode   string `xml:"faultcode"`
	FaultString string `xml:"faultstring"`
}

// parseSOAPFault returns a descriptive error if resp contains a SOAP fault,
// otherwise nil. It parses the fault structurally rather than scanning strings.
func parseSOAPFault(resp []byte) error {
	var env struct {
		Fault soapFault `xml:"Body>Fault"`
	}
	if err := xml.Unmarshal(resp, &env); err != nil {
		return nil // not parseable as a fault envelope
	}

	f := env.Fault
	subcode := strings.TrimSpace(f.Code.Subcode.Value)
	reason := strings.TrimSpace(f.Reason.Text)
	if reason == "" {
		reason = strings.TrimSpace(f.FaultString)
	}
	code := subcode
	if code == "" {
		code = strings.TrimSpace(f.Code.Value)
	}
	if code == "" {
		code = strings.TrimSpace(f.FaultCode)
	}
	if code == "" && reason == "" {
		return nil // no fault present
	}

	// Friendly messages for common ONVIF fault subcodes.
	switch {
	case strings.Contains(subcode, "UsernameClash"):
		return fmt.Errorf("username already exists")
	case strings.Contains(subcode, "UsernameMissing"):
		return fmt.Errorf("username not found")
	case strings.Contains(subcode, "TooManyUsers"):
		return fmt.Errorf("maximum number of users reached")
	case strings.Contains(subcode, "FixedUser"):
		return fmt.Errorf("cannot modify or delete fixed user")
	case strings.Contains(subcode, "Password"):
		return fmt.Errorf("password does not meet requirements")
	case strings.Contains(subcode, "NotAuthorized"):
		return fmt.Errorf("not authorized")
	}

	switch {
	case code != "" && reason != "":
		return fmt.Errorf("%s: %s", code, reason)
	case reason != "":
		return fmt.Errorf("%s", reason)
	default:
		return fmt.Errorf("SOAP fault: %s", code)
	}
}
