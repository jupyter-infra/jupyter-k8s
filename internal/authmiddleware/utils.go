package authmiddleware

// URL and group parsing helper functions

// splitGroups splits a comma-separated or space-separated list of groups
func splitGroups(groups string) []string {
	if groups == "" {
		return []string{}
	}

	var result []string
	var current []rune

	for i := 0; i < len(groups); i++ {
		char := rune(groups[i])

		// Handle quoted strings
		if char == '"' && (i == 0 || groups[i-1] == ',' || groups[i-1] == ' ') {
			// Beginning of a quoted section
			endQuotePos := -1
			for j := i + 1; j < len(groups); j++ {
				if groups[j] == '"' {
					endQuotePos = j
					break
				}
			}

			if endQuotePos != -1 {
				// Extract content between quotes (without the quotes)
				quoted := groups[i+1 : endQuotePos]
				result = append(result, quoted)

				// Move index past the end quote
				i = endQuotePos

				// Skip current token accumulation
				continue
			}
		}

		// Handle comma/space separator (not in quotes)
		if char == ',' || char == ' ' {
			if len(current) > 0 {
				result = append(result, string(current))
				current = nil
			}
			continue
		}

		// Accumulate current token
		current = append(current, char)
	}

	// Add any remaining content
	if len(current) > 0 {
		result = append(result, string(current))
	}

	return result
}

// containsSpecialChars checks if a string contains commas or spaces
func containsSpecialChars(s string) bool {
	for _, r := range s {
		if r == ',' || r == ' ' {
			return true
		}
	}
	return false
}

// JoinGroups joins a slice of groups into a comma-separated string
func JoinGroups(groups []string) string {
	result := ""
	for i, g := range groups {
		if i > 0 {
			result += ","
		}
		// If group contains commas or spaces, quote it
		if containsSpecialChars(g) {
			result += "\"" + g + "\""
		} else {
			result += g
		}
	}
	return result
}

// isValidRedirectURL checks if a redirect URL is valid
func isValidRedirectURL(url, host string) bool {
	// Accept relative URLs
	if len(url) > 0 && url[0] == '/' {
		return true
	}

	// For absolute URLs, check if the host matches
	return hasHost(url, host)
}

// hasHost checks if a URL has the given host
func hasHost(url, host string) bool {
	// Simple check - should be improved in production
	if len(url) < len(host) {
		return false
	}

	return url == host ||
		url == "https://"+host ||
		url == "http://"+host ||
		(len(url) > len(host)+8 && url[:len(host)+8] == "https://"+host) ||
		(len(url) > len(host)+7 && url[:len(host)+7] == "http://"+host)
}

// hasProtocol checks if a URL has a protocol
func hasProtocol(url string) bool {
	return len(url) > 7 && url[:7] == "http://" || len(url) > 8 && url[:8] == "https://"
}

// splitAndTrim splits a string by the given separator and trims spaces from each part
func splitAndTrim(s, sep string) []string {
	parts := make([]string, 0)
	for _, p := range splitString(s, sep) {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

// splitString splits a string by the given separator
func splitString(s, sep string) []string {
	var result []string
	parts := []rune(s)
	last := 0
	for i, c := range parts {
		if c == rune(sep[0]) {
			result = append(result, string(parts[last:i]))
			last = i + 1
		}
	}
	result = append(result, string(parts[last:]))
	return result
}
