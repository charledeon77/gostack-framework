// Package http (Navigator, Bridge, Tempose, and Glide) houses the core HTTP request-response lifecycle management.
package http

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	netHTTP "net/http"
	"net/url"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
)

type multiBodyCloser struct {
	io.Reader
	closer io.Closer
}

func (m *multiBodyCloser) Close() error {
	if m.closer != nil {
		return m.closer.Close()
	}
	return nil
}

func maxRequestBodySize() int64 {
	maxBody := int64(8 * 1024 * 1024) // 8MB default
	if customSize := os.Getenv("MAX_REQUEST_BODY_SIZE"); customSize != "" {
		if val, err := strconv.ParseInt(customSize, 10, 64); err == nil && val > 0 {
			maxBody = val
		}
	}
	return maxBody
}

type headResponseWriter struct {
	netHTTP.ResponseWriter
}

func (h *headResponseWriter) Write(b []byte) (int, error) {
	return len(b), nil
}

// Engine represents the operational HTTP server configuration block.
// It manages the template view renderer, routing tables, and server lifecycle options.
type Engine struct {
	Router                 *Router
	Tempose                *Tempose
	SecurityHeadersConfig  SecurityHeadersConfig
	DisableSecurityHeaders bool
}

// NewEngine establishes an operational HTTP processing core with default security headers mounted (SEC-12).
//
// Parameters:
//   - router: An initialized routing context registry.
//   - tempose: A configured template view engine instance.
func NewEngine(router *Router, tempose *Tempose) *Engine {
	return &Engine{
		Router:                router,
		Tempose:               tempose,
		SecurityHeadersConfig: DefaultSecurityHeadersConfig(),
	}
}

// ServeHTTP acts as the low-level execution entry point required by Go's standard http.Server interface.
func (e *Engine) ServeHTTP(w netHTTP.ResponseWriter, r *netHTTP.Request) {
	// D44: Generate per-request CSP nonce at the top of ServeHTTP before security headers
	// and panic recovery so both normal responses and the 500 panic page share a valid nonce.
	cspNonce, err := GenerateCSPNonce()
	if err != nil {
		log.Printf("[GoStack Security] Failed to generate CSP nonce: %v", err)
	}
	if cspNonce != "" {
		r = r.WithContext(context.WithValue(r.Context(), cspNonceContextKey{}, cspNonce))
	}

	// Global Crash-Catcher & Panic Recovery Shield (SEC-05)
	defer func() {
		if rec := recover(); rec != nil {
			stack := debug.Stack()
			log.Printf("[GoStack Critical Panic Recovered] %v\nStack Trace:\n%s", rec, stack)

			// Never drop the TCP socket ungracefully — return a standardized 500 response
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_ = writeResponse(w, netHTTP.StatusInternalServerError, []byte(fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><title>500 Internal Server Error</title><style nonce="%s">body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;background:#0b0f19;color:#f8fafc;padding:40px;display:flex;justify-content:center;align-items:center;height:80vh;} .card{background:#1e293b;padding:30px;border-radius:12px;border:1px solid #334155;max-width:600px;text-align:center;} h1{color:#ef4444;margin-top:0;} p{color:#94a3b8;line-height:1.6;}</style></head>
<body><div class="card"><h1>500 Internal Server Error</h1><p>An unexpected condition was encountered while processing this request. The application kernel intercepted the crash safely.</p></div></body></html>`, cspNonce)))
		}
	}()

	// Automatic production security headers at engine level (SEC-12 / D44)
	if !e.DisableSecurityHeaders {
		ApplySecurityHeadersWithNonce(w, r, e.SecurityHeadersConfig, cspNonce)
	}

	// Enforce request body size limit (SEC-09 / D46)
	maxBody := maxRequestBodySize()
	// D46: Reject oversized requests immediately by Content-Length before reading a single byte.
	if r.ContentLength > maxBody {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_ = writeResponse(w, netHTTP.StatusRequestEntityTooLarge, []byte("413 Request Entity Too Large: request body exceeds configured limit"))
		return
	}
	if r.Body != nil {
		r.Body = netHTTP.MaxBytesReader(w, r.Body, maxBody)
	}

	writeBodyOrFormError := func(err error, malformedMsg string) {
		var maxBytesErr *netHTTP.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_ = writeResponse(w, netHTTP.StatusRequestEntityTooLarge, []byte("413 Request Entity Too Large: request body exceeds configured limit"))
			return
		}
		_ = writeResponse(w, netHTTP.StatusBadRequest, []byte(malformedMsg))
	}

	dispatch := func(ctx *Context, fn func(*Context) error) {
		sc := &statusCapture{ResponseWriter: ctx.Writer, code: netHTTP.StatusOK}
		ctx.Writer = sc
		if err := fn(ctx); err != nil {
			var maxBytesErr *netHTTP.MaxBytesError
			if errors.As(err, &maxBytesErr) {
				sc.ResponseWriter.Header().Set("Content-Type", "text/plain; charset=utf-8")
				_ = writeResponse(sc.ResponseWriter, netHTTP.StatusRequestEntityTooLarge, []byte("413 Request Entity Too Large: request body exceeds configured limit"))
				return
			}
			if e.Router != nil && e.Router.errorHandler != nil {
				e.Router.errorHandler(ctx, err)
				return
			}
			_ = writeResponse(sc.ResponseWriter, netHTTP.StatusInternalServerError, []byte("500 Internal Server Error - GoStack Engine"))
			return
		}
		// D46: If a handler called Context.Post (which cannot return an error directly) and a
		// form/multipart parse error occurred without the handler writing a response, surface 413/400.
		if !sc.Written() && ctx.FormErr() != nil {
			writeBodyOrFormError(ctx.FormErr(), "400 Bad Request: Malformed form body")
		}
	}

	effectiveMethod := r.Method
	effectiveWriter := w

	// D46 / C26: Validate multipart Content-Type header (media type + boundary) without buffering,
	// and validate application/x-www-form-urlencoded bodies for ALL mutating methods (POST/PUT/PATCH/DELETE).
	var multipartBoundary string
	if r.Method == netHTTP.MethodPost || r.Method == netHTTP.MethodPut || r.Method == netHTTP.MethodPatch || r.Method == netHTTP.MethodDelete {
		ct := r.Header.Get("Content-Type")
		lowerCT := strings.ToLower(ct)
		if strings.Contains(lowerCT, "multipart/form-data") {
			mediaType, params, parseErr := mime.ParseMediaType(ct)
			if parseErr != nil || mediaType != "multipart/form-data" || params["boundary"] == "" {
				_ = writeResponse(w, netHTTP.StatusBadRequest, []byte("400 Bad Request: Malformed multipart Content-Type header"))
				return
			}
			multipartBoundary = params["boundary"]
		} else if strings.Contains(lowerCT, "application/x-www-form-urlencoded") {
			if r.Method == netHTTP.MethodDelete {
				// Go's net/http Request.ParseForm() skips reading the body on DELETE;
				// read and validate it explicitly (bounded by MaxBytesReader) and restore r.Body.
				if r.Body != nil {
					bodyBytes, readErr := io.ReadAll(r.Body)
					if readErr != nil {
						writeBodyOrFormError(readErr, "400 Bad Request: Malformed form body")
						return
					}
					vals, parseErr := url.ParseQuery(string(bodyBytes))
					if parseErr != nil {
						_ = writeResponse(w, netHTTP.StatusBadRequest, []byte("400 Bad Request: Malformed form body"))
						return
					}
					r.PostForm = vals
					if r.Form == nil {
						r.Form = make(url.Values)
					}
					// D46: Mirror net/http precedence — body values first, query values appended after.
					for k, vs := range vals {
						r.Form[k] = append(r.Form[k], vs...)
					}
					for k, vs := range r.URL.Query() {
						r.Form[k] = append(r.Form[k], vs...)
					}
					r.Body = &multiBodyCloser{
						Reader: bytes.NewReader(bodyBytes),
						closer: r.Body,
					}
				}
			} else if err := r.ParseForm(); err != nil {
				writeBodyOrFormError(err, "400 Bad Request: Malformed form body")
				return
			}
		}
	}

	// HTTP Method Spoofing (PAR-10 / D46): Allow POST forms with _method=PUT/PATCH/DELETE
	// or X-HTTP-Method-Override header to match the overridden HTTP method.
	if r.Method == netHTTP.MethodPost {
		var override string
		if header := r.Header.Get("X-HTTP-Method-Override"); header != "" {
			override = header
		} else if query := r.URL.Query().Get("_method"); query != "" {
			override = query
		} else if r.PostForm != nil {
			override = r.PostFormValue("_method")
		} else if multipartBoundary != "" && r.Body != nil {
			// D46: Peek only the first multipart part (1 KB cap) for _method, then rebuild r.Body
			// with io.MultiReader. Multipart _method must be the first form field (otherwise use
			// ?_method= or X-HTTP-Method-Override). Never call part.Close() on an unconsumed part,
			// because mime/multipart.Part.Close drains the entire part via io.Copy(io.Discard, p).
			var peekBuf bytes.Buffer
			origBody := r.Body
			mr := multipart.NewReader(io.TeeReader(origBody, &peekBuf), multipartBoundary)
			if part, partErr := mr.NextPart(); partErr == nil {
				if part.FormName() == "_method" {
					// D46: Read with a 1025-byte limit and NEVER call part.Close(), because
					// mime/multipart.Part.Close drains the entire remaining part via io.Copy(io.Discard, p).
					valBytes, readErr := io.ReadAll(io.LimitReader(part, 1025))
					if readErr != nil {
						var maxBytesErr *netHTTP.MaxBytesError
						if errors.As(readErr, &maxBytesErr) {
							_ = writeResponse(w, netHTTP.StatusRequestEntityTooLarge, []byte("413 Request Entity Too Large: request body exceeds configured limit"))
							return
						}
						_ = writeResponse(w, netHTTP.StatusBadRequest, []byte("400 Bad Request: Malformed _method field"))
						return
					}
					if len(valBytes) > 1024 {
						_ = writeResponse(w, netHTTP.StatusBadRequest, []byte("400 Bad Request: _method field exceeds maximum length"))
						return
					}
					override = strings.TrimSpace(string(valBytes))
				}
				// Never call part.Close() on an unconsumed part either.
			} else {
				var maxBytesErr *netHTTP.MaxBytesError
				if errors.As(partErr, &maxBytesErr) {
					_ = writeResponse(w, netHTTP.StatusRequestEntityTooLarge, []byte("413 Request Entity Too Large: request body exceeds configured limit"))
					return
				}
			}
			r.Body = &multiBodyCloser{
				Reader: io.MultiReader(bytes.NewReader(peekBuf.Bytes()), origBody),
				closer: origBody,
			}
		}

		if override != "" {
			upper := strings.ToUpper(override)
			if upper == netHTTP.MethodPut || upper == netHTTP.MethodPatch || upper == netHTTP.MethodDelete {
				effectiveMethod = upper
			}
		}
	}

	// Support HEAD method: route to GET if no explicit HEAD handler, discard response body (SEC-05)
	if r.Method == "HEAD" {
		if _, hasHead := e.Router.GetRoutes()["HEAD "+r.URL.Path]; !hasHead {
			if route, _ := e.Router.Match("HEAD", r.URL.Path); route == nil {
				effectiveMethod = "GET"
				effectiveWriter = &headResponseWriter{ResponseWriter: w}
			}
		}
	}

	routes := e.Router.GetRoutes()
	key := effectiveMethod + " " + r.URL.Path

	if handler, exists := routes[key]; exists {
		ctx := &Context{
			Writer:  effectiveWriter,
			Request: r,
			Tempose: e.Tempose,
			Router:  e.Router,
		}
		dispatch(ctx, handler)
		return
	}

	// Try dynamic/parameterized routing match
	if route, params := e.Router.Match(effectiveMethod, r.URL.Path); route != nil {
		ctx := &Context{
			Writer:  effectiveWriter,
			Request: r,
			Tempose: e.Tempose,
			Router:  e.Router,
		}
		ctx.Set("params", params)
		dispatch(ctx, route.Handler)
		return
	}

	// Collect all allowed methods for this path (static and dynamic) (SEC-05)
	allowed := e.Router.AllowedMethods(r.URL.Path)

	// Automatic OPTIONS handling (SEC-05)
	if r.Method == "OPTIONS" {
		if len(allowed) > 0 {
			hasOptions := false
			for _, m := range allowed {
				if m == "OPTIONS" {
					hasOptions = true
					break
				}
			}
			if !hasOptions {
				allowed = append(allowed, "OPTIONS")
			}
			w.Header().Set("Allow", strings.Join(allowed, ", "))
			w.WriteHeader(netHTTP.StatusNoContent)
			return
		}
	}

	// 405 Method Not Allowed with Allow header (SEC-05)
	if len(allowed) > 0 {
		w.Header().Set("Allow", strings.Join(allowed, ", "))
		if e.Router.methodNotAllowedHandler != nil {
			ctx := &Context{
				Writer:  w,
				Request: r,
				Tempose: e.Tempose,
				Router:  e.Router,
			}
			e.Router.methodNotAllowedHandler(ctx)
		} else {
			_ = writeResponse(w, netHTTP.StatusMethodNotAllowed, []byte("405 Method Not Allowed - GoStack Engine"))
		}
		return
	}

	// 404 Not Found
	if e.Router.notFoundHandler != nil {
		ctx := &Context{
			Writer:  w,
			Request: r,
			Tempose: e.Tempose,
			Router:  e.Router,
		}
		e.Router.notFoundHandler(ctx)
	} else {
		_ = writeResponse(w, netHTTP.StatusNotFound, []byte("404 Page Not Found - GoStack Engine"))
	}
}

// writeResponse safely writes status code and body, logging errors on broken connections (C25).
func writeResponse(w netHTTP.ResponseWriter, statusCode int, body []byte) error {
	w.WriteHeader(statusCode)
	if len(body) > 0 {
		if _, err := w.Write(body); err != nil {
			log.Printf("[GoStack HTTP] Response write error (status %d): %v", statusCode, err)
			return err
		}
	}
	return nil
}