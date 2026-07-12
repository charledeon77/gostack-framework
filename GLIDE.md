# Glide — GoStack Reactive Runtime

**Glide** is GoStack''s built-in browser-side reactivity engine. It ships embedded inside the GoStack binary — no CDN link, no build step, no `npm install`. It works by scanning your HTML for `gs-*` attributes and binding reactive state to the DOM at runtime.

> Think of it as a purpose-built Alpine.js, designed from the ground up for GoStack components.

---

## Table of Contents

1. [How It Works](#how-it-works)
2. [Declaring a Component — gs-data](#declaring-a-component--gs-data)
3. [Rendering — gs-text, gs-html](#rendering--gs-text-gs-html)
4. [Conditionals — gs-if, gs-show](#conditionals--gs-if-gs-show)
5. [Lists — gs-each](#lists--gs-each)
6. [FLIP Layout Animation — gs-animate](#flip-layout-animation--gs-animate)
7. [Two-Way Binding — gs-model](#two-way-binding--gs-model)
8. [Component Sync — gs-modelable](#component-sync--gs-modelable)
9. [Dynamic Attributes — :attr / gs-bind:attr](#dynamic-attributes--attr--gs-bindattr)
10. [Events — @event / gs-on:event](#events--event--gs-onevent)
11. [Event Modifiers](#event-modifiers)
12. [Lifecycle — gs-init](#lifecycle--gs-init)
13. [Reactive Side Effects — gs-effect](#reactive-side-effects--gs-effect)
14. [Element References — gs-ref](#element-references--gs-ref)
15. [Intersection Observer — gs-intersect](#intersection-observer--gs-intersect)
16. [Transitions — gs-transition](#transitions--gs-transition)
17. [Subtree Bypass — gs-ignore](#subtree-bypass--gs-ignore)
18. [Render Cloak — gs-cloak](#render-cloak--gs-cloak)
19. [Teleportation — gs-teleport](#teleportation--gs-teleport)
20. [Form Validation — gs-validate](#form-validation--gs-validate)
21. [Magic Properties](#magic-properties)
22. [Watchers — $watch](#watchers--watch)
23. [Render Coordination — $nextTick](#render-coordination--nexttick)
24. [Global Stores — Glide.store / $store](#global-stores--glidestore--store)
25. [SPA Router — gs-route](#spa-router--gs-route)
26. [Modal Helpers](#modal-helpers)
27. [Persistence — persist() / $persist](#persistence--persist--persist)
28. [Plugin System — Glide.directive, Glide.magic](#plugin-system--glidedirective-glidemagic)
29. [Client Components — Glide.component](#client-components--glidecomponent)
30. [Public API — Glide.*](#public-api--glide)
31. [Complete Example](#complete-example)

---

## How It Works

Glide initialises automatically when the page loads. It scans every element that has a `gs-data` attribute and turns it into a **reactive scope** (a component). All `gs-*` directives inside that element are bound to the scope''s state.

```html
<div gs-data="{ name: ''World'' }">
    <h1 gs-text="''Hello, '' + name"></h1>
</div>
```

Each `gs-data` block is fully isolated. State does not bleed between sibling or nested components. Glide is progressive-enhancement — if JavaScript is unavailable, the server-rendered HTML is still displayed correctly.

---

## Declaring a Component — `gs-data`

`gs-data` accepts any valid JavaScript object expression.

```html
<div gs-data="{
    count: 0,
    user: { name: ''Ada'', role: ''admin'' },
    tags: [''go'', ''webdev'']
}">
    ...
</div>
```

**Nested components** are fully supported. An inner `gs-data` creates a completely independent scope — it does not inherit from the outer one.

```html
<div gs-data="{ outer: ''A'' }">
    <div gs-data="{ inner: ''B'' }">
        <!-- inner can only see ''inner'', not ''outer'' -->
    </div>
</div>
```

---

## Rendering — `gs-text`, `gs-html`

### `gs-text`
Sets the element''s text content to the evaluated expression. HTML is escaped automatically.

```html
<span gs-text="user.name"></span>
<span gs-text="''Welcome, '' + user.name + ''!''"></span>
<span gs-text="count > 0 ? count + '' items'' : ''Empty''"></span>
```

### `gs-html`
Sets the element''s inner HTML. Script tags, inline event handlers, and `javascript:` URLs are automatically stripped.

```html
<div gs-html="richContent"></div>
```

---

## Conditionals — `gs-if`, `gs-show`

### `gs-if`
**Structurally mounts or unmounts** the element. When false, the element is completely removed from the DOM — no hidden node remains.

```html
<div gs-data="{ loggedIn: false }">
    <nav gs-if="loggedIn">Dashboard nav</nav>
    <p gs-if="!loggedIn">Please log in.</p>
</div>
```

Combine with `gs-transition` to animate the mount/unmount.

### `gs-show`
**Toggles visibility** via `display: none`. The element always stays in the DOM.

```html
<div gs-show="isLoading">Loading...</div>
```

Use `gs-show` when the element is expensive to re-create. Use `gs-if` when you want it truly absent (e.g. for security-sensitive UI).

---

## Lists — `gs-each`

Renders a list by cloning the element for each item in an array.

```html
<div gs-data="{ products: [
    { id: 1, name: ''Widget'', price: 9.99 },
    { id: 2, name: ''Gadget'', price: 24.99 }
] }">
    <div gs-each="product in products" :key="product.id">
        <span gs-text="product.name"></span>
        <span gs-text="''$'' + product.price"></span>
    </div>
</div>
```

### Index access

```html
<li gs-each="item, index in items">
    <span gs-text="(index + 1) + ''. '' + item.name"></span>
</li>
```

### `:key` — Keyed Reconciliation

Always provide `:key` on reorderable or filterable lists. Glide uses it to match existing DOM nodes to new list positions, preserving **focus, cursor position, and input values** across re-renders.

```html
<input gs-each="item in items" :key="item.id" gs-model="item.value">
```

### Mutating the list

All standard array mutation methods trigger a reactive update automatically:

```html
<button @click="items.push({ id: Date.now(), name: ''New'' })">Add</button>
<button @click="items.splice(0, 1)">Remove First</button>
<button @click="items.sort((a, b) => a.name.localeCompare(b.name))">Sort</button>
```

Supported mutating methods: `push`, `pop`, `shift`, `unshift`, `splice`, `sort`, `reverse`.

---

## Two-Way Binding — `gs-model`

Binds an input''s value to a state property bidirectionally.

```html
<div gs-data="{ email: '''', agreed: false, role: ''viewer'' }">
    <input type="text"     gs-model="email">
    <input type="checkbox" gs-model="agreed">
    <select gs-model="role">
        <option value="viewer">Viewer</option>
        <option value="editor">Editor</option>
        <option value="admin">Admin</option>
    </select>
    <input type="range"  gs-model="volume" min="0" max="100">
    <input type="text"   gs-model="user.name">
</div>
```

---

## Dynamic Attributes — `:attr` / `gs-bind:attr`

Binds any HTML attribute dynamically. `:attr` is the shorthand for `gs-bind:attr`.

```html
<button :disabled="count === 0">Submit</button>
<a :href="''/user/'' + userId">Profile</a>
<img :src="imageUrl" :alt="imageAlt">
```

### `:class`

Accepts an object (condition map), array, or string.

```html
<div :class="{ active: isActive, ''text-red'': hasError }"></div>
<div :class="[''base'', isActive ? ''active'' : '''']"></div>
```

### `:style`

Accepts an object of camelCase or kebab-case CSS properties.

```html
<div :style="{ backgroundColor: brandColor, fontSize: size + ''px'' }"></div>
```

---

## Events — `@event` / `gs-on:event`

Binds DOM event listeners. `@event` is the shorthand for `gs-on:event`.

```html
<button @click="count++">Increment</button>
<form @submit.prevent="handleSubmit()">...</form>
<input @keyup.enter="search()">
<input @keydown.escape="clearSearch()">
```

Inside event expressions, `$event` gives you the native browser event and `$el` gives you the element.

---

## Event Modifiers

Modifiers are chained after the event name with a dot: `@click.stop.prevent`.

### Flow Modifiers

| Modifier | Behaviour |
|---|---|
| `.stop` | Calls `event.stopPropagation()` |
| `.prevent` | Calls `event.preventDefault()` |
| `.self` | Only fires if `event.target === element` |
| `.once` | Listener auto-removes after first fire |

### Target Modifiers

| Modifier | Behaviour |
|---|---|
| `.window` | Binds to `window` |
| `.document` | Binds to `document` |
| `.outside` | Fires only when clicking **outside** the element |

```html
<div @click.outside="open = false" gs-if="open">Dropdown</div>
<div @keydown.escape.window="closeModal()">...</div>
```

### Rate Modifiers

| Modifier | Behaviour |
|---|---|
| `.debounce` | Waits 250ms for inactivity before firing |
| `.debounce.500ms` | Custom debounce delay |
| `.throttle` | Fires at most once per 250ms |
| `.throttle.1000ms` | Custom throttle interval |

```html
<input @input.debounce.300ms="search(query)">
<div @scroll.window.throttle.100ms="onScroll()"></div>
```

### Keyboard Key Modifiers

`.enter` · `.escape` / `.esc` · `.space` · `.tab` · `.delete`

### System Key Modifiers

`.ctrl` · `.shift` · `.alt` · `.meta`

```html
<input @keydown.ctrl.enter="submitForm()">
```

### Performance Modifiers

`.passive` · `.capture`

---

## Lifecycle — `gs-init`

Code in `gs-init` runs once after Glide initialises the component. Use it for setup logic, API calls, or registering watchers.

```html
<div gs-data="{ users: [], loading: true }"
     gs-init="
         fetch(''/api/users'')
             .then(r => r.json())
             .then(data => { users = data; loading = false; });
     ">
    <div gs-if="loading">Loading...</div>
    <ul gs-if="!loading">
        <li gs-each="user in users" gs-text="user.name"></li>
    </ul>
</div>
```

`gs-init` can also be placed on child elements:

```html
<div gs-data="{ value: 0 }">
    <canvas gs-init="initChart($el)"></canvas>
</div>
```

---

## Reactive Side Effects — `gs-effect`

`gs-effect` evaluates an expression on every render pass. Use it to run imperative code in response to state changes.

```html
<div gs-data="{ query: ''''" gs-effect="document.title = query ? ''Results: '' + query : ''Search''">
    <input gs-model="query">
</div>
```

---

## Element References — `gs-ref`

`gs-ref` registers a DOM element into the `$refs` object.

```html
<div gs-data="{ content: '' ''}">
    <input gs-ref="inputEl" type="text">
    <button @click="$refs.inputEl.focus()">Focus</button>
</div>
```

---

## Intersection Observer — `gs-intersect`

Fires an expression when the element enters the viewport.

```html
<div gs-data="{ visible: false }"
     gs-intersect="visible = true"
     :class="{ ''fade-in'': visible }">
    Animates when scrolled into view.
</div>
```

---

## Transitions — `gs-transition`

Add `gs-transition` to any element controlled by `gs-if` or `gs-show` to animate its entrance and exit.

```html
<div gs-if="show" gs-transition="fade">Fade in/out</div>
<div gs-if="show" gs-transition="slide">Slide down/up</div>
<div gs-if="show" gs-transition="scale">Scale in/out</div>
<div gs-if="show" gs-transition="blur">Blur in/out</div>
<div gs-if="show" gs-transition="fly">Fly in/out</div>
<div gs-if="show" gs-transition>Default (scale)</div>
```

### Presets

| Name | Effect | Duration |
|---|---|---|
| `fade` | Opacity only | 250ms |
| `slide` | `translateY(-10px)` + opacity | 260ms |
| `scale` | `scale(0.9→1)` + opacity | 220ms |
| `blur` | `blur(6px→0)` + opacity | 280ms |
| `fly` | `translate(20px, -20px)` + opacity | 300ms |

### Custom Timing

Override a preset''s timing per element:

```html
<div gs-if="show" gs-transition="fade" gs-transition:duration="500">Slower fade</div>
<div gs-if="show" gs-transition="slide" gs-transition:delay="100ms">Delayed slide</div>
<div gs-if="show" gs-transition="scale" gs-transition:easing="ease-in-out">Custom easing</div>
```

| Override | Format | Description |
|---|---|---|
| `gs-transition:duration` | Milliseconds (e.g. `500`) | Overrides preset duration |
| `gs-transition:delay` | CSS time value (e.g. `100ms`) | Adds transition delay |
| `gs-transition:easing` | CSS easing function | Replaces preset easing |

### Loop transitions

Add `gs-transition` to a `gs-each` template. New items animate in; removed items animate out before being destroyed.

```html
<div gs-each="item in items" :key="item.id" gs-transition="slide">
    <span gs-text="item.name"></span>
</div>
```

---

## Magic Properties

| Magic | Type | Description |
|---|---|---|
| `$el` | `Element` | The current DOM element |
| `$event` | `Event` | The triggering browser event |
| `$refs` | `Object` | Map of elements registered with `gs-ref` |
| `$store` | `Object` | Global reactive stores |
| `$watch` | `Function` | Register a state watcher |
| `$nextTick` | `Function` | Defer code until after the next render |
| `$dispatch` | `Function` | Dispatch a custom DOM event that bubbles up |
| `$root` | `Element` | The component''s root `gs-data` element |
| `$data` | `Object` | The raw (unwrapped) state object — safe for third-party APIs |
| `$id` | `Function` | Generate a unique component-scoped ID: `$id(''label'')` → `glide-1-label` |
| `$persist` | `Function` | Alias for `persist()` available in event handlers and init |
| `$errors` | `Object` | Map of validation errors populated by `gs-validate` |

### `$dispatch`

```html
<!-- Child fires an event -->
<div gs-data="{ value: '' '' }">
    <input gs-model="value" @input="$dispatch(''value-changed'', { value: value })">
</div>

<!-- Parent listens -->
<div gs-data="{}" @value-changed.window="console.log($event.detail.value)">
    ...
</div>
```

---

## Watchers — `$watch`

`$watch(expression, callback)` calls `callback(newValue, oldValue)` every time the watched expression changes.

```html
<div gs-data="{ query: '' '', results: [] }"
     gs-init="
         $watch(''query'', (newVal) => {
             if (newVal.length > 2) {
                 fetch(''/search?q='' + newVal)
                     .then(r => r.json())
                     .then(data => results = data);
             }
         });
     ">
    <input gs-model="query">
    <ul>
        <li gs-each="r in results" gs-text="r.title"></li>
    </ul>
</div>
```

Errors thrown inside a watcher callback are caught and logged to the console without interrupting other watchers or the render cycle.

---

## Render Coordination — `$nextTick`

`$nextTick(callback)` defers the callback until after Glide has finished applying the current render pass to the DOM.

```html
<div gs-data="{ message: ''Initial'' }">
    <p gs-ref="output" gs-text="message"></p>
    <button @click="
        message = ''Updated'';
        // DOM still shows ''Initial'' here
        $nextTick(() => {
            // DOM has now been updated
            console.log($refs.output.textContent); // ''Updated''
        });
    ">Update</button>
</div>
```

---

## Global Stores — `Glide.store` / `$store`

Global stores hold reactive state shared across multiple independent components. When any component mutates the store, all components that read it re-render.

### Registering a store

Register before the page boots (in a `<script>` tag after the Glide runtime injection):

```html
<script>
    Glide.store(''cart'', { count: 0, items: [] });
    Glide.store(''theme'', { dark: true });
</script>
```

### Accessing a store

```html
<!-- Component A: writes -->
<div gs-data="{}">
    <button @click="$store.cart.count++">Add to Cart</button>
</div>

<!-- Component B: reads — updates automatically -->
<div gs-data="{}">
    <span gs-text="$store.cart.count + '' items''"></span>
</div>
```

### Reading from JavaScript

```js
const count = Glide.store(''cart'').count;
```

---

## SPA Router — `gs-route`

The Glide router intercepts clicks on `gs-route` links and performs a **morphing navigation** — it fetches the target page and surgically updates only the `gs-route-target` container, without a full browser reload.

### Setup

```html
<!-- Mark the region that morphs on navigation -->
<div gs-route-target>
    ...
</div>

<!-- Mark links that use SPA navigation -->
<a href="/dashboard" gs-route>Dashboard</a>
<a href="/settings" gs-route>Settings</a>
```

### How it works

1. User clicks a `gs-route` link.
2. Glide intercepts the click and fetches the target URL.
3. The fetched HTML is parsed and its `gs-route-target` is compared to the current one.
4. Glide morphs the DOM in-place — only changed nodes are updated.
5. New reactive scopes in the morphed content are automatically initialised.
6. Browser history is updated via `history.pushState`.

Back and forward navigation (`popstate`) is handled automatically.

### Fallback

If the fetch fails for any reason, Glide falls back to a full browser navigation. The user always arrives at the destination.

---

## Modal Helpers

```js
GoStack.showModal(''confirm'');   // Shows #gs-modal-confirm
GoStack.closeModal(''confirm'');  // Hides #gs-modal-confirm
```

```html
<div id="gs-modal-confirm" gs-transition="scale" class="gs-hidden">
    <p>Are you sure?</p>
    <button @click="GoStack.closeModal(''confirm'')">Cancel</button>
    <button @click="confirm(); GoStack.closeModal(''confirm'')">Yes</button>
</div>
```

If the modal carries `gs-transition`, the show/hide is animated automatically.

---

## Persistence — `persist()` / `$persist`

`persist(key, defaultValue)` syncs a state property to `localStorage` automatically — loaded on first render, saved on every update.

```html
<div gs-data="{
    theme: persist(''theme'', ''light''),
    sidebarOpen: persist(''sidebar'', true)
}">
    <button @click="theme = theme === ''light'' ? ''dark'' : ''light''">
        Toggle Theme
    </button>
</div>
```

`$persist` is also available as a magic property inside event handlers and `gs-init`:

```html
<div gs-data="{}" gs-init="$persist(''visitCount'', 0)">
    <button @click="$persist(''visitCount'', visitCount + 1)">Visited</button>
</div>
```

Stored under the key `gs_<key>` in `localStorage`.

---

## Subtree Bypass — `gs-ignore`

Tells Glide to skip an element and all its descendants. Use this to protect third-party widgets (charts, maps, WYSIWYG editors) from Glide''s DOM processing.

```html
<div gs-data="{ loaded: true }">
    <h2 gs-text="''Dashboard''"></h2>
    <div gs-ignore>
        <!-- Google Maps, Chart.js, or any third-party widget -->
        <div id="external-chart"></div>
    </div>
</div>
```

---

## Render Cloak — `gs-cloak`

Prevents flash of unrendered content (FOUC). Elements with `gs-cloak` are hidden via CSS until Glide finishes initialising them, then the attribute is automatically removed.

```html
<div gs-data="{ name: ''World'' }" gs-cloak>
    <h1 gs-text="''Hello, '' + name"></h1>
</div>
```

No extra CSS required — Glide injects `[gs-cloak] { display: none !important; }` automatically.

---

## Teleportation — `gs-teleport`

Moves an element to a different location in the DOM while preserving its reactive scope.

```html
<div gs-data="{ showTooltip: false }">
    <button @click="showTooltip = !showTooltip">Show Tooltip</button>
    <div gs-teleport="body" gs-if="showTooltip" gs-transition="fade">
        <div class="tooltip">I am rendered at the end of body!</div>
    </div>
</div>
```

The element is physically relocated to the target selector (e.g. `body`), escaping any `overflow: hidden` ancestors, but its bindings, events, and reactive updates continue working.

---

## Form Validation — `gs-validate`

Automatic HTML5 form validation with error binding and AJAX submission.

```html
<div gs-data="{ submitted: false }">
    <form gs-validate="
        fetch(''/api/signup'', { method: ''POST'', body: $formData })
            .then(function() { submitted = true; });
    ">
        <input name="email" type="email" required placeholder="Email">
        <span gs-if="$errors.email" gs-text="$errors.email" style="color:red"></span>

        <input name="password" type="password" required minlength="8" placeholder="Password">
        <span gs-if="$errors.password" gs-text="$errors.password" style="color:red"></span>

        <button type="submit">Sign Up</button>
    </form>
    <p gs-if="submitted">Success!</p>
</div>
```

When submitted, Glide runs HTML5 `checkValidity()` on every named input. Invalid fields populate `$errors` (keyed by input `name`). If all pass, the `gs-validate` expression runs with `$formData` (a `FormData` instance) and `$form` (a plain object) injected.

---

## FLIP Layout Animation — `gs-animate`

Add `gs-animate="flip"` to a `gs-each` list for smooth FLIP (First-Last-Invert-Play) layout animations when items are reordered.

```html
<div gs-each="item in items" :key="item.id" gs-animate="flip">
    <span gs-text="item.name"></span>
</div>
<button @click="items.sort((a, b) => a.name.localeCompare(b.name))">Sort</button>
```

When the list mutates, Glide records each keyed element''s position, reconciles the DOM, then animates each element from its old position to its new position using CSS transforms.

---

## Component Sync — `gs-modelable`

Synchronises a child component''s property bidirectionally with a parent scope''s property.

```html
<!-- Parent -->
<div gs-data="{ parentColor: ''red'' }">
    <p gs-text="''Parent sees: '' + parentColor"></p>
    <!-- Child syncs ''localColor'' with parent''s ''parentColor'' -->
    <div gs-data="{ localColor: '''' }" gs-modelable="localColor" gs-model="parentColor">
        <input gs-model="localColor" placeholder="Type a color">
    </div>
</div>
```

When the child''s `localColor` changes, the parent''s `parentColor` updates automatically.

---

## Plugin System — `Glide.directive`, `Glide.magic`

### Custom Directives

Register your own `gs-*` directives:

```js
Glide.directive(''tooltip'', function(el, utils) {
    const text = utils.evaluate();
    el.title = text;
    el.style.cursor = ''help'';
});
```

```html
<span gs-tooltip="''Click to edit''">Edit</span>
```

The callback receives the element and a utilities object with `expression` (raw attribute string), `evaluate(expr?)` (evaluate against scope), and `scope` (the reactive data).

### Custom Magic Properties

Register `$`-prefixed magic properties available in every scope:

```js
Glide.magic(''now'', function(el, scope) {
    return new Date().toLocaleTimeString();
});
```

```html
<span gs-text="$now"></span>
```

---

## Client Components — `Glide.component`

Register reusable client-side components with custom tag names:

```js
Glide.component(''gs-counter'', {
    data: function(props) {
        return { count: parseInt(props.start || 0) };
    },
    template: ''<button @click="count++" gs-text="count"></button>''
});
```

```html
<gs-counter start="5"></gs-counter>
<gs-counter start="10"></gs-counter>
```

Each instance gets its own isolated reactive scope. Props are forwarded from the element''s HTML attributes to the `data` function.

---

## Public API — `Glide.*`

| Property / Method | Description |
|---|---|
| `Glide.version` | Current Glide version string (`''1.0.10''`) |
| `Glide.store(name)` | Read the store named `name` |
| `Glide.store(name, value)` | Register a new global store with initial `value` |
| `Glide.init()` | Re-scan the document for uninitialised `gs-data` components |
| `Glide.eval(expr, scope)` | Evaluate a Glide expression string against a scope object |
| `Glide.directive(name, fn)` | Register a custom `gs-<name>` directive |
| `Glide.magic(name, fn)` | Register a custom `$<name>` magic property |
| `Glide.component(tag, def)` | Register a reusable component with a custom tag name |

---

## Complete Example

```html
<div gs-data="{
    tasks: [
        { id: 1, text: ''Design the schema'', done: true },
        { id: 2, text: ''Build the API'',     done: false },
        { id: 3, text: ''Write the docs'',    done: false }
    ],
    newTask: '''',
    filter: ''all'',
    get pending() {
        return this.tasks.filter(t => !t.done).length;
    }
}"
gs-init="$watch(''tasks'', () => { document.title = pending + '' tasks remaining''; })">

    <h2 gs-text="pending + '' tasks remaining''"></h2>

    <!-- Add task -->
    <form @submit.prevent="
        if (newTask.trim()) {
            tasks.push({ id: Date.now(), text: newTask.trim(), done: false });
            newTask = '''';
        }
    ">
        <input gs-model="newTask" placeholder="New task..." @keyup.escape="newTask = ''''">
        <button type="submit">Add</button>
    </form>

    <!-- Filter buttons -->
    <div>
        <button @click="filter = ''all''"    :class="{ active: filter === ''all'' }">All</button>
        <button @click="filter = ''active''" :class="{ active: filter === ''active'' }">Active</button>
        <button @click="filter = ''done''"   :class="{ active: filter === ''done'' }">Done</button>
    </div>

    <!-- Task list -->
    <ul>
        <li gs-each="task in tasks" :key="task.id" gs-transition="slide"
            gs-if="filter === ''all'' || (filter === ''done'' ? task.done : !task.done)">
            <input type="checkbox" gs-model="task.done">
            <span gs-text="task.text" :style="{ textDecoration: task.done ? ''line-through'' : ''none'' }"></span>
            <button @click="tasks.splice(tasks.indexOf(task), 1)">x</button>
        </li>
    </ul>

</div>
```

---

*Glide is part of the GoStack framework. It is embedded in the GoStack binary — no external dependencies, no build step required.*
