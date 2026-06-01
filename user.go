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

	// Fallback to manual extraction (namespace-agnostic)
	respStr := string(resp)
	for {
		userStart := findTagOpen(respStr, "User")
		if userStart == -1 {
			break
		}

		userEnd := findTagClose(respStr[userStart:], "User")
		if userEnd == -1 {
			break
		}

		userXML := respStr[userStart : userStart+userEnd]
		var user User

		user.Username = extractBetweenTags(userXML, "Username")
		user.UserLevel = UserLevel(extractBetweenTags(userXML, "UserLevel"))

		if user.Username != "" {
			users = append(users, user)
		}

		respStr = respStr[userStart+userEnd:]
	}

	// If the response contains Username elements but we extracted nothing,
	// parsing failed rather than the camera having no users
	if len(users) == 0 && strings.Contains(string(resp), "Username>") {
		return nil, fmt.Errorf("failed to parse GetUsers response")
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

// containsSOAPFault checks if the response contains a SOAP fault element
// with any namespace prefix (e.g. s:Fault, SOAP-ENV:Fault, env:Fault, soap:Fault)
func containsSOAPFault(resp string) bool {
	// Check for Fault element with any namespace prefix or no prefix
	if strings.Contains(resp, ":Fault>") || strings.Contains(resp, ":Fault ") {
		return true
	}
	if strings.Contains(resp, "<Fault>") || strings.Contains(resp, "<Fault ") {
		return true
	}
	return false
}

// parseSOAPFault checks for SOAP faults and returns a descriptive error
func parseSOAPFault(resp []byte) error {
	respStr := string(resp)

	if !containsSOAPFault(respStr) {
		return nil
	}

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

	// Try to extract fault reason text (with any namespace prefix)
	// Look for Reason > Text pattern used in SOAP 1.2
	if reason := extractBetweenTags(respStr, "Reason"); reason != "" {
		if text := extractTextElement(reason); text != "" {
			return fmt.Errorf("SOAP fault: %s", text)
		}
	}

	// Try SOAP 1.1 faultstring
	if start := strings.Index(respStr, "<faultstring>"); start != -1 {
		start += len("<faultstring>")
		if end := strings.Index(respStr[start:], "</faultstring>"); end != -1 {
			return fmt.Errorf("SOAP fault: %s", respStr[start:start+end])
		}
	}

	return fmt.Errorf("SOAP fault in response")
}

// extractBetweenTags finds content between opening and closing tags with any namespace prefix
func extractBetweenTags(s, localName string) string {
	// Find the opening tag, matching any namespace prefix and ignoring
	// attributes: <localName>, <localName ...>, <prefix:localName>,
	// <prefix:localName ...>. Scan '<' boundaries and compare the de-prefixed
	// tag name, skipping closing tags, declarations and comments.
	contentStart := -1
	for i := 0; i+1 < len(s); i++ {
		if s[i] != '<' || s[i+1] == '/' || s[i+1] == '?' || s[i+1] == '!' {
			continue
		}
		gt := strings.Index(s[i:], ">")
		if gt == -1 {
			return ""
		}
		if matchLocalName(s[i+1:i+gt], localName) {
			contentStart = i + gt + 1
			break
		}
	}
	if contentStart == -1 {
		return ""
	}

	// Find the matching closing tag and return the content before it. We must
	// stop at the '<' that opens the closing tag, not at the ":localName>"
	// inside it — otherwise a namespaced close tag like </tt:XAddr> leaves a
	// trailing "</tt" on the extracted value.
	content := s[contentStart:]
	for i := 0; i+1 < len(content); i++ {
		if content[i] != '<' || content[i+1] != '/' {
			continue
		}
		gt := strings.Index(content[i:], ">")
		if gt == -1 {
			return ""
		}
		if matchLocalName(content[i+2:i+gt], localName) {
			return content[:i]
		}
	}
	return ""
}

// matchLocalName reports whether an XML tag body (the text between '<'/'</' and
// '>', e.g. `tt:Username Type="x"` or `XAddr`) names the given local element,
// ignoring any namespace prefix, attributes and a self-closing slash.
func matchLocalName(tag, localName string) bool {
	if sp := strings.IndexByte(tag, ' '); sp != -1 {
		tag = tag[:sp]
	}
	tag = strings.TrimSuffix(tag, "/")
	if c := strings.LastIndex(tag, ":"); c != -1 {
		tag = tag[c+1:]
	}
	return tag == localName
}

// findTagOpen finds the start of an opening XML tag with any namespace prefix
func findTagOpen(s, localName string) int {
	// Try with namespace prefix: <prefix:Name> or <prefix:Name ...>
	marker := ":" + localName + ">"
	if idx := strings.Index(s, marker); idx != -1 {
		for i := idx - 1; i >= 0 && i > idx-20; i-- {
			if s[i] == '<' && (i+1 >= len(s) || s[i+1] != '/') {
				return i
			}
		}
	}
	marker = ":" + localName + " "
	if idx := strings.Index(s, marker); idx != -1 {
		for i := idx - 1; i >= 0 && i > idx-20; i-- {
			if s[i] == '<' && (i+1 >= len(s) || s[i+1] != '/') {
				return i
			}
		}
	}
	// Try without prefix
	for _, pattern := range []string{"<" + localName + ">", "<" + localName + " "} {
		if idx := strings.Index(s, pattern); idx != -1 {
			return idx
		}
	}
	return -1
}

// findTagClose finds the end of a closing XML tag with any namespace prefix,
// returning offset from the start of the search string
func findTagClose(s, localName string) int {
	// Try with namespace prefix: </prefix:Name>
	marker := ":" + localName + ">"
	for i := 0; i < len(s); {
		idx := strings.Index(s[i:], marker)
		if idx == -1 {
			break
		}
		pos := i + idx
		// Walk back to find '</'
		for j := pos - 1; j >= 0 && j > pos-20; j-- {
			if s[j] == '<' && j+1 < len(s) && s[j+1] == '/' {
				return pos + len(marker)
			}
		}
		i = pos + len(marker)
	}
	// Try without prefix
	closeTag := "</" + localName + ">"
	if idx := strings.Index(s, closeTag); idx != -1 {
		return idx + len(closeTag)
	}
	return -1
}

// extractTextElement extracts text content from a Text element (SOAP 1.2 fault reason)
func extractTextElement(s string) string {
	// <Text>...</Text> or <prefix:Text xml:lang="en">...</prefix:Text>.
	// extractBetweenTags handles namespace prefixes and attributes.
	return extractBetweenTags(s, "Text")
}
