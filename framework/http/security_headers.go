package http

// SecurityHeadersConfig defines customizable settings for HTTP response security headers.
type SecurityHeadersConfig struct {
	// XFrameOptions controls whether the site can be embedded in frames/iframes. Default: "DENY".
	XFrameOptions string
	// XContentTypeOptions prevents MIME-type sniffing. Default: "nosniff".
	XContentTypeOptions string
	// XXSSProtection enables legacy browser XSS filters. Default: "1; mode=block".
	XXSSProtection string
	// ReferrerPolicy controls the Referer header behavior. Default: "strict-origin-when-cross-origin".
	ReferrerPolicy string
	// StrictTransportSecurity enforces HTTPS (HSTS). Default: "max-age=31536000; includeSubDomains".
	StrictTransportSecurity string
	// ContentSecurityPolicy defines allowed content sources. Default: "default-src 'self'".
	ContentSecurityPolicy string
	// PermissionsPolicy restricts browser features and APIs. Default: "camera=(), microphone=(), geolocation=()".
	PermissionsPolicy string
}

// DefaultSecurityHeadersConfig returns standard, production-ready security header defaults.
func DefaultSecurityHeadersConfig() SecurityHeadersConfig {
	return SecurityHeadersConfig{
		XFrameOptions:           "DENY",
		XContentTypeOptions:     "nosniff",
		XXSSProtection:          "1; mode=block",
		ReferrerPolicy:          "strict-origin-when-cross-origin",
		StrictTransportSecurity: "max-age=31536000; includeSubDomains",
		ContentSecurityPolicy:   "default-src 'self'; script-src 'self' 'unsafe-inline' https:; style-src 'self' 'unsafe-inline' https:; img-src 'self' data: https:; font-src 'self' https:;",
		PermissionsPolicy:       "camera=(), microphone=(), geolocation=()",
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
		header := ctx.Writer.Header()

		if config.XFrameOptions != "" {
			header.Set("X-Frame-Options", config.XFrameOptions)
		}
		if config.XContentTypeOptions != "" {
			header.Set("X-Content-Type-Options", config.XContentTypeOptions)
		}
		if config.XXSSProtection != "" {
			header.Set("X-XSS-Protection", config.XXSSProtection)
		}
		if config.ReferrerPolicy != "" {
			header.Set("Referrer-Policy", config.ReferrerPolicy)
		}
		if config.StrictTransportSecurity != "" {
			header.Set("Strict-Transport-Security", config.StrictTransportSecurity)
		}
		if config.ContentSecurityPolicy != "" {
			header.Set("Content-Security-Policy", config.ContentSecurityPolicy)
		}
		if config.PermissionsPolicy != "" {
			header.Set("Permissions-Policy", config.PermissionsPolicy)
		}

		return next(ctx)
	}
}
