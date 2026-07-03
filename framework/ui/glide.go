/*
Purpose:
This file contains the Glide client-side reactive directive engine, embedded as a Go
string constant so the entire runtime ships inside the binary — zero HTTP round-trips
to load a JS file.

Philosophy:
Glide is GoStack's browser-side reactivity layer. Like Alpine.js, it works directly
on existing HTML via data attributes. Unlike Alpine, it is purpose-built for GoStack
components and carries zero third-party footprint.
*/
package ui

// GlideJS is the Glide reactive directive engine runtime, injected into every
// GoStack page via WriteMasterAssetBlock. It is written as a pure JavaScript IIFE
// with zero external dependencies and works in any modern browser.
const GlideJS = `
(function(global) {
    'use strict';

    // ── Expression Helpers ────────────────────────────────────────────────────

    function evaluate(expr, scope, extra) {
        try {
            var context = new Proxy(extra || {}, {
                has: function(target, key) {
                    return key in target || key in scope || key in global;
                },
                get: function(target, key) {
                    if (target && key in target) return target[key];
                    return scope[key];
                },
                set: function(target, key, value) {
                    if (target && key in target) {
                        target[key] = value;
                    } else {
                        scope[key] = value;
                    }
                    return true;
                }
            });
            return (new Function('scope', 'with(scope) { return (' + expr + ') }'))(context);
        } catch(e) {
            return undefined;
        }
    }

    function execute(stmt, scope, extra) {
        try {
            var context = new Proxy(extra || {}, {
                has: function(target, key) {
                    return key in target || key in scope || key in global;
                },
                get: function(target, key) {
                    if (target && key in target) return target[key];
                    return scope[key];
                },
                set: function(target, key, value) {
                    if (target && key in target) {
                        target[key] = value;
                    } else {
                        scope[key] = value;
                    }
                    return true;
                }
            });
            (new Function('scope', 'with(scope) { ' + stmt + ' }'))(context);
        } catch(e) {
            // silent: expression errors should not crash the page
        }
    }

    function parseData(expr, context) {
        try {
            var keys = Object.keys(context || {});
            var vals = keys.map(function(k) { return context[k]; });
            return (new Function(keys.join(','), 'return (' + expr + ')')).apply(null, vals);
        } catch(e) {
            return {};
        }
    }

    // ── Nested Property Helper ────────────────────────────────────────────────

    function setPath(obj, path, value) {
        var parts = path.split('.');
        var current = obj;
        for (var i = 0; i < parts.length - 1; i++) {
            if (current[parts[i]] === undefined) {
                current[parts[i]] = {};
            }
            current = current[parts[i]];
        }
        current[parts[parts.length - 1]] = value;
    }

    // ── Reactivity via Proxy & Array Trapping ─────────────────────────────────

    function makeReactive(obj, callback) {
        if (Array.isArray(obj)) {
            var arrayMethods = ['push', 'pop', 'shift', 'unshift', 'splice', 'sort', 'reverse'];
            arrayMethods.forEach(function(method) {
                var original = Array.prototype[method];
                Object.defineProperty(obj, method, {
                    value: function() {
                        var args = Array.prototype.slice.call(arguments);
                        for (var i = 0; i < args.length; i++) {
                            if (args[i] !== null && typeof args[i] === 'object') {
                                args[i] = makeReactive(args[i], callback);
                            }
                        }
                        var result = original.apply(this, args);
                        callback();
                        return result;
                    }
                });
            });
            for (var i = 0; i < obj.length; i++) {
                if (obj[i] !== null && typeof obj[i] === 'object') {
                    obj[i] = makeReactive(obj[i], callback);
                }
            }
            return obj;
        }
        return new Proxy(obj, {
            set: function(target, key, value) {
                target[key] = value;
                callback();
                return true;
            },
            get: function(target, key) {
                var val = target[key];
                if (val !== null && typeof val === 'object') {
                    return makeReactive(val, callback);
                }
                return val;
            }
        });
    }

    // ── Scope Boundary Check ──────────────────────────────────────────────────
    // Returns true only if el's nearest [gs-data] ancestor IS rootEl.

    function isDirectChild(el, rootEl) {
        var parent = el.parentElement;
        while (parent && parent !== rootEl) {
            if (parent.hasAttribute('gs-data')) return false;
            parent = parent.parentElement;
        }
        return true;
    }

    // ── Transition Support ────────────────────────────────────────────────────

    function toggleElement(el, show) {
        var isTransition = el.hasAttribute('gs-transition');
        
        if (!isTransition) {
            if (show) {
                el.classList.remove('gs-hidden');
                el.style.display = '';
                el.style.visibility = '';
            } else {
                el.classList.add('gs-hidden');
                el.style.display = 'none';
                el.style.visibility = 'hidden';
            }
            return;
        }
        
        if (show) {
            el.classList.remove('gs-hidden');
            el.style.transition = 'opacity 0.2s ease, transform 0.2s ease';
            el.style.transform = 'scale(0.95)';
            el.style.opacity = '0';
            el.style.display = '';
            el.style.visibility = '';
            el.offsetHeight; // force reflow
            requestAnimationFrame(function() {
                el.style.transform = 'scale(1)';
                el.style.opacity = '1';
            });
        } else {
            el.style.transition = 'opacity 0.2s ease, transform 0.2s ease';
            el.style.transform = 'scale(1)';
            el.style.opacity = '1';
            requestAnimationFrame(function() {
                el.style.transform = 'scale(0.95)';
                el.style.opacity = '0';
            });
            var onTransitionEnd = function(e) {
                if (e.target !== el) return;
                el.classList.add('gs-hidden');
                el.style.display = 'none';
                el.style.visibility = 'hidden';
                el.removeEventListener('transitionend', onTransitionEnd);
            };
            el.addEventListener('transitionend', onTransitionEnd);
        }
    }

    // ── DOM Update Pass ───────────────────────────────────────────────────────

    // Define inside scope to allow hoist functions
    function updateScope(rootEl, data, eachMeta) {
        // gs-if
        rootEl.querySelectorAll('[gs-if]').forEach(function(el) {
            if (!isDirectChild(el, rootEl)) return;
            var result = !!evaluate(el.getAttribute('gs-if'), data);
            toggleElement(el, result);
        });

        // gs-show
        rootEl.querySelectorAll('[gs-show]').forEach(function(el) {
            if (!isDirectChild(el, rootEl)) return;
            var result = !!evaluate(el.getAttribute('gs-show'), data);
            toggleElement(el, result);
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

        // gs-effect
        rootEl.querySelectorAll('[gs-effect]').forEach(function(el) {
            if (!isDirectChild(el, rootEl)) return;
            evaluate(el.getAttribute('gs-effect'), data);
        });
        if (rootEl.hasAttribute('gs-effect')) {
            evaluate(rootEl.getAttribute('gs-effect'), data);
        }

        // gs-model
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

    function applyDirectivesToNode(el, data) {
        if (!el || el.nodeType !== 1) return;

        if (el.hasAttribute('gs-if')) {
            toggleElement(el, evaluate(el.getAttribute('gs-if'), data));
        }
        if (el.hasAttribute('gs-show')) {
            toggleElement(el, evaluate(el.getAttribute('gs-show'), data));
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
        if (el.hasAttribute('gs-effect')) {
            evaluate(el.getAttribute('gs-effect'), data);
        }
        if (el.hasAttribute('gs-click')) {
            var stmt = el.getAttribute('gs-click');
            el.addEventListener('click', function(e) {
                execute(stmt, data, { $event: e, event: e, $el: el });
            });
        }

        Array.from(el.children).forEach(function(child) {
            applyDirectivesToNode(child, data);
        });
    }

    // ── Event Binding ─────────────────────────────────────────────────────────

    function bindEvents(rootEl, reactive, refs) {
        // gs-click
        rootEl.querySelectorAll('[gs-click]').forEach(function(el) {
            if (el._glideClick) return;
            if (!isDirectChild(el, rootEl)) return;
            el._glideClick = true;
            var stmt = el.getAttribute('gs-click');
            el.addEventListener('click', function(e) {
                execute(stmt, reactive, { $event: e, event: e, $el: el, $refs: refs });
            });
        });

        // gs-model
        rootEl.querySelectorAll('[gs-model]').forEach(function(el) {
            if (el._glideModel) return;
            if (!isDirectChild(el, rootEl)) return;
            el._glideModel = true;
            var key = el.getAttribute('gs-model');
            var eventType = (el.type === 'checkbox' || el.tagName === 'SELECT') ? 'change' : 'input';
            el.addEventListener(eventType, function() {
                var val;
                if (el.type === 'checkbox') {
                    val = el.checked;
                } else if (el.type === 'number' || el.type === 'range') {
                    val = parseFloat(el.value);
                } else {
                    val = el.value;
                }
                setPath(reactive, key, val);
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
                execute(stmt, reactive, { $event: e, event: e, $el: el, $refs: refs });
            });
        });

        // gs-change
        rootEl.querySelectorAll('[gs-change]').forEach(function(el) {
            if (el._glideChange) return;
            if (!isDirectChild(el, rootEl)) return;
            el._glideChange = true;
            var stmt = el.getAttribute('gs-change');
            el.addEventListener('change', function(e) {
                execute(stmt, reactive, { $event: e, event: e, $el: el, $refs: refs });
            });
        });
    }

    // ── Scope Initializer ─────────────────────────────────────────────────────

    function initScope(rootEl) {
        var persistedKeys = {};
        
        var extraContext = {
            persist: function(key, defaultValue) {
                persistedKeys[key] = true;
                var saved = localStorage.getItem('gs_' + key);
                if (saved !== null) {
                    try { return JSON.parse(saved); } catch(e) { return saved; }
                }
                localStorage.setItem('gs_' + key, JSON.stringify(defaultValue));
                return defaultValue;
            }
        };

        var plainData = parseData(rootEl.getAttribute('gs-data'), extraContext);
        var eachMeta  = new Map();

        // Scan references
        var refs = {};
        rootEl.querySelectorAll('[gs-ref]').forEach(function(el) {
            if (!isDirectChild(el, rootEl)) return;
            refs[el.getAttribute('gs-ref')] = el;
        });
        if (rootEl.hasAttribute('gs-ref')) {
            refs[rootEl.getAttribute('gs-ref')] = rootEl;
        }

        var extraExec = {
            $refs: refs,
            $dispatch: function(name, detail) {
                rootEl.dispatchEvent(new CustomEvent(name, { bubbles: true, detail: detail }));
            }
        };

        // Pre-process gs-each
        rootEl.querySelectorAll('[gs-each]').forEach(function(tplEl) {
            if (!isDirectChild(tplEl, rootEl)) return;

            var placeholder = document.createComment('gs-each:' + tplEl.getAttribute('gs-each'));
            tplEl.parentNode.insertBefore(placeholder, tplEl);

            eachMeta.set(tplEl, {
                template:    tplEl.cloneNode(true),
                placeholder: placeholder
            });

            tplEl.style.display = 'none';
        });

        var pending = false;

        function scheduleUpdate() {
            if (pending) return;
            pending = true;
            requestAnimationFrame(function() {
                pending = false;
                
                // Save persisted keys to localStorage
                Object.keys(persistedKeys).forEach(function(key) {
                    localStorage.setItem('gs_' + key, JSON.stringify(plainData[key]));
                });

                updateScope(rootEl, plainData, eachMeta);
                bindEvents(rootEl, reactive, refs);
            });
        }

        var reactive = makeReactive(plainData, scheduleUpdate);

        // Initial render & bind
        updateScope(rootEl, plainData, eachMeta);
        bindEvents(rootEl, reactive, refs);

        // gs-init Lifecycle
        rootEl.querySelectorAll('[gs-init]').forEach(function(el) {
            if (!isDirectChild(el, rootEl)) return;
            execute(el.getAttribute('gs-init'), reactive, Object.assign({ $el: el }, extraExec));
        });
        if (rootEl.hasAttribute('gs-init')) {
            execute(rootEl.getAttribute('gs-init'), reactive, Object.assign({ $el: rootEl }, extraExec));
        }

        // gs-intersect Observer
        if (typeof IntersectionObserver !== 'undefined') {
            rootEl.querySelectorAll('[gs-intersect]').forEach(function(el) {
                if (!isDirectChild(el, rootEl)) return;
                var observer = new IntersectionObserver(function(entries) {
                    entries.forEach(function(entry) {
                        if (entry.isIntersecting) {
                            execute(el.getAttribute('gs-intersect'), reactive, Object.assign({ $el: el }, extraExec));
                        }
                    });
                });
                observer.observe(el);
            });
            if (rootEl.hasAttribute('gs-intersect')) {
                var observer = new IntersectionObserver(function(entries) {
                    entries.forEach(function(entry) {
                        if (entry.isIntersecting) {
                            execute(rootEl.getAttribute('gs-intersect'), reactive, Object.assign({ $el: rootEl }, extraExec));
                        }
                    });
                });
                observer.observe(rootEl);
            }
        }

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

    // Global Modal Helpers
    global.GoStack = {
        closeModal: function(id) {
            var el = document.getElementById('gs-modal-' + id);
            if (el) {
                if (el.hasAttribute('gs-transition')) {
                    toggleElement(el, false);
                } else {
                    el.classList.add('gs-hidden');
                }
            }
        },
        showModal: function(id) {
            var el = document.getElementById('gs-modal-' + id);
            if (el) {
                if (el.hasAttribute('gs-transition')) {
                    toggleElement(el, true);
                } else {
                    el.classList.remove('gs-hidden');
                }
            }
        }
    };

})(typeof window !== 'undefined' ? window : this);
`
