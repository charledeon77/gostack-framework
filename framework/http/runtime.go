// Package http (Navigator, Bridge, Tempose, and Glide) houses the core HTTP request-response lifecycle management.
package http

import (
	"net/http"
)

// Engine represents the operational HTTP server configuration block.
// It manages the template view renderer, routing tables, and server lifecycle options.
type Engine struct {
	Router  *Router
	Tempose *Tempose
}

// NewEngine establishes an operational HTTP processing core.
//
// Parameters:
//   - router: An initialized routing context registry.
//   - tempose: A configured template view engine instance.
func NewEngine(router *Router, tempose *Tempose) *Engine {
	return &Engine{
		Router:  router,
		Tempose: tempose,
	}
}

// ServeHTTP acts as the low-level execution entry point required by Go's standard http.Server interface.
// Every single inbound network connection request shifts through this method pass.
//
// How It Works:
//  1. It monitors incoming connection request patterns against the internal Router table.
//  2. If a match is found, it instantiates the framework's custom Context block.
//  3. It populates that Context with the raw response stream, the request metadata, and the Engine's direct Tempose reference.
//  4. It dispatches execution to the controller handler seamlessly.
//  5. If no route matches, it writes a clean RFC-standard 404 Not Found error state.
func (e *Engine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	routes := e.Router.GetRoutes()

	// Dispatch key: "METHOD /path" — enables distinct GET/POST/PUT/DELETE handlers
	// on the same path without conflict.
	key := r.Method + " " + r.URL.Path

	if handler, exists := routes[key]; exists {
		ctx := &Context{
			Writer:  w,
			Request: r,
			Tempose: e.Tempose,
		}
		handler(ctx)
		return
	}

	// Return 405 if the path exists under a different method, 404 otherwise.
	for k := range routes {
		if len(k) > len(r.URL.Path) && k[len(k)-len(r.URL.Path):] == r.URL.Path {
			w.WriteHeader(http.StatusMethodNotAllowed)
			_, _ = w.Write([]byte("405 Method Not Allowed - GoStack Engine"))
			return
		}
	}

	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte("404 Page Not Found - GoStack Engine"))
}

// GoStackRuntimeJS holds the core client-side reactivity engine text.
// This is maintained as a raw string literal to compile directly into 
// the text section of the executable, bypassing runtime disk lookups.
const GoStackRuntimeJS = `(function () {
    class GoStackRuntime {
        constructor() {
            this.stores = new Map();
        }

        init() {
            const components = document.querySelectorAll('[gs-state]');
            components.forEach((el, index) => {
                const componentId = el.id || 'gs-cmp-' + index;
                if (!el.id) el.id = componentId;
                this.hydrateComponent(el, componentId);
            });
        }

        hydrateComponent(element, id) {
            const rawStateAttr = element.getAttribute('gs-state');
            let initialState = {};
            try {
                initialState = JSON.parse(rawStateAttr);
            } catch (e) {
                console.error('[Glide] Failed to parse gs-state:', e);
                return;
            }

            const self = this;
            const reactiveState = new Proxy(initialState, {
                set(target, property, value) {
                    if (target[property] === value) return true;
                    target[property] = value;
                    self.renderComponent(element, target);
                    return true;
                },
                get(target, property) {
                    return target[property];
                }
            });

            this.stores.set(id, reactiveState);
            this.bindEvents(element, reactiveState);
            this.renderComponent(element, reactiveState);
        }

        bindEvents(rootElement, state) {
            const models = rootElement.querySelectorAll('[gs-model]');
            models.forEach(inputEl => {
                const stateKey = inputEl.getAttribute('gs-model');
                if (state[stateKey] !== undefined) {
                    inputEl.value = state[stateKey];
                }
                inputEl.addEventListener('input', (e) => {
                    state[stateKey] = e.target.value;
                });
            });

            const clickables = rootElement.querySelectorAll('[gs-click], [gs-on\\:click]');
            clickables.forEach(clickableEl => {
                const actionExpression = clickableEl.getAttribute('gs-click') || clickableEl.getAttribute('gs-on:click');
                clickableEl.addEventListener('click', () => {
                    const runner = new Function('state', 'with(state) { ' + actionExpression + ' }');
                    try { 
                        runner(state); 
                    } catch (err) { 
                        console.error('[Glide] Execution failed:', err); 
                    }
                });
            });
        }

        renderComponent(rootElement, state) {
            const textNodes = rootElement.querySelectorAll('[gs-text]');
            textNodes.forEach(node => {
                const stateKey = node.getAttribute('gs-text');
                if (state[stateKey] !== undefined && node.textContent !== String(state[stateKey])) {
                    node.textContent = state[stateKey];
                }
            });

            const visibleNodes = rootElement.querySelectorAll('[gs-show]');
            visibleNodes.forEach(node => {
                const stateKey = node.getAttribute('gs-show');
                if (state[stateKey] !== undefined) {
                    node.style.display = !!state[stateKey] ? '' : 'none';
                }
            });
        }
    }

    document.addEventListener('DOMContentLoaded', () => {
        window.GoStack = new GoStackRuntime();
        window.GoStack.init();
    });
})();`