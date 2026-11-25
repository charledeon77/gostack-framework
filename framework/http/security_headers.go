package http

import (
	"crypto/rand"
	"encoding/base64"
	netHTTP "net/http"
	"strings"
)

// GenerateCSPNonce generates a cryptographically random 128-bit base64-encoded CSP nonce.
func GenerateCSPNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawStdEncoding.EncodeToString(b), nil
}

// injectCSPNonce adds 'nonce-<nonce>' to style-src in the CSP policy string.
// If style-src is not explicitly present, it appends "style-src 'self' 'nonce-<nonce>';".
func injectCSPNonce(csp, nonce string) string {
	if csp == "" || nonce == "" {
		return csp
	}
	token := "'nonce-" + nonce + "'"
	directives := strings.Split(csp, ";")
	foundStyle := false
	for i, d := range directives {
		trimmed := strings.TrimSpace(d)
		if strings.HasPrefix(trimmed, "style-src ") || trimmed == "style-src" {
			directives[i] = " " + trimmed + " " + token
			foundStyle = true
		}
	}
	if foundStyle {
		return strings.Join(directives, ";")
	}
	trimmedCSP := strings.TrimRight(strings.TrimSpace(csp), ";")
	return trimmedCSP + "; style-src 'self' " + token + ";"
}

// SecurityHeadersConfig defines customizable settings for HTTP response security headers (SEC-12).
type SecurityHeadersConfig struct {
	// XFrameOptions controls whether the site can be embedded in frames/iframes. Default: "DENY".
	XFrameOptions string
	// XContentTypeOptions prevents MIME-type sniffing. Default: "nosniff".
	XContentTypeOptions string
	// ReferrerPolicy controls the Referer header behavior. Default: "strict-origin-when-cross-origin".
	ReferrerPolicy string
	// StrictTransportSecurity enforces HTTPS (HSTS). Default: "max-age=31536000; includeSubDomains".
	// Emitted exclusively over secure HTTPS/TLS requests (SEC-12).
	StrictTransportSecurity string
	// ContentSecurityPolicy defines allowed content sources. Strict default: "default-src 'self'; img-src 'self' data:; font-src 'self'; frame-ancestors 'none';".
	ContentSecurityPolicy string
	// PermissionsPolicy restricts browser features and APIs. Default: "camera=(), microphone=(), geolocation=()".
	PermissionsPolicy string
	// Disable completely disables automatic security headers when explicitly set to true.
	Disable bool
}

// DefaultSecurityHeadersConfig returns standard, production-ready security header defaults matching OWASP recommendations.
func DefaultSecurityHeadersConfig() SecurityHeadersConfig {
	return SecurityHeadersConfig{
		XFrameOptions:           "DENY",
		XContentTypeOptions:     "nosniff",
		ReferrerPolicy:          "strict-origin-when-cross-origin",
		StrictTransportSecurity: "max-age=31536000; includeSubDomains",
		ContentSecurityPolicy:   "default-src 'self'; img-src 'self' data:; font-src 'self'; frame-ancestors 'none';",
		PermissionsPolicy:       "camera=(), microphone=(), geolocation=()",
		Disable:                 false,
	}
}

// ApplySecurityHeaders writes configured security headers to the ResponseWriter.
func ApplySecurityHeaders(w netHTTP.ResponseWriter, r *netHTTP.Request, config SecurityHeadersConfig) {
	ApplySecurityHeadersWithNonce(w, r, config, "")
}

// ApplySecurityHeadersWithNonce writes configured security headers to the ResponseWriter,
// injecting the per-request CSP nonce into style-src when non-empty (D44).
func ApplySecurityHeadersWithNonce(w netHTTP.ResponseWriter, r *netHTTP.Request, config SecurityHeadersConfig, nonce string) {
	if config.Disable {
		return
	}

	header := w.Header()

	if config.XFrameOptions != "" {
		header.Set("X-Frame-Options", config.XFrameOptions)
	}
	if config.XContentTypeOptions != "" {
		header.Set("X-Content-Type-Options", config.XContentTypeOptions)
	}
	if config.ReferrerPolicy != "" {
		header.Set("Referrer-Policy", config.ReferrerPolicy)
	}
	if config.ContentSecurityPolicy != "" {
		csp := injectCSPNonce(config.ContentSecurityPolicy, nonce)
		header.Set("Content-Security-Policy", csp)
	}
	if config.PermissionsPolicy != "" {
		header.Set("Permissions-Policy", config.PermissionsPolicy)
	}
	// HSTS: RFC 6797 dictates Strict-Transport-Security must only be served over HTTPS (SEC-12)
	if config.StrictTransportSecurity != "" && isRequestSecure(r) {
		header.Set("Strict-Transport-Security", config.StrictTransportSecurity)
	}
}

// SecurityHeaders returns an HTTP middleware that injects modern, battle-tested security headers
// matching the maturity standards of Laravel and Django.
func SecurityHeaders(cfg ...SecurityHeadersConfig) Middleware {
	config := DefaultSecurityHeadersConfig()
	if len(cfg) > 0 {
		config = cfg[0]
	}

	return func(ctx *Context, next NextHandler) error {
		ApplySecurityHeaders(ctx.Writer, ctx.Request, config)
		return next(ctx)
	}
}
