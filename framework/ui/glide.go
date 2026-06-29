/*
Purpose:
This file contains the Glide client-side reactive directive engine, embedded as a Go
string constant so the entire runtime ships inside the binary — zero HTTP round-trips
to load a JS file.

Philosophy:
Glide is GoStack's browser-side reactivity layer. Like Alpine.js, it works directly
on existing HTML via data attributes. Unlike Alpine, it is purpose-built for GoStack
components and carries zero third-party footprint.

Architecture:
- GlideJS is a self-contained IIFE (Immediately Invoked Function Expression).
- It finds all [gs-data] root elements, creates reactive Proxy scopes, then wires
  all directive attributes (gs-text, gs-click, gs-model, gs-if, gs-each, gs-class,
  gs-html, gs-submit, gs-change, gs-attr) to live DOM updates.
- Updates are batched through requestAnimationFrame to avoid thrashing.
- MutationObserver watches for server-rendered or lazily appended scopes.

Directives:
  gs-data="{ key: value }"   — declares a reactive state scope
  gs-text="expr"              — binds expression to element text content
  gs-html="expr"              — binds expression to element innerHTML
  gs-model="key"              — two-way binds <input>/<select>/<textarea> to state key
  gs-click="stmt"             — executes statement on click
  gs-submit="stmt"            — executes statement on form submit (prevents default)
  gs-change="stmt"            — executes statement on input change
  gs-if="expr"                — shows/hides element based on truthy expression
  gs-show="expr"              — toggles CSS visibility (keeps element in flow)
  gs-each="item in list"      — repeats element for each item in a state array
  gs-class="{ cls: expr }"    — toggles CSS classes based on an object expression
  gs-attr="{ attr: expr }"    — sets element attributes from an object expression
*/
package ui

// GlideJS is the Glide reactive directive engine runtime, injected into every
// GoStack page via WriteMasterAssetBlock. It is written as a pure JavaScript IIFE
// with zero external dependencies and works in any modern browser.
const GlideJS = `
(function(global) {
    'use strict';

    // ──────────────────────────────────────────────────────────────────────────
    // Glide — GoStack UI Reactive Directive Engine  v1.0
    // ──────────────────────────────────────────────────────────────────────────

    // ── Expression Helpers ────────────────────────────────────────────────────

    function evaluate(expr, scope) {
        try {
            var keys = Object.keys(scope);
            var vals = keys.map(function(k) { return scope[k]; });
            return (new Function(keys.join(','), '"use strict"; return (' + expr + ')'))
                .apply(null, vals);
        } catch(e) {
            return undefined;
        }
    }

    function execute(stmt, scope) {
        try {
            var keys = Object.keys(scope);
            var vals = keys.map(function(k) { return scope[k]; });
            (new Function(keys.join(','), '"use strict"; ' + stmt))
                .apply(null, vals);
        } catch(e) {
            // silent: expression errors should not crash the page
        }
    }

    function parseData(expr) {
        try {
            return (new Function('"use strict"; return (' + expr + ')'))();
        } catch(e) {
            return {};
        }
    }

    // ── Reactivity via Proxy ──────────────────────────────────────────────────

    function makeReactive(obj, callback) {
        return new Proxy(obj, {
            set: function(target, key, value) {
                target[key] = value;
                callback();
                return true;
            },
            get: function(target, key) {
                var val = target[key];
                if (val !== null && typeof val === 'object' && !Array.isArray(val)) {
                    return makeReactive(val, callback);
                }
                return val;
            }
        });
    }

    // ── Scope Boundary Check ──────────────────────────────────────────────────
    // Returns true only if el's nearest [gs-data] ancestor IS rootEl.
    // This prevents outer scopes from processing inner nested scopes.

    function isDirectChild(el, rootEl) {
        var parent = el.parentElement;
        while (parent && parent !== rootEl) {
            if (parent.hasAttribute('gs-data')) return false;
            parent = parent.parentElement;
        }
        return true;
    }

    // ── DOM Update Pass ───────────────────────────────────────────────────────

    function updateScope(rootEl, data, eachMeta) {
        // gs-if
        rootEl.querySelectorAll('[gs-if]').forEach(function(el) {
            if (!isDirectChild(el, rootEl)) return;
            var result = evaluate(el.getAttribute('gs-if'), data);
            el.style.display = result ? '' : 'none';
        });

        // gs-show
        rootEl.querySelectorAll('[gs-show]').forEach(function(el) {
            if (!isDirectChild(el, rootEl)) return;
            var result = evaluate(el.getAttribute('gs-show'), data);
            el.style.visibility = result ? '' : 'hidden';
        });

        // gs-text
        rootEl.querySelectorAll('[gs-text]').forEach(function(el) {
            if (!isDirectChild(el, rootEl)) return;
            var result = evaluate(el.getAttribute('gs-text'), data);
            if (result !== undefined && result !== null) el.textContent = String(result);
        });

        // gs-html
        rootEl.querySelectorAll('[gs-html]').forEach(function(el) {
            if (!isDirectChild(el, rootEl)) return;
            var result = evaluate(el.getAttribute('gs-html'), data);
            if (result !== undefined && result !== null) el.innerHTML = String(result);
        });

        // gs-class
        rootEl.querySelectorAll('[gs-class]').forEach(function(el) {
            if (!isDirectChild(el, rootEl)) return;
            var classMap = evaluate(el.getAttribute('gs-class'), data);
            if (classMap && typeof classMap === 'object') {
                Object.entries(classMap).forEach(function(entry) {
                    el.classList.toggle(entry[0], !!entry[1]);
                });
            }
        });

        // gs-attr
        rootEl.querySelectorAll('[gs-attr]').forEach(function(el) {
            if (!isDirectChild(el, rootEl)) return;
            var attrMap = evaluate(el.getAttribute('gs-attr'), data);
            if (attrMap && typeof attrMap === 'object') {
                Object.entries(attrMap).forEach(function(entry) {
                    el.setAttribute(entry[0], entry[1]);
                });
            }
        });

        // gs-model — sync current data value → input DOM value
        rootEl.querySelectorAll('[gs-model]').forEach(function(el) {
            if (!isDirectChild(el, rootEl)) return;
            var key = el.getAttribute('gs-model');
            var val = evaluate(key, data);
            if (val === undefined) return;
            if (el.type === 'checkbox') {
                el.checked = !!val;
            } else if (el.tagName === 'SELECT') {
                el.value = String(val);
            } else {
                if (document.activeElement !== el) {
                    el.value = String(val);
                }
            }
        });

        // gs-each
        rootEl.querySelectorAll('[gs-each]').forEach(function(templateEl) {
            if (!isDirectChild(templateEl, rootEl)) return;

            var meta = eachMeta.get(templateEl);
            if (!meta) return;

            var expr      = templateEl.getAttribute('gs-each');
            var match     = expr.match(/^(\w+)(?:,\s*(\w+))?\s+in\s+(.+)$/);
            if (!match) return;

            var itemVar   = match[1];
            var indexVar  = match[2] || null;
            var listExpr  = match[3].trim();
            var list      = evaluate(listExpr, data);

            if (!Array.isArray(list)) return;

            var placeholder = meta.placeholder;
            var template    = meta.template;

            // Remove previous clones
            var sibling = placeholder.nextSibling;
            while (sibling && sibling._glideClone) {
                var next = sibling.nextSibling;
                sibling.parentNode.removeChild(sibling);
                sibling = next;
            }

            // Insert fresh clones
            list.forEach(function(item, idx) {
                var clone = template.cloneNode(true);
                clone.removeAttribute('gs-each');
                clone._glideClone = true;

                var itemScope = Object.assign({}, data, { [itemVar]: item });
                if (indexVar) itemScope[indexVar] = idx;

                applyDirectivesToNode(clone, itemScope);

                placeholder.parentNode.insertBefore(clone, placeholder.nextSibling);
            });
        });
    }

    // Apply directives to a cloned element tree (used for gs-each clones)
    function applyDirectivesToNode(el, data) {
        if (!el || el.nodeType !== 1) return;

        if (el.hasAttribute('gs-if')) {
            el.style.display = evaluate(el.getAttribute('gs-if'), data) ? '' : 'none';
        }
        if (el.hasAttribute('gs-text')) {
            var r = evaluate(el.getAttribute('gs-text'), data);
            if (r !== undefined) el.textContent = String(r);
        }
        if (el.hasAttribute('gs-html')) {
            var r = evaluate(el.getAttribute('gs-html'), data);
            if (r !== undefined) el.innerHTML = String(r);
        }
        if (el.hasAttribute('gs-class')) {
            var classMap = evaluate(el.getAttribute('gs-class'), data);
            if (classMap && typeof classMap === 'object') {
                Object.entries(classMap).forEach(function(entry) {
                    el.classList.toggle(entry[0], !!entry[1]);
                });
            }
        }
        if (el.hasAttribute('gs-attr')) {
            var attrMap = evaluate(el.getAttribute('gs-attr'), data);
            if (attrMap && typeof attrMap === 'object') {
                Object.entries(attrMap).forEach(function(entry) {
                    el.setAttribute(entry[0], entry[1]);
                });
            }
        }
        if (el.hasAttribute('gs-click')) {
            var stmt = el.getAttribute('gs-click');
            el.addEventListener('click', function() { execute(stmt, data); });
        }

        Array.from(el.children).forEach(function(child) {
            applyDirectivesToNode(child, data);
        });
    }

    // ── Event Binding ─────────────────────────────────────────────────────────

    function bindEvents(rootEl, reactive) {
        // gs-click
        rootEl.querySelectorAll('[gs-click]').forEach(function(el) {
            if (el._glideClick) return;
            if (!isDirectChild(el, rootEl)) return;
            el._glideClick = true;
            var stmt = el.getAttribute('gs-click');
            el.addEventListener('click', function(e) {
                execute(stmt, reactive);
            });
        });

        // gs-model (input → data)
        rootEl.querySelectorAll('[gs-model]').forEach(function(el) {
            if (el._glideModel) return;
            if (!isDirectChild(el, rootEl)) return;
            el._glideModel = true;
            var key = el.getAttribute('gs-model');
            var eventType = (el.type === 'checkbox' || el.tagName === 'SELECT') ? 'change' : 'input';
            el.addEventListener(eventType, function() {
                if (el.type === 'checkbox') {
                    reactive[key] = el.checked;
                } else if (el.type === 'number' || el.type === 'range') {
                    reactive[key] = parseFloat(el.value);
                } else {
                    reactive[key] = el.value;
                }
            });
        });

        // gs-submit
        rootEl.querySelectorAll('[gs-submit]').forEach(function(el) {
            if (el._glideSubmit) return;
            if (!isDirectChild(el, rootEl)) return;
            el._glideSubmit = true;
            var stmt = el.getAttribute('gs-submit');
            el.addEventListener('submit', function(e) {
                e.preventDefault();
                execute(stmt, reactive);
            });
        });

        // gs-change
        rootEl.querySelectorAll('[gs-change]').forEach(function(el) {
            if (el._glideChange) return;
            if (!isDirectChild(el, rootEl)) return;
            el._glideChange = true;
            var stmt = el.getAttribute('gs-change');
            el.addEventListener('change', function() {
                execute(stmt, reactive);
            });
        });
    }

    // ── Scope Initializer ─────────────────────────────────────────────────────

    function initScope(rootEl) {
        var plainData = parseData(rootEl.getAttribute('gs-data'));
        var eachMeta  = new Map();

        // Pre-process gs-each: anchor each template with a comment placeholder
        // and hide the original element (used only as the Map key)
        rootEl.querySelectorAll('[gs-each]').forEach(function(tplEl) {
            if (!isDirectChild(tplEl, rootEl)) return;

            var placeholder = document.createComment('gs-each:' + tplEl.getAttribute('gs-each'));
            tplEl.parentNode.insertBefore(placeholder, tplEl);

            eachMeta.set(tplEl, {
                template:    tplEl.cloneNode(true),
                placeholder: placeholder
            });

            // Hide the original; clones will appear after the placeholder
            tplEl.style.display = 'none';
        });

        var pending = false;

        function scheduleUpdate() {
            if (pending) return;
            pending = true;
            requestAnimationFrame(function() {
                pending = false;
                updateScope(rootEl, plainData, eachMeta);
                bindEvents(rootEl, reactive);
            });
        }

        var reactive = makeReactive(plainData, scheduleUpdate);

        // Initial render
        updateScope(rootEl, plainData, eachMeta);
        bindEvents(rootEl, reactive);

        // Expose for devtools: element._glide.data
        rootEl._glide = { data: reactive, refresh: scheduleUpdate };
    }

    // ── Bootstrap ─────────────────────────────────────────────────────────────

    function scanAndInit() {
        document.querySelectorAll('[gs-data]').forEach(function(el) {
            if (!el._glideInit) {
                el._glideInit = true;
                initScope(el);
            }
        });
    }

    // Watch for dynamically injected scopes (server-side partials, HTMX swaps, etc.)
    if (typeof MutationObserver !== 'undefined') {
        var observer = new MutationObserver(function(mutations) {
            var needsScan = false;
            mutations.forEach(function(m) {
                m.addedNodes.forEach(function(node) {
                    if (node.nodeType === 1) {
                        if (node.hasAttribute('gs-data') || node.querySelector('[gs-data]')) {
                            needsScan = true;
                        }
                    }
                });
            });
            if (needsScan) scanAndInit();
        });
        observer.observe(document.documentElement, { childList: true, subtree: true });
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', scanAndInit);
    } else {
        scanAndInit();
    }

    // Public API
    global.Glide = {
        version: '1.0.0',
        init:    scanAndInit,
        eval:    evaluate
    };

})(typeof window !== 'undefined' ? window : this);
`
