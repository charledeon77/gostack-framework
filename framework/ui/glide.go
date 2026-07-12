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

    // ── gs-cloak CSS ─────────────────────────────────────────────────────────
    const cloakStyle = document.createElement('style');
    cloakStyle.textContent = '[gs-cloak] { display: none !important; }';
    document.head.appendChild(cloakStyle);

    let globalComponentId = 0;
    const customDirectives = {};
    const customMagics = {};
    const componentRegistry = {};

    // ── Global Reactive Stores ────────────────────────────────────────────────

    const globalCallbacks = new Set();
    const rawStores = {};
    const globalStores = makeReactive(rawStores, function() {
        globalCallbacks.forEach(function(cb) { cb(); });
    });

    const registerStore = function(name, value) {
        if (value === undefined) {
            return globalStores[name];
        }
        globalStores[name] = makeReactive(value, function() {
            globalCallbacks.forEach(function(cb) { cb(); });
        });
        return globalStores[name];
    };

    function destroyNode(node) {
        if (node._glideCleanups) {
            node._glideCleanups.forEach(function(c) { c(); });
        }
        if (node.nodeType === 1 && node.children) {
            Array.from(node.children).forEach(destroyNode);
        }
        if (node.parentNode) {
            node.parentNode.removeChild(node);
        }
    }

    // ── Expression Helpers ────────────────────────────────────────────────────

    function evaluate(expr, scope, extra) {
        try {
            const context = new Proxy(extra || {}, {
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
            const context = new Proxy(extra || {}, {
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
            const keys = Object.keys(context || {});
            const vals = keys.map(function(k) { return context[k]; });
            return (new Function(keys.join(','), 'return (' + expr + ')')).apply(null, vals);
        } catch(e) {
            return {};
        }
    }

    // ── Nested Property Helper ────────────────────────────────────────────────

    function setPath(obj, path, value) {
        const parts = path.split('.');
        let current = obj;
        for (let i = 0; i < parts.length - 1; i++) {
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
            const arrayMethods = ['push', 'pop', 'shift', 'unshift', 'splice', 'sort', 'reverse'];
            arrayMethods.forEach(function(method) {
                const original = Array.prototype[method];
                Object.defineProperty(obj, method, {
                    value: function() {
                        const args = Array.prototype.slice.call(arguments);
                        for (let i = 0; i < args.length; i++) {
                            if (args[i] !== null && typeof args[i] === 'object') {
                                args[i] = makeReactive(args[i], callback);
                            }
                        }
                        const result = original.apply(this, args);
                        callback();
                        return result;
                    }
                });
            });
            for (let i = 0; i < obj.length; i++) {
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
                const val = target[key];
                if (typeof key === 'string' && key.startsWith('$')) return val;
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
        let parent = el.parentElement;
        while (parent && parent !== rootEl) {
            if (parent.hasAttribute('gs-data')) return false;
            parent = parent.parentElement;
        }
        return true;
    }

    // ── Scope Traversal & Throttling Helpers ──────────────────────────────────

    function walkScope(el, callback) {
        if (!el || el.nodeType !== 1) return;
        if (el.hasAttribute('gs-ignore')) return;
        callback(el);
        Array.from(el.children).forEach(function(child) {
            if (child.hasAttribute('gs-data')) return;
            if (child.hasAttribute('gs-ignore')) return;
            walkScope(child, callback);
        });
    }

    function debounce(func, wait) {
        let timeout;
        return function() {
            const context = this, args = arguments;
            clearTimeout(timeout);
            timeout = setTimeout(function() {
                func.apply(context, args);
            }, wait);
        };
    }

    function throttle(func, limit) {
        let inThrottle;
        return function() {
            const args = arguments, context = this;
            if (!inThrottle) {
                func.apply(context, args);
                inThrottle = true;
                setTimeout(function() { inThrottle = false; }, limit);
            }
        };
    }

    // ── Transition Support ────────────────────────────────────────────────────

    // Named transition presets. Each preset defines the CSS that drives
    // both the enter (show) and leave (hide) directions.
    const TRANSITION_PRESETS = {
        fade: {
            duration: 250,
            enter: { css: 'opacity 0.25s ease',
                     from: { opacity: '0' },
                     to:   { opacity: '1' } },
            leave: { css: 'opacity 0.25s ease',
                     from: { opacity: '1' },
                     to:   { opacity: '0' } }
        },
        slide: {
            duration: 260,
            enter: { css: 'opacity 0.26s ease, transform 0.26s ease',
                     from: { opacity: '0', transform: 'translateY(-10px)' },
                     to:   { opacity: '1', transform: 'translateY(0)'     } },
            leave: { css: 'opacity 0.26s ease, transform 0.26s ease',
                     from: { opacity: '1', transform: 'translateY(0)'     },
                     to:   { opacity: '0', transform: 'translateY(-10px)' } }
        },
        scale: {
            duration: 220,
            enter: { css: 'opacity 0.22s ease, transform 0.22s ease',
                     from: { opacity: '0', transform: 'scale(0.9)'  },
                     to:   { opacity: '1', transform: 'scale(1)'    } },
            leave: { css: 'opacity 0.22s ease, transform 0.22s ease',
                     from: { opacity: '1', transform: 'scale(1)'    },
                     to:   { opacity: '0', transform: 'scale(0.9)'  } }
        },
        blur: {
            duration: 280,
            enter: { css: 'opacity 0.28s ease, filter 0.28s ease',
                     from: { opacity: '0', filter: 'blur(6px)'  },
                     to:   { opacity: '1', filter: 'blur(0px)'  } },
            leave: { css: 'opacity 0.28s ease, filter 0.28s ease',
                     from: { opacity: '1', filter: 'blur(0px)'  },
                     to:   { opacity: '0', filter: 'blur(6px)'  } }
        },
        fly: {
            duration: 300,
            enter: { css: 'opacity 0.3s ease, transform 0.3s ease',
                     from: { opacity: '0', transform: 'translate(20px, -20px)' },
                     to:   { opacity: '1', transform: 'translate(0, 0)'        } },
            leave: { css: 'opacity 0.3s ease, transform 0.3s ease',
                     from: { opacity: '1', transform: 'translate(0, 0)'        },
                     to:   { opacity: '0', transform: 'translate(20px, -20px)' } }
        }
    };

    // Default when gs-transition has no value (e.g. just gs-transition="")
    const DEFAULT_TRANSITION = TRANSITION_PRESETS.scale;

    function applyTransitionStyles(el, styles) {
        Object.keys(styles).forEach(function(k) { el.style[k] = styles[k]; });
    }

    function clearTransitionStyles(el, styles) {
        Object.keys(styles).forEach(function(k) { el.style.removeProperty(k); });
    }

    // Cancel any running transition on the element
    function cancelTransition(el) {
        if (el._glideTransitionEnd) {
            el.removeEventListener('transitionend', el._glideTransitionEnd);
            el._glideTransitionEnd = null;
        }
        if (el._glideTransitionFallback) {
            clearTimeout(el._glideTransitionFallback);
            el._glideTransitionFallback = null;
        }
    }

    // ── Transition Override Helpers ───────────────────────────────────────────

    function applyTransitionOverrides(el, phase) {
        const hasDur = el.hasAttribute('gs-transition:duration');
        const hasDel = el.hasAttribute('gs-transition:delay');
        const hasEas = el.hasAttribute('gs-transition:easing');
        if (!hasDur && !hasDel && !hasEas) return phase;
        let css = phase.css;
        if (hasDur) {
            const ms = parseInt(el.getAttribute('gs-transition:duration'));
            if (!isNaN(ms)) css = css.replace(/[\d.]+s/g, (ms / 1000) + 's');
        }
        if (hasEas) {
            const eas = el.getAttribute('gs-transition:easing');
            css = css.replace(/\b(ease|ease-in|ease-out|ease-in-out|linear)\b/g, eas);
        }
        if (hasDel) {
            const del = el.getAttribute('gs-transition:delay');
            css = css.split(',').map(function(p) { return p.trim() + ' ' + del; }).join(', ');
        }
        return { css: css, from: phase.from, to: phase.to };
    }

    // Play a CSS transition on el. Applies 'from' styles, forces a reflow,
    // then applies 'to' styles using double-rAF so the browser always paints
    // the 'from' frame before transitioning.
    // onDone() is called when the animation completes (or falls back).
    function playTransition(el, phase, onDone) {
        cancelTransition(el);
        phase = applyTransitionOverrides(el, phase);

        el.style.transition = 'none';
        applyTransitionStyles(el, phase.from);
        el.offsetHeight; // force paint sync

        requestAnimationFrame(function() {
            requestAnimationFrame(function() {
                el.style.transition = phase.css;
                applyTransitionStyles(el, phase.to);

                const cleanup = function() {
                    cancelTransition(el);
                    el.style.transition = '';
                    clearTransitionStyles(el, phase.to);
                    if (onDone) onDone();
                };

                const onEnd = function(e) {
                    if (e.target !== el) return;
                    cleanup();
                };
                el._glideTransitionEnd = onEnd;
                el.addEventListener('transitionend', onEnd);

                // Safety fallback — fires if transitionend never arrives
                const preset = TRANSITION_PRESETS[
                    (el.getAttribute('gs-transition') || '').trim().toLowerCase()
                ] || DEFAULT_TRANSITION;
                el._glideTransitionFallback = setTimeout(function() {
                    cleanup();
                }, (preset.duration || 300) + 100);
            });
        });
    }

    function toggleElement(el, show) {
        const isTransition = el.hasAttribute('gs-transition');

        if (!isTransition) {
            if (show) {
                el.classList.remove('gs-hidden');
                el.style.display = '';
                el.style.visibility = '';
            } else {
                el.classList.add('gs-hidden');
                el.style.display = 'none';
            }
            return;
        }

        const typeName = (el.getAttribute('gs-transition') || '').trim().toLowerCase();
        const preset = TRANSITION_PRESETS[typeName] || DEFAULT_TRANSITION;

        if (show) {
            cancelTransition(el);
            el.classList.remove('gs-hidden');
            el.style.display = '';
            el.style.visibility = '';
            playTransition(el, preset.enter, null);
        } else {
            playTransition(el, preset.leave, function() {
                el.classList.add('gs-hidden');
                el.style.display = 'none';
            });
        }
    }

    // Animate a newly inserted loop clone (enter) if the template carried gs-transition
    function enterClone(clone, template) {
        const typeName = (template.getAttribute('gs-transition') || '').trim().toLowerCase();
        if (!template.hasAttribute('gs-transition')) return;
        if (clone.hasAttribute('gs-transition')) return; // already handled
        clone.setAttribute('gs-transition', typeName);
        const preset = TRANSITION_PRESETS[typeName] || DEFAULT_TRANSITION;
        playTransition(clone, preset.enter, null);
    }

    // Animate a leaving loop clone (leave) then destroy it
    function leaveClone(clone, template) {
        const typeName = (template.getAttribute('gs-transition') || '').trim().toLowerCase();
        if (!template.hasAttribute('gs-transition')) {
            destroyNode(clone);
            return;
        }
        const preset = TRANSITION_PRESETS[typeName] || DEFAULT_TRANSITION;
        playTransition(clone, preset.leave, function() {
            if (clone.parentNode) destroyNode(clone);
        });
    }

    // ── DOM Update Pass ───────────────────────────────────────────────────────

    // Define inside scope to allow hoist functions
    function updateScope(el, data, isRoot) {
        if (!el || el.nodeType !== 1) return;

        // A. Scope Boundary Check (prevent walking into nested components)
        if (!isRoot && el.hasAttribute('gs-data')) return;
        if (el.hasAttribute('gs-ignore')) return;

        // B. gs-if directive (guards child evaluation)
        if (el.hasAttribute('gs-if')) {
            const show = !!evaluate(el.getAttribute('gs-if'), data, { $el: el });
            toggleElement(el, show);
            if (!show) return;
        }

        // C. gs-show directive
        if (el.hasAttribute('gs-show')) {
            const show = !!evaluate(el.getAttribute('gs-show'), data, { $el: el });
            toggleElement(el, show);
        }

        // D. Loop Reconciliation (gs-each)
        if (el.hasAttribute('gs-each')) {
            const expr = el.getAttribute('gs-each');
            const match = expr.match(/^(\w+)(?:,\s*(\w+))?\s+in\s+(.+)$/);
            if (!match) return;

            const itemVar = match[1];
            const indexVar = match[2] || null;
            const listExpr = match[3].trim();
            const list = evaluate(listExpr, data, { $el: el });

            if (!Array.isArray(list)) return;

            // Lazy Compile Loop Blueprint
            if (!el._glidePlaceholder) {
                const placeholder = document.createComment('gs-each:' + expr);
                el.parentNode.insertBefore(placeholder, el);
                
                const template = el.cloneNode(true);
                template.removeAttribute('gs-each');
                
                el._glidePlaceholder = placeholder;
                el._glideTemplate = template;
                el.parentNode.removeChild(el); // Remove original template to stop query pollution
            }

            const placeholder = el._glidePlaceholder;
            const template = el._glideTemplate;

            // Gather active clones
            const oldClones = [];
            let sibling = placeholder.nextSibling;
            while (sibling && sibling._glideClone) {
                oldClones.push(sibling);
                sibling = sibling.nextSibling;
            }

            const oldMap = new Map();
            oldClones.forEach(function(clone, i) {
                const key = clone._glideKey !== undefined ? clone._glideKey : String(i);
                oldMap.set(key, clone);
            });

            let keyExpr = null;
            if (template.hasAttribute(':key')) {
                keyExpr = template.getAttribute(':key');
            } else if (template.hasAttribute('gs-bind:key')) {
                keyExpr = template.getAttribute('gs-bind:key');
            }

            // FLIP Animation: Record initial positions
            const isFlip = el.hasAttribute('gs-animate') && el.getAttribute('gs-animate').toLowerCase() === 'flip';
            const firstRects = isFlip ? new Map() : null;
            if (isFlip) {
                oldClones.forEach(function(clone) {
                    firstRects.set(clone._glideKey, clone.getBoundingClientRect());
                });
            }

            let lastNode = placeholder;
            const newKeys = new Set();

            list.forEach(function(item, idx) {
                const itemScope = Object.create(data);
                itemScope[itemVar] = item;
                if (indexVar) itemScope[indexVar] = idx;

                let key = String(idx);
                if (keyExpr) {
                    const rawKey = evaluate(keyExpr, itemScope, { $el: template });
                    key = (rawKey !== null && rawKey !== undefined) ? String(rawKey) : String(idx);
                }

                newKeys.add(key);

                let clone = oldMap.get(key);
                if (clone) {
                    oldMap.delete(key);
                    clone._glideScope = itemScope;
                    updateScopeDirectivesOnly(clone, itemScope);
                } else {
                    clone = template.cloneNode(true);
                    clone.removeAttribute(':key');
                    clone.removeAttribute('gs-bind:key');
                    clone._glideClone = true;
                    clone._glideKey = key;
                    clone._glideNew = true;
                    clone._glideScope = itemScope;
                    updateScopeDirectivesOnly(clone, itemScope);
                }

                // Focus-Preserved Move
                if (lastNode.nextSibling !== clone) {
                    const activeEl = document.activeElement;
                    const activeInside = clone.contains(activeEl);
                    let selectionStart = null, selectionEnd = null;

                    if (activeInside && typeof activeEl.selectionStart === 'number') {
                        selectionStart = activeEl.selectionStart;
                        selectionEnd = activeEl.selectionEnd;
                    }

                    placeholder.parentNode.insertBefore(clone, lastNode.nextSibling);

                    if (activeInside && document.activeElement !== activeEl) {
                        activeEl.focus();
                        if (selectionStart !== null) {
                            activeEl.setSelectionRange(selectionStart, selectionEnd);
                        }
                    }
                }

                // Play enter animation on first-time inserts
                if (clone._glideNew) {
                    clone._glideNew = false;
                    enterClone(clone, template);
                }

                lastNode = clone;
            });

            oldClones.forEach(function(clone) {
                if (!newKeys.has(clone._glideKey)) {
                    leaveClone(clone, template);
                }
            });

            // FLIP Animation: Animate from old to new positions
            if (isFlip && firstRects) {
                let flipNode = placeholder.nextSibling;
                while (flipNode && flipNode._glideClone) {
                    (function(node) {
                        const first = firstRects.get(node._glideKey);
                        if (first) {
                            const last = node.getBoundingClientRect();
                            const dx = first.left - last.left;
                            const dy = first.top - last.top;
                            if (dx !== 0 || dy !== 0) {
                                node.style.transform = 'translate(' + dx + 'px, ' + dy + 'px)';
                                node.style.transition = 'none';
                                node.offsetHeight;
                                requestAnimationFrame(function() {
                                    node.style.transition = 'transform 0.3s ease';
                                    node.style.transform = '';
                                    const onEnd = function() {
                                        node.style.transition = '';
                                        node.removeEventListener('transitionend', onEnd);
                                    };
                                    node.addEventListener('transitionend', onEnd);
                                });
                            }
                        }
                    })(flipNode);
                    flipNode = flipNode.nextSibling;
                }
            }

            return; // Stopped: Clones evaluation handled internally
        }

        // E. Evaluate Directives on el
        updateDirectivesOnNode(el, data);

        // F. Walk children
        Array.from(el.children).forEach(function(child) {
            if (child.hasAttribute('gs-ignore')) return;
            updateScope(child, data, false);
        });
    }

    function updateScopeDirectivesOnly(el, data) {
        updateDirectivesOnNode(el, data);
        Array.from(el.children).forEach(function(child) {
            if (child.hasAttribute('gs-data')) return;
            if (child.hasAttribute('gs-ignore')) return;
            updateScope(child, data, false);
        });
    }

    function updateDirectivesOnNode(el, data) {
        if (el.hasAttribute('gs-text')) {
            const r = evaluate(el.getAttribute('gs-text'), data, { $el: el });
            if (r !== undefined && r !== null) el.textContent = String(r);
        }
        if (el.hasAttribute('gs-html')) {
            const r = evaluate(el.getAttribute('gs-html'), data, { $el: el });
            if (r !== undefined && r !== null) el.innerHTML = String(r);
        }
        if (el.hasAttribute('gs-class')) {
            const classMap = evaluate(el.getAttribute('gs-class'), data, { $el: el });
            if (classMap && typeof classMap === 'object') {
                Object.entries(classMap).forEach(function(entry) {
                    el.classList.toggle(entry[0], !!entry[1]);
                });
            }
        }
        if (el.hasAttribute('gs-attr')) {
            const attrMap = evaluate(el.getAttribute('gs-attr'), data, { $el: el });
            if (attrMap && typeof attrMap === 'object') {
                Object.entries(attrMap).forEach(function(entry) {
                    el.setAttribute(entry[0], entry[1]);
                });
            }
        }
        if (el.hasAttribute('gs-effect')) {
            evaluate(el.getAttribute('gs-effect'), data, { $el: el });
        }
        if (el.hasAttribute('gs-model')) {
            const key = el.getAttribute('gs-model');
            const val = evaluate(key, data, { $el: el });
            if (val !== undefined) {
                if (el.type === 'checkbox') {
                    el.checked = !!val;
                } else if (el.tagName === 'SELECT') {
                    el.value = String(val);
                } else {
                    if (document.activeElement !== el) {
                        el.value = String(val);
                    }
                }
            }
        }

        // Dynamic attribute bindings (:attr)
        el._glide_last_bind = el._glide_last_bind || {};
        Array.from(el.attributes).forEach(function(attr) {
            const name = attr.name;
            const isBind = name.startsWith('gs-bind:') || name.startsWith(':');
            if (!isBind) return;

            const attrName = name.startsWith('gs-bind:') ? name.slice(8) : name.slice(1);
            const expr = attr.value;
            const val = evaluate(expr, data, { $el: el });

            if (el._glide_last_bind[name] === val) return;
            el._glide_last_bind[name] = val;

            if (attrName === 'class') {
                if (el._glideInitialClass === undefined) {
                    el._glideInitialClass = el.className || '';
                }
                if (val && typeof val === 'object' && !Array.isArray(val)) {
                    el.className = el._glideInitialClass;
                    Object.entries(val).forEach(function(entry) {
                        el.classList.toggle(entry[0], !!entry[1]);
                    });
                } else if (Array.isArray(val)) {
                    el.className = (el._glideInitialClass + ' ' + val.filter(Boolean).join(' ')).trim();
                } else if (typeof val === 'string') {
                    el.className = (el._glideInitialClass + ' ' + val).trim();
                }
            } else if (attrName === 'style') {
                if (el._glideInitialStyle === undefined) {
                    el._glideInitialStyle = el.getAttribute('style') || '';
                }
                el._glideLastStyleKeys = el._glideLastStyleKeys || [];
                el._glideLastStyleKeys.forEach(function(k) { el.style.removeProperty(k); });

                const newStyleKeys = [];
                const applyStyleObj = function(obj) {
                    Object.entries(obj).forEach(function(entry) {
                        const styleName = entry[0].replace(/([a-z0-9]|(?=[A-Z]))([A-Z])/g, '$1-$2').toLowerCase();
                        el.style.setProperty(styleName, String(entry[1]));
                        newStyleKeys.push(styleName);
                    });
                };

                if (val && typeof val === 'object' && !Array.isArray(val)) {
                    applyStyleObj(val);
                    el._glideLastStyleKeys = newStyleKeys;
                } else if (Array.isArray(val)) {
                    val.forEach(function(styleObj) {
                        if (styleObj && typeof styleObj === 'object') applyStyleObj(styleObj);
                    });
                    el._glideLastStyleKeys = newStyleKeys;
                } else if (typeof val === 'string') {
                    el.setAttribute('style', (el._glideInitialStyle + '; ' + val).trim());
                    el._glideLastStyleKeys = [];
                }
            } else {
                if (attrName === 'checked' && 'checked' in el) {
                    el.checked = !!val;
                } else if (attrName === 'value' && 'value' in el) {
                    el.value = (val === null || val === undefined) ? '' : String(val);
                }
                if (val === false || val === null || val === undefined) {
                    el.removeAttribute(attrName);
                } else {
                    el.setAttribute(attrName, String(val));
                }
            }
        });

        // Custom directives (registered via Glide.directive)
        Object.keys(customDirectives).forEach(function(name) {
            const attrName = 'gs-' + name;
            if (el.hasAttribute(attrName)) {
                const expr = el.getAttribute(attrName);
                try {
                    customDirectives[name](el, {
                        expression: expr,
                        evaluate: function(e) { return evaluate(e || expr, data, { $el: el }); },
                        scope: data
                    });
                } catch(e) { console.error('[Glide] Custom directive error:', e); }
            }
        });
    }

    // ── Event Binding ─────────────────────────────────────────────────────────

    function bindEvents(rootEl, reactive, refs) {
        walkScope(rootEl, function(el) {
            // 1. gs-model two-way binding
            if (el.hasAttribute('gs-model')) {
                const key = el.getAttribute('gs-model');
                const boundKey = '_glide_bound_gs_model_' + key;
                if (!el[boundKey]) {
                    el[boundKey] = true;
                    const eventType = (el.type === 'checkbox' || el.tagName === 'SELECT') ? 'change' : 'input';
                    el.addEventListener(eventType, function() {
                        let val;
                        if (el.type === 'checkbox') {
                            val = el.checked;
                        } else if (el.type === 'number' || el.type === 'range') {
                            val = parseFloat(el.value);
                        } else {
                            val = el.value;
                        }
                        
                        // Resolve scope dynamically
                        let currentScope = reactive;
                        let p = el;
                        while (p) {
                            if (p._glideScope) {
                                currentScope = p._glideScope;
                                break;
                            }
                            p = p.parentElement;
                        }
                        setPath(currentScope, key, val);
                    });
                }
            }

            // 2. Generic Event Listeners: gs-on:event or @event
            Array.from(el.attributes).forEach(function(attr) {
                const name = attr.name;
                const isEvent = name.startsWith('gs-on:') || name.startsWith('@');
                if (!isEvent) return;

                const boundKey = '_glide_bound_event_' + name;
                if (el[boundKey]) return;
                el[boundKey] = true;

                const rawEvent = name.startsWith('gs-on:') ? name.slice(6) : name.slice(1);
                const parts = rawEvent.split('.');
                const eventName = parts[0];
                const modifiers = parts.slice(1);
                const stmt = attr.value;

                let listener = function(e) {
                    // System modifier key filters
                    if (modifiers.includes('ctrl') && !e.ctrlKey) return;
                    if (modifiers.includes('shift') && !e.shiftKey) return;
                    if (modifiers.includes('alt') && !e.altKey) return;
                    if (modifiers.includes('meta') && !e.metaKey) return;

                    // Event flow controls
                    if (modifiers.includes('stop')) e.stopPropagation();
                    if (modifiers.includes('prevent')) e.preventDefault();
                    if (modifiers.includes('self') && e.target !== el) return;
                    if (modifiers.includes('outside')) {
                        if (el.contains(e.target) || el.offsetWidth === 0 || el.offsetHeight === 0) return;
                    }

                    // Key filters for keyboard events (e.g. @keyup.escape, @keydown.enter)
                    if (eventName.startsWith('key')) {
                        const keyFilter = modifiers.find(function(m) {
                            if (m.match(/^\d+/) || m.endsWith('ms')) return false; // exclude debounce/throttle durations
                            return !['ctrl', 'shift', 'alt', 'meta', 'stop', 'prevent', 'self', 'once', 'window', 'document', 'outside', 'debounce', 'throttle', 'passive', 'capture'].includes(m);
                        });
                        if (keyFilter) {
                            let checkKey = keyFilter.toLowerCase();
                            let eventKey = e.key.toLowerCase();
                            if (checkKey === 'escape') checkKey = 'esc';
                            if (eventKey === 'escape') eventKey = 'esc';
                            if (checkKey === 'space') checkKey = ' ';
                            if (eventKey === ' ') eventKey = 'space';
                            
                            if (eventKey !== checkKey) return;
                        }
                    }

                    // Resolve scope dynamically
                    let currentScope = reactive;
                    let p = el;
                    while (p) {
                        if (p._glideScope) {
                            currentScope = p._glideScope;
                            break;
                        }
                        p = p.parentElement;
                    }

                    execute(stmt, currentScope, { $event: e, event: e, $el: el, $refs: refs });
                };

                // Debounce / Throttle wraps
                if (modifiers.includes('debounce')) {
                    const debounceIdx = modifiers.indexOf('debounce');
                    let ms = 250;
                    if (debounceIdx + 1 < modifiers.length) {
                        const potentialMs = parseInt(modifiers[debounceIdx + 1]);
                        if (!isNaN(potentialMs)) ms = potentialMs;
                    }
                    listener = debounce(listener, ms);
                } else if (modifiers.includes('throttle')) {
                    const throttleIdx = modifiers.indexOf('throttle');
                    let ms = 250;
                    if (throttleIdx + 1 < modifiers.length) {
                        const potentialMs = parseInt(modifiers[throttleIdx + 1]);
                        if (!isNaN(potentialMs)) ms = potentialMs;
                    }
                    listener = throttle(listener, ms);
                }

                // Native listener options
                const options = {};
                if (modifiers.includes('passive')) options.passive = true;
                if (modifiers.includes('capture')) options.capture = true;
                if (modifiers.includes('once')) options.once = true;

                // Bind targets & Cleanup tracking
                el._glideCleanups = el._glideCleanups || [];
                if (modifiers.includes('window')) {
                    window.addEventListener(eventName, listener, options);
                    el._glideCleanups.push(function() { window.removeEventListener(eventName, listener, options); });
                } else if (modifiers.includes('document')) {
                    document.addEventListener(eventName, listener, options);
                    el._glideCleanups.push(function() { document.removeEventListener(eventName, listener, options); });
                } else if (modifiers.includes('outside')) {
                    document.addEventListener('click', listener, options);
                    el._glideCleanups.push(function() { document.removeEventListener('click', listener, options); });
                } else {
                    el.addEventListener(eventName, listener, options);
                }
            });

            // 3. Backward compatibility triggers
            if (el.hasAttribute('gs-click') && !el._glideClick) {
                el._glideClick = true;
                const stmt = el.getAttribute('gs-click');
                el.addEventListener('click', function(e) {
                    let currentScope = reactive;
                    let p = el;
                    while (p) {
                        if (p._glideScope) {
                            currentScope = p._glideScope;
                            break;
                        }
                        p = p.parentElement;
                    }
                    execute(stmt, currentScope, { $event: e, event: e, $el: el, $refs: refs });
                });
            }
            if (el.hasAttribute('gs-submit') && !el._glideSubmit) {
                el._glideSubmit = true;
                const stmt = el.getAttribute('gs-submit');
                el.addEventListener('submit', function(e) {
                    e.preventDefault();
                    let currentScope = reactive;
                    let p = el;
                    while (p) {
                        if (p._glideScope) {
                            currentScope = p._glideScope;
                            break;
                        }
                        p = p.parentElement;
                    }
                    execute(stmt, currentScope, { $event: e, event: e, $el: el, $refs: refs });
                });
            }
            if (el.hasAttribute('gs-change') && !el._glideChange) {
                el._glideChange = true;
                const stmt = el.getAttribute('gs-change');
                el.addEventListener('change', function(e) {
                    let currentScope = reactive;
                    let p = el;
                    while (p) {
                        if (p._glideScope) {
                            currentScope = p._glideScope;
                            break;
                        }
                        p = p.parentElement;
                    }
                    execute(stmt, currentScope, { $event: e, event: e, $el: el, $refs: refs });
                });
            }
        });
    }

    // ── Scope Initializer ─────────────────────────────────────────────────────

    function initScope(rootEl) {
        const persistedKeys = {};
        const teleportedElements = [];
        
        const extraContext = {
            persist: function(key, defaultValue) {
                persistedKeys[key] = true;
                const saved = localStorage.getItem('gs_' + key);
                if (saved !== null) {
                    try { return JSON.parse(saved); } catch(e) { return saved; }
                }
                localStorage.setItem('gs_' + key, JSON.stringify(defaultValue));
                return defaultValue;
            }
        };

        const plainData = rootEl._glideComponentData || parseData(rootEl.getAttribute('gs-data'), extraContext);
        delete rootEl._glideComponentData;

        // Scan references
        const refs = {};
        rootEl.querySelectorAll('[gs-ref]').forEach(function(el) {
            if (!isDirectChild(el, rootEl)) return;
            refs[el.getAttribute('gs-ref')] = el;
        });
        if (rootEl.hasAttribute('gs-ref')) {
            refs[rootEl.getAttribute('gs-ref')] = rootEl;
        }

        const extraExec = {
            $refs: refs,
            $dispatch: function(name, detail) {
                rootEl.dispatchEvent(new CustomEvent(name, { bubbles: true, detail: detail }));
            }
        };

        const watchersList = [];
        const nextTickQueue = [];

        const $watch = function(expr, callback) {
            watchersList.push({
                expr: expr,
                callback: callback,
                lastVal: evaluate(expr, reactive)
            });
        };

        const $nextTick = function(callback) {
            if (pending) {
                nextTickQueue.push(callback);
            } else {
                Promise.resolve().then(callback);
            }
        };

        let pending = false;

        function scheduleUpdate() {
            if (pending) return;
            pending = true;
            requestAnimationFrame(function() {
                pending = false;
                
                // Save persisted keys to localStorage
                Object.keys(persistedKeys).forEach(function(key) {
                    localStorage.setItem('gs_' + key, JSON.stringify(plainData[key]));
                });

                updateScope(rootEl, plainData, true);
                teleportedElements.forEach(function(tel) {
                    updateScope(tel, plainData, false);
                });
                bindEvents(rootEl, reactive, refs);

                // Process reactive watchers post-render
                watchersList.forEach(function(w) {
                    const newVal = evaluate(w.expr, reactive);
                    if (newVal !== w.lastVal) {
                        const oldVal = w.lastVal;
                        w.lastVal = newVal;
                        try { w.callback(newVal, oldVal); } catch(e) { console.error('[Glide] $watch callback error:', e); }
                    }
                });

                // Drain post-render nextTick queue
                while (nextTickQueue.length > 0) {
                    const callback = nextTickQueue.shift();
                    try { callback(); } catch(e) { console.error(e); }
                }
            });
        }

        const reactive = makeReactive(plainData, scheduleUpdate);
        reactive.$refs = refs;
        reactive.$dispatch = extraExec.$dispatch;
        reactive.$watch = $watch;
        reactive.$nextTick = $nextTick;
        reactive.$store = globalStores;
        reactive.$root = rootEl;
        reactive.$data = plainData;
        reactive.$errors = {};

        const compId = ++globalComponentId;
        let idSeq = 0;
        reactive.$id = function(name) {
            return 'glide-' + compId + '-' + (name || (++idSeq));
        };

        reactive.$persist = function(key, defaultValue) {
            persistedKeys[key] = true;
            const saved = localStorage.getItem('gs_' + key);
            if (saved !== null) {
                try { return JSON.parse(saved); } catch(e) { return saved; }
            }
            localStorage.setItem('gs_' + key, JSON.stringify(defaultValue));
            return defaultValue;
        };

        // Custom magics (registered via Glide.magic)
        Object.keys(customMagics).forEach(function(name) {
            Object.defineProperty(reactive, '$' + name, {
                get: function() {
                    try { return customMagics[name](rootEl, reactive); } catch(e) { return undefined; }
                },
                configurable: true
            });
        });

        // Register scheduleUpdate to be triggered when global stores change
        globalCallbacks.add(scheduleUpdate);
        rootEl._glideCleanups = rootEl._glideCleanups || [];
        rootEl._glideCleanups.push(function() {
            globalCallbacks.delete(scheduleUpdate);
        });

        // Initial render & bind
        updateScope(rootEl, plainData, true);
        bindEvents(rootEl, reactive, refs);

        // gs-init Lifecycle
        rootEl.querySelectorAll('[gs-init]').forEach(function(el) {
            if (!isDirectChild(el, rootEl)) return;
            execute(el.getAttribute('gs-init'), reactive, Object.assign({ $el: el }, extraExec));
        });
        if (rootEl.hasAttribute('gs-init')) {
            execute(rootEl.getAttribute('gs-init'), reactive, Object.assign({ $el: rootEl }, extraExec));
        }

        // gs-modelable: bidirectional parent/child property sync
        if (rootEl.hasAttribute('gs-modelable')) {
            const exposedProp = rootEl.getAttribute('gs-modelable');
            let parentScopeEl = rootEl.parentElement;
            while (parentScopeEl && !parentScopeEl._glide) {
                parentScopeEl = parentScopeEl.parentElement;
            }
            if (parentScopeEl && parentScopeEl._glide && rootEl.hasAttribute('gs-model')) {
                const parentProp = rootEl.getAttribute('gs-model');
                const parentData = parentScopeEl._glide.data;
                // Initial sync: parent → child
                const pVal = evaluate(parentProp, parentData);
                if (pVal !== undefined) plainData[exposedProp] = pVal;
                // Watch child → parent
                watchersList.push({
                    expr: exposedProp,
                    callback: function(newVal) { setPath(parentData, parentProp, newVal); },
                    lastVal: plainData[exposedProp]
                });
            }
        }

        // gs-teleport: move element to external target while keeping scope
        rootEl.querySelectorAll('[gs-teleport]').forEach(function(el) {
            if (!isDirectChild(el, rootEl)) return;
            if (el._glideTeleported) return;
            el._glideTeleported = true;
            const target = document.querySelector(el.getAttribute('gs-teleport'));
            if (target) {
                target.appendChild(el);
                teleportedElements.push(el);
            }
        });

        // gs-intersect Observer
        if (typeof IntersectionObserver !== 'undefined') {
            rootEl.querySelectorAll('[gs-intersect]').forEach(function(el) {
                if (!isDirectChild(el, rootEl)) return;
                const observer = new IntersectionObserver(function(entries) {
                    entries.forEach(function(entry) {
                        if (entry.isIntersecting) {
                            execute(el.getAttribute('gs-intersect'), reactive, Object.assign({ $el: el }, extraExec));
                        }
                    });
                });
                observer.observe(el);
            });
            if (rootEl.hasAttribute('gs-intersect')) {
                const observer = new IntersectionObserver(function(entries) {
                    entries.forEach(function(entry) {
                        if (entry.isIntersecting) {
                            execute(rootEl.getAttribute('gs-intersect'), reactive, Object.assign({ $el: rootEl }, extraExec));
                        }
                    });
                });
                observer.observe(rootEl);
            }
        }

        // gs-validate: form validation & AJAX submission
        rootEl.querySelectorAll('form[gs-validate]').forEach(function(form) {
            if (!isDirectChild(form, rootEl)) return;
            if (form._glideValidate) return;
            form._glideValidate = true;
            form.setAttribute('novalidate', '');
            form.addEventListener('submit', function(e) {
                e.preventDefault();
                reactive.$errors = {};
                let isValid = true;
                Array.from(form.elements).forEach(function(input) {
                    if (input.name && !input.checkValidity()) {
                        isValid = false;
                        reactive.$errors[input.name] = input.validationMessage;
                    }
                });
                if (isValid) {
                    const stmt = form.getAttribute('gs-validate');
                    if (stmt) {
                        const fd = new FormData(form);
                        const formObj = {};
                        fd.forEach(function(v, k) { formObj[k] = v; });
                        execute(stmt, reactive, Object.assign({
                            $el: form, $formData: fd, $form: formObj
                        }, extraExec));
                    }
                }
                scheduleUpdate();
            });
        });

        rootEl._glide = { data: reactive, refresh: scheduleUpdate };
    }

    // ── Bootstrap ─────────────────────────────────────────────────────────────

    function scanAndInit() {
        // Expand registered components (Glide.component)
        Object.keys(componentRegistry).forEach(function(name) {
            document.querySelectorAll(name + ':not([gs-data])').forEach(function(el) {
                if (el._glideComponent) return;
                el._glideComponent = true;
                const def = componentRegistry[name];
                const props = {};
                Array.from(el.attributes).forEach(function(attr) {
                    props[attr.name] = attr.value;
                });
                if (def.template) el.innerHTML = def.template;
                const dataFn = typeof def.data === 'function' ? def.data : function() { return def.data || {}; };
                el._glideComponentData = dataFn(props);
                el.setAttribute('gs-data', '{}');
            });
        });

        document.querySelectorAll('[gs-data]').forEach(function(el) {
            if (!el._glideInit) {
                el._glideInit = true;
                initScope(el);
                el.removeAttribute('gs-cloak');
            }
        });
    }

    // ── DOM Morphing Router ───────────────────────────────────────────────────

    function morph(oldNode, newNode) {
        if (oldNode.nodeType !== newNode.nodeType || oldNode.nodeName !== newNode.nodeName) {
            const clone = oldNode.ownerDocument.importNode(newNode, true);
            oldNode.parentNode.replaceChild(clone, oldNode);
            return;
        }

        if (oldNode.nodeType === 3 || oldNode.nodeType === 8) {
            if (oldNode.nodeValue !== newNode.nodeValue) {
                oldNode.nodeValue = newNode.nodeValue;
            }
            return;
        }

        if (oldNode.nodeType === 1) {
            // Morph attributes
            Array.from(oldNode.attributes).forEach(function(attr) {
                if (!newNode.hasAttribute(attr.name)) {
                    oldNode.removeAttribute(attr.name);
                }
            });
            Array.from(newNode.attributes).forEach(function(attr) {
                if (oldNode.getAttribute(attr.name) !== attr.value) {
                    oldNode.setAttribute(attr.name, attr.value);
                }
            });

            // Reconcile children
            const oldChildren = Array.from(oldNode.childNodes);
            const newChildren = Array.from(newNode.childNodes);
            const maxLen = Math.max(oldChildren.length, newChildren.length);

            for (let i = 0; i < maxLen; i++) {
                const oldChild = oldChildren[i];
                const newChild = newChildren[i];

                if (!oldChild && newChild) {
                    oldNode.appendChild(oldNode.ownerDocument.importNode(newChild, true));
                } else if (oldChild && !newChild) {
                    destroyNode(oldChild);
                } else if (oldChild && newChild) {
                    morph(oldChild, newChild);
                }
            }
        }
    }

    function navigate(url, push) {
        if (push === undefined) push = true;
        fetch(url)
            .then(function(res) { return res.text(); })
            .then(function(html) {
                const parser = new DOMParser();
                const doc = parser.parseFromString(html, 'text/html');

                const oldTarget = document.querySelector('[gs-route-target]') || document.body;
                const newTarget = doc.querySelector('[gs-route-target]') || doc.body;

                if (oldTarget && newTarget) {
                    morph(oldTarget, newTarget);
                }

                if (push) {
                    history.pushState(null, '', url);
                }

                scanAndInit();
            })
            .catch(function(err) {
                console.error('SPA Navigation error:', err);
                window.location.href = url;
            });
    }

    document.addEventListener('click', function(e) {
        const link = e.target.closest('a');
        if (link && link.hasAttribute('gs-route')) {
            const url = link.getAttribute('href');
            if (url && !url.startsWith('#') && !url.startsWith('javascript:')) {
                e.preventDefault();
                navigate(url);
            }
        }
    });

    window.addEventListener('popstate', function() {
        navigate(window.location.pathname, false);
    });

    if (typeof MutationObserver !== 'undefined') {
        const observer = new MutationObserver(function(mutations) {
            let needsScan = false;
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
        version: '1.0.10',
        init:    scanAndInit,
        eval:    evaluate,
        store:   registerStore,
        directive: function(name, callback) { customDirectives[name] = callback; },
        magic:     function(name, callback) { customMagics[name] = callback; },
        component: function(name, definition) { componentRegistry[name] = definition; }
    };

    // Global Modal Helpers
    global.GoStack = {
        closeModal: function(id) {
            const el = document.getElementById('gs-modal-' + id);
            if (el) {
                if (el.hasAttribute('gs-transition')) {
                    toggleElement(el, false);
                } else {
                    el.classList.add('gs-hidden');
                }
            }
        },
        showModal: function(id) {
            const el = document.getElementById('gs-modal-' + id);
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
