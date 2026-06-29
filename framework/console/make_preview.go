package console

import (
	"encoding/json"
	"fmt"
	"github.com/charledeon77/gostack-framework/framework/ui"
	"html/template"
	"io"
	"net/http"
	"path/filepath"
	"strings"
)

// PreviewCommand spins up a local web server to display the component gallery.
type PreviewCommand struct{}

func (c *PreviewCommand) Name() string {
	return "ui:preview"
}

func (c *PreviewCommand) Description() string {
	return "Launch the local GoStack Component Gallery dashboard"
}

type ComponentInfo struct {
	Description  string   `json:"description"`
	Dependencies []string `json:"dependencies"`
	Files        []string `json:"files"`
}

type ComponentRegistry struct {
	Name       string                   `json:"name"`
	Version    string                   `json:"version"`
	Components map[string]ComponentInfo `json:"components"`
}

const dashboardHTML = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>GoStack Component Gallery</title>
    <style id="gostack-core-styles">
        {{ .CoreCSS }}
    </style>
    <style>
        /* Dashboard Layout Styles */
        body { margin: 0; font-family: sans-serif; background-color: #f1f5f9; color: #0f172a; }
        header { background-color: white; padding: 1.5rem 2rem; border-bottom: 1px solid #e2e8f0; display: flex; justify-content: space-between; align-items: center; }
        header h2 { margin: 0; }
        .container { max-width: 1200px; margin: 2rem auto; padding: 0 1rem; }
        .component-section { background: white; border-radius: 0.5rem; border: 1px solid #e2e8f0; margin-bottom: 2rem; overflow: hidden; box-shadow: 0 1px 3px 0 rgba(0,0,0,0.1); }
        .component-header { padding: 1.5rem; border-bottom: 1px solid #e2e8f0; background-color: #f8fafc; display: flex; justify-content: space-between; align-items: center; }
        .component-title { margin: 0; font-size: 1.25rem; font-weight: 600; text-transform: capitalize; }
        .install-cmd { background: #1e293b; color: #a5b4fc; padding: 0.5rem 1rem; border-radius: 0.25rem; font-family: monospace; font-size: 0.875rem; cursor: pointer; transition: background 0.2s; }
        .install-cmd:hover { background: #0f172a; }
        .preview-area { padding: 3rem; display: flex; justify-content: center; align-items: center; background-image: radial-gradient(#cbd5e1 1px, transparent 1px); background-size: 20px 20px; }
        .code-area { background: #f8fafc; padding: 1.5rem; border-top: 1px solid #e2e8f0; font-family: monospace; white-space: pre-wrap; font-size: 0.875rem; color: #334155; margin: 0; }
    </style>
</head>
<body>
    <header>
        <h2>GoStack UI Gallery</h2>
        <span style="color: #64748b; font-size: 0.875rem;">Running on :8081</span>
    </header>
    <div class="container">
        {{ if eq (len .Components) 0 }}
            <div style="text-align: center; padding: 4rem; background: white; border-radius: 0.5rem; border: 1px solid #e2e8f0;">
                <h3 style="margin-top: 0;">No components found</h3>
                <p style="color: #64748b;">Please ensure the <code>gostack_components</code> directory exists alongside your GoStack directory, and contains a <code>registry.json</code>.</p>
            </div>
        {{ end }}

        {{ range $name, $html := .Components }}
        <section class="component-section">
            <div class="component-header">
                <h3 class="component-title">{{ $name }}</h3>
                <div class="install-cmd" onclick="navigator.clipboard.writeText('gost add {{ $name }}'); alert('Copied to clipboard!')">
                    > gost add {{ $name }}
                </div>
            </div>
            
            <!-- The Magic Opt-in 'gs-css' is applied here to the container! -->
            <div class="preview-area" gs-css>
                {{ $html }}
            </div>

            <pre class="code-area">{{ $html }}</pre>
        </section>
        {{ end }}
    </div>
</body>
</html>
`

func (c *PreviewCommand) Execute(args []string) error {
	port := "8081"

	fmt.Printf("🎨 Fetching component registry from GitHub...\n")
	registryURL := "https://raw.githubusercontent.com/Charledeon77/gostack-components/main/registry.json"
	resp, err := http.Get(registryURL)
	if err != nil {
		return fmt.Errorf("failed to fetch registry: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to fetch registry: HTTP %d", resp.StatusCode)
	}

	var registry ComponentRegistry
	if err := json.NewDecoder(resp.Body).Decode(&registry); err != nil {
		return fmt.Errorf("failed to parse registry JSON: %w", err)
	}

	componentsData := make(map[string]template.HTML)
	for name, comp := range registry.Components {
		for _, file := range comp.Files {
			if strings.HasSuffix(file, ".html") {
				fileURL := "https://raw.githubusercontent.com/Charledeon77/gostack-components/main/" + file
				fmt.Printf("📥 Fetching preview for component '%s' (%s)...\n", name, filepath.Base(file))
				fResp, err := http.Get(fileURL)
				if err != nil {
					fmt.Printf("⚠️ Warning: failed to fetch preview file for '%s': %v\n", name, err)
					continue
				}
				defer fResp.Body.Close()

				if fResp.StatusCode == http.StatusOK {
					htmlBytes, err := io.ReadAll(fResp.Body)
					if err == nil {
						componentsData[name] = template.HTML(string(htmlBytes))
					}
				}
			}
		}
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		tmpl, err := template.New("dashboard").Parse(dashboardHTML)
		if err != nil {
			http.Error(w, "Failed to parse dashboard template", 500)
			return
		}

		data := struct {
			CoreCSS    template.CSS
			Components map[string]template.HTML
		}{
			CoreCSS:    template.CSS(ui.CoreBaseCSS),
			Components: componentsData,
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.Execute(w, data); err != nil {
			fmt.Printf("Template execution error: %v\n", err)
		}
	})

	fmt.Printf("🎨 Starting GoStack Component Gallery...\n")
	fmt.Printf("👉 View the gallery at: http://localhost:%s\n", port)
	fmt.Printf("Press Ctrl+C to stop.\n")

	return http.ListenAndServe(":"+port, nil)
}
