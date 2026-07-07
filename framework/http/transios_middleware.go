package http

import (
	"github.com/charledeon77/gostack-framework/framework/contract"
	netHTTP "net/http"
)

// TransiosMiddleware intercepts the HTTP request lifecycle to load and configure
// the localization translator, dynamically resolved client locale preference,
// and request context bindings.
//
// Parameters:
//   - t: The global translator instance registered in the container.
//   - defaultLocale: The default fallback language locale if unresolved.
func TransiosMiddleware(t contract.Translator, defaultLocale string) Middleware {
	if defaultLocale == "" {
		defaultLocale = "en"
	}
	
	return func(ctx *Context, next NextHandler) error {
		// 1. Inject the translator singleton reference into context values
		ctx.Set("translator", t)

		// 2. Resolve client language preference
		activeLocale := ctx.Locale()
		if activeLocale == "" {
			activeLocale = defaultLocale
		}

		// 3. Keep the request-scoped locale state in context values
		ctx.Set("locale", activeLocale)

		// 4. Optionally write back a persistent cookie if the query parameter explicitly set the language
		if lang := ctx.Query("lang"); lang != "" {
			netHTTP.SetCookie(ctx.Writer, &netHTTP.Cookie{
				Name:     "locale",
				Value:    lang,
				Path:     "/",
				HttpOnly: true,
				Secure:   false, // local development default
			})
		}

		return next(ctx)
	}
}
