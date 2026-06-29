// Package http (Navigator, Bridge, Tempose, and Glide) houses the core HTTP request-response lifecycle management.
// It abstracts raw net/http primitives into a high-level developer API.
package http

import (
	"encoding/json"
	"net/http"
)

// Context acts as the unified "Gateway" for every single HTTP request passing through the engine.
// It encapsulates the raw underlying connection streams (ResponseWriter, Request) and pairs them
// with a direct reference to the Tempose view rendering engine.
//
// Architectural Design Choice:
// By storing a direct pointer to *Tempose rather than referencing the global foundation.Container,
// we decouple the generic HTTP layer from the high-level application orchestrator. This eliminates 
// recursive dependency loops (Import Cycles) and keeps the framework's packages strictly ordered.
type Context struct {
	// Writer is the standard library stream manager used to flush headers and payloads back to the client.
	Writer http.ResponseWriter

	// Request represents the incoming HTTP data payload, containing headers, cookies, URLs, and forms.
	Request *http.Request

	// Tempose points directly to the companion view engine, enabling isolation during server-side composition.
	Tempose *Tempose

	// values stores request-scoped values passed between middlewares and handlers.
	values map[string]any
}

// Set stores a key-value pair in the request-scoped context.
func (c *Context) Set(key string, val any) {
	if c.values == nil {
		c.values = make(map[string]any)
	}
	c.values[key] = val
}

// Get retrieves a key-value pair from the request-scoped context.
func (c *Context) Get(key string) any {
	if c.values == nil {
		return nil
	}
	return c.values[key]
}

// Render facilitates the server-side composition of HTML view templates.
//
// Architectural Note:
// Because the Tempose engine is intentionally built as a "pure" compiler (relying strictly on io.Writer),
// the HTTP Context acts as the protocol bridge here. It explicitly commits the HTTP 200 OK status header
// into the response wire right before handing the stream over to the compiler pass.
//
// Parameters:
//   - viewName: The logical identifier or path of the template file to execute (e.g., "home").
//   - data: Any arbitrary structure, struct, or map to bind dynamically inside the template bindings.
//
// Returns:
//   - An error if template reading, compilation, or streaming fails.
func (c *Context) Render(viewName string, data any) error {
	c.Writer.WriteHeader(http.StatusOK)
	return c.Tempose.Render(c.Writer, viewName, data)
}

// JSON standardizes the API response delivery pipeline.
// It forcefully alters the outgoing header metadata to application/json, flushes the requested 
// HTTP status code, and serializes the target data structure directly into the raw network stream.
//
// Parameters:
//   - code: The RFC-compliant HTTP status integer to commit (e.g., 200, 201, 400).
//   - data: The Go interface layout (struct, map, slice) to stringify into valid JSON syntax.
//
// Returns:
//   - An error if the json encoder encounters unsupported types or writing blocks.
func (c *Context) JSON(code int, data any) error {
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(code)
	return json.NewEncoder(c.Writer).Encode(data)
}

// Query abstracts URL query string parameter resolution into a clean, predictable API.
// It fetches parameters appended directly onto the request URI string (e.g., /search?term=value).
//
// Parameters:
//   - key: The query parameter name string to extract.
//
// Returns:
//   - The string value matching the key. If the key does not exist, it returns an empty string "".
func (c *Context) Query(key string) string {
	return c.Request.URL.Query().Get(key)
}

// Post abstracts POST body payload and multipart form data evaluation.
// It reads input fields submitted by form elements or client payloads seamlessly.
//
// Parameters:
//   - key: The name identifier of the form input field to extract.
//
// Returns:
//   - The string data corresponding to the key, returning an empty string if omitted.
func (c *Context) Post(key string) string {
	return c.Request.PostFormValue(key)
}

// Locale extracts the client language preference, checking the URL query parameter
// and falling back to the standard HTTP Accept-Language header.
func (c *Context) Locale() string {
	if lang := c.Query("lang"); lang != "" {
		return lang
	}
	if accept := c.Request.Header.Get("Accept-Language"); accept != "" {
		if len(accept) >= 2 {
			return accept[:2]
		}
	}
	return "en"
}

// Trans translates a message key using the registered translator interface
// stored in the request context, interpolating variables.
func (c *Context) Trans(key string, replace ...map[string]string) string {
	transVal := c.Get("translator")
	if transVal == nil {
		return key
	}
	type translatorInterface interface {
		Trans(locale string, key string, replace map[string]string) string
	}
	t, ok := transVal.(translatorInterface)
	if !ok {
		return key
	}
	var repl map[string]string
	if len(replace) > 0 {
		repl = replace[0]
	}
	return t.Trans(c.Locale(), key, repl)
}