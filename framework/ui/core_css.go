package ui

// CoreBaseCSS provides the global semantic styles for elements opting-in via [gs-css].
const CoreBaseCSS = `
/* GoStack Core Semantic Base (Opt-In) */
[gs-css], [gs-css] * {
    box-sizing: border-box;
}

[gs-css] {
    --gs-primary: #3b82f6;
    --gs-primary-hover: #2563eb;
    --gs-bg: #ffffff;
    --gs-surface: #f8fafc;
    --gs-text: #0f172a;
    --gs-text-muted: #64748b;
    --gs-border: #e2e8f0;
    --gs-radius: 0.5rem;
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
    color: var(--gs-text);
}

[gs-css] button, button[gs-css] {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    background-color: var(--gs-primary);
    color: white;
    border: none;
    border-radius: var(--gs-radius);
    padding: 0.5rem 1rem;
    font-size: 0.875rem;
    font-weight: 500;
    cursor: pointer;
    transition: background-color 0.2s, transform 0.1s;
}

[gs-css] button:hover, button[gs-css]:hover {
    background-color: var(--gs-primary-hover);
}

[gs-css] button:active, button[gs-css]:active {
    transform: translateY(1px);
}

[gs-css] input[type="text"], [gs-css] input[type="email"], [gs-css] input[type="password"], input[gs-css] {
    width: 100%;
    padding: 0.5rem 0.75rem;
    border: 1px solid var(--gs-border);
    border-radius: var(--gs-radius);
    font-size: 0.875rem;
    color: var(--gs-text);
    background-color: var(--gs-bg);
    transition: border-color 0.2s, box-shadow 0.2s;
}

[gs-css] input:focus, input[gs-css]:focus {
    outline: none;
    border-color: var(--gs-primary);
    box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.15);
}

[gs-css] article, article[gs-css] {
    background-color: var(--gs-bg);
    border: 1px solid var(--gs-border);
    border-radius: var(--gs-radius);
    padding: 1.5rem;
    box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.1), 0 2px 4px -1px rgba(0, 0, 0, 0.06);
}

[gs-css] label, label[gs-css] {
    display: block;
    font-size: 0.875rem;
    font-weight: 500;
    color: var(--gs-text);
    margin-bottom: 0.375rem;
}

[gs-css] h1, [gs-css] h2, [gs-css] h3, h1[gs-css], h2[gs-css], h3[gs-css] {
    margin-top: 0;
    margin-bottom: 1rem;
    font-weight: 600;
    line-height: 1.25;
}

[gs-css] p, p[gs-css] {
    margin-top: 0;
    margin-bottom: 1rem;
    line-height: 1.5;
}
`
