package portutils

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

// ProcessedPortInfo holds information about a parsed port.
type ProcessedPortInfo struct {
	OriginalHostPort      string // The host port string as parsed (can be empty)
	OriginalContainerPort string // The container port string as parsed
	Comment               string // Raw comment text, if any
	ProtocolStrippedPort  string // Container port number after stripping /tcp or /udp, used for consolidation key
}

var nonAlphanumericUnderscoreRegex = regexp.MustCompile(`[^a-z0-9_]+`)

// SanitizeCommentToLabel converts a comment string into a valid HCL label.
func SanitizeCommentToLabel(comment string) string {
	if comment == "" {
		return ""
	}
	lower := strings.ToLower(strings.TrimSpace(comment))
	// Replace spaces and hyphens with underscores first
	intermediate := strings.ReplaceAll(lower, " ", "_")
	intermediate = strings.ReplaceAll(intermediate, "-", "_")
	// Remove all other non-alphanumeric characters (keeps underscores)
	sanitized := nonAlphanumericUnderscoreRegex.ReplaceAllString(intermediate, "")
	// Remove leading/trailing underscores that might result
	sanitized = strings.Trim(sanitized, "_")
	// Prevent multiple underscores together if they form due to replacements
	if strings.Contains(sanitized, "__") { // Only compile if needed
		sanitized = regexp.MustCompile(`_+`).ReplaceAllString(sanitized, "_")
	}
	return sanitized
}

var wellKnownPorts = map[string]string{
	"80":   "http",
	"443":  "https",
	"21":   "ftp",
	"22":   "ssh",
	"23":   "telnet",
	"25":   "smtp",
	"53":   "dns",
	"110":  "pop3",
	"143":  "imap",
	"3306": "mysql",
	"5432": "postgresql",
}

// GetWellKnownPortLabel returns a common label for a port number string.
func GetWellKnownPortLabel(portStr string) string {
	// portStr is assumed to be just the number, already stripped of /tcp, /udp
	return wellKnownPorts[portStr]
}

// ParseInt64ForPort parses a string to an int64, typically for port numbers.
func ParseInt64ForPort(s string) (int64, error) {
	if s == "" {
		return 0, fmt.Errorf("port string is empty")
	}
	var i int64
	_, err := fmt.Sscan(strings.TrimSpace(s), &i)
	if err != nil {
		return 0, fmt.Errorf("could not parse port '%s' to int64: %w", s, err)
	}
	return i, nil
}

// CreateCommentTokens is a helper to create HCL comment tokens.
// Includes a newline after the comment line.
func CreateCommentTokens(commentText string) hclwrite.Tokens {
	return hclwrite.Tokens{
		{
			Type:  hclsyntax.TokenComment,
			Bytes: []byte("# " + commentText),
		},
		{
			Type:  hclsyntax.TokenNewline, // Newline after the comment text
			Bytes: []byte("\n"),
		},
	}
}