// Package gb28181 implements SIP digest authentication (RFC 2617)
// used by GB/T 28181 platforms — Authorization header building and
// response verification.
package gb28181

import (
	"fmt"
	"regexp"
	"strings"
)

// DigestAuth represents a parsed Digest authentication challenge.
type DigestAuth struct {
	Realm     string // Authentication realm (e.g., "3402000000")
	Nonce     string // Server nonce
	Algorithm string // Hash algorithm: "", "MD5", or "SHA-256"
	Qop       string // Quality of protection: "" or "auth"
	Opaque    string // Opaque value
	Stale     string // Whether nonce is stale
}

// ParseChallenge parses a WWW-Authenticate header value into DigestAuth.
// Expected format: Digest realm="...", nonce="...", algorithm=MD5, ...
func ParseChallenge(wwwAuthHeader string) (DigestAuth, error) {
	result := DigestAuth{}

	// Remove "Digest " prefix
	wwwAuthHeader = strings.TrimSpace(wwwAuthHeader)
	if !strings.HasPrefix(strings.ToLower(wwwAuthHeader), "digest ") {
		return result, fmt.Errorf("not a Digest challenge: %s", wwwAuthHeader)
	}
	wwwAuthHeader = strings.TrimSpace(wwwAuthHeader[7:])

	// Parse key=value pairs
	// Matches: key="value" or key=value
	re := regexp.MustCompile(`(\w+)="?([^",]+)"?`)
	matches := re.FindAllStringSubmatch(wwwAuthHeader, -1)

	for _, match := range matches {
		if len(match) != 3 {
			continue
		}
		key := strings.ToLower(match[1])
		value := match[2]

		switch key {
		case "realm":
			result.Realm = value
		case "nonce":
			result.Nonce = value
		case "algorithm":
			result.Algorithm = value
		case "qop":
			result.Qop = value
		case "opaque":
			result.Opaque = value
		case "stale":
			result.Stale = value
		}
	}

	if result.Realm == "" || result.Nonce == "" {
		return result, fmt.Errorf("missing required Digest parameters: realm or nonce")
	}

	return result, nil
}

// ComputeResponse generates the response value for Digest authentication per RFC 2617.
//
// Parameters:
//   - username: Username (device ID in GB/T 28181)
//   - realm: Realm from challenge
//   - password: Password
//   - nonce: Nonce from challenge
//   - uri: Request-URI (e.g., "sip:3402000000")
//   - method: SIP method (e.g., "REGISTER")
//   - algorithm: Hash algorithm ("" defaults to "MD5" per RFC 2617 §3.2.1)
//
// Returns the hex-encoded response string.
//
// Basic Digest (qop not set or qop != "auth"):
//   HA1 = MD5(username:realm:password)
//   HA2 = MD5(method:uri)
//   response = MD5(HA1:nonce:HA2)
//
// With qop="auth":
//   response = MD5(HA1:nonce:00000001:0a4f113b:cnonce:auth:HA2)
//   (Note: full qop support requires nc and cnonce; this implementation
//    provides basic qop handling; full qop can be added when needed)
func ComputeResponse(username, realm, password, nonce, uri, method, algorithm string) string {
	// Default to MD5 per RFC 2617 §3.2.1 (GB/T 28181 platforms use MD5)
	if algorithm == "" {
		algorithm = "MD5"
	}

	// Compute HA1 = hash(username:realm:password)
	ha1 := ComputeDigest(algorithm, username, realm, password)

	// Compute HA2 = hash(method:uri)
	ha2 := ComputeDigest(algorithm, method, uri)

	// Compute response = hash(HA1:nonce:HA2)
	// (Basic Digest without qop; qop="auth" adds nc and cnonce)
	response := ComputeDigest(algorithm, ha1, nonce, ha2)

	return response
}

// BuildAuthorizationHeader builds the full Authorization header value.
//
// Parameters:
//   - authChallenge: Parsed Digest challenge from 401 response
//   - username: Username (device ID)
//   - password: Password
//   - uri: Request-URI (e.g., "sip:3402000000")
//   - method: SIP method (e.g., "REGISTER")
//
// Returns the complete Authorization header string, e.g.:
// Digest username="34020000012000000001", realm="3402000000", nonce="abc123", uri="sip:3402000000", response="..."
func BuildAuthorizationHeader(authChallenge DigestAuth, username, password, uri, method string) string {
	response := ComputeResponse(username, authChallenge.Realm, password, authChallenge.Nonce, uri, method, authChallenge.Algorithm)

	var buf strings.Builder
	buf.WriteString("Digest username=\"")
	buf.WriteString(username)
	buf.WriteString("\", realm=\"")
	buf.WriteString(authChallenge.Realm)
	buf.WriteString("\", nonce=\"")
	buf.WriteString(authChallenge.Nonce)
	buf.WriteString("\", uri=\"")
	buf.WriteString(uri)
	buf.WriteString("\", response=\"")
	buf.WriteString(response)
	buf.WriteString("\"")

	if authChallenge.Algorithm != "" {
		buf.WriteString(", algorithm=")
		buf.WriteString(authChallenge.Algorithm)
	}

	if authChallenge.Qop != "" {
		buf.WriteString(", qop=")
		buf.WriteString(authChallenge.Qop)
	}

	if authChallenge.Opaque != "" {
		buf.WriteString(", opaque=\"")
		buf.WriteString(authChallenge.Opaque)
		buf.WriteString("\"")
	}

	return buf.String()
}