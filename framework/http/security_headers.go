package http

import (
	netHTTP "net/http"
)

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
		header.Set("Content-Security-Policy", config.ContentSecurityPolicy)
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
