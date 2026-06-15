package onvif

import (
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
)

var (
	ErrMissingToken     = errors.New("missing username token")
	ErrEmptyUsername    = errors.New("empty username")
	ErrPasswordMismatch = errors.New("password mismatch")
)

// Auth holds expected credentials for WS-UsernameToken validation.
type Auth struct {
	Username string
	Password string
}

// AuthResult carries the outcome of authentication.
type AuthResult struct {
	Username string
	OK       bool
}

// Validate checks UsernameToken credentials.
// It supports both PasswordText (direct comparison) and
// PasswordDigest: base64(SHA1(DecodeBase64(Nonce) + Created + Password)).
// Password type is detected by inspecting the Password element's XML attributes
// — since UsernameToken only stores the text content, we try digest first
// (if nonce is non-empty) and fall back to plaintext.
func (a *Auth) Validate(token *UsernameToken) error {
	if token == nil {
		return ErrMissingToken
	}
	if token.Username == "" {
		return ErrEmptyUsername
	}
	if subtle.ConstantTimeCompare([]byte(token.Username), []byte(a.Username)) != 1 {
		return fmt.Errorf("%w: username mismatch", ErrPasswordMismatch)
	}

	if token.Nonce != "" {
		// Digest mode: compare computed digest with provided password
		expected := CheckDigest(token.Nonce, token.Created, a.Password)
		if subtle.ConstantTimeCompare([]byte(token.Password), []byte(expected)) != 1 {
			return ErrPasswordMismatch
		}
		return nil
	}

	// PasswordText mode: direct comparison
	if subtle.ConstantTimeCompare([]byte(token.Password), []byte(a.Password)) != 1 {
		return ErrPasswordMismatch
	}
	return nil
}

// CheckDigest computes the ONVIF WS-UsernameToken password digest:
// base64(SHA1(DecodeBase64(nonce) + created + password))
func CheckDigest(nonce, created, password string) string {
	nonceBytes, _ := base64.StdEncoding.DecodeString(nonce)
	h := sha1.New()
	h.Write(nonceBytes)
	h.Write([]byte(created))
	h.Write([]byte(password))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}
