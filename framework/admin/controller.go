package admin

import (
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"fmt"
	"html"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/charledeon77/gostack-framework/framework/contract"
)

const adminCSRFCookieName = "admin_csrf"

// adminCSS is the shared stylesheet served at /admin/assets/admin.css.
// All admin HTML templates reference it via <link> instead of inline <style> blocks (D44).
const adminCSS = `
/* GoStack Admin Panel — system-font stylesheet (D44: CSP-compliant, no CDN fonts) */
:root {
	--bg-main: #0b0f19;
	--bg-card: #151d30;
	--border-color: #24324f;
	--text-primary: #f8fafc;
	--text-secondary: #94a3b8;
	--primary-cyan: #06b6d4;
	--primary-cyan-hover: #22d3ee;
	--accent-yellow: #f59e0b;
	--accent-red: #ef4444;
	--accent-green: #10b981;
}
body {
	font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
	background: var(--bg-main);
	color: var(--text-primary);
	margin: 0;
	padding: 0;
	-webkit-font-smoothing: antialiased;
}
.header {
	background: var(--bg-card);
	padding: 1.25rem 2rem;
	border-bottom: 1px solid var(--border-color);
	display: flex;
	align-items: center;
	justify-content: space-between;
}
.logo-area { display: flex; align-items: center; gap: 0.75rem; }
.logo {
	font-size: 1.35rem;
	font-weight: 700;
	background: linear-gradient(135deg, var(--primary-cyan), var(--primary-cyan-hover));
	-webkit-background-clip: text;
	-webkit-text-fill-color: transparent;
	display: flex;
	align-items: center;
	gap: 0.5rem;
}
.driver-badge {
	font-size: 0.75rem;
	font-weight: 600;
	text-transform: uppercase;
	letter-spacing: 0.05em;
	padding: 0.25rem 0.6rem;
	border-radius: 9999px;
	background: rgba(6, 182, 212, 0.15);
	color: var(--primary-cyan);
	border: 1px solid rgba(6, 182, 212, 0.3);
}
.back-btn, .back {
	color: var(--text-secondary);
	text-decoration: none;
	font-size: 0.9rem;
	font-weight: 500;
	display: flex;
	align-items: center;
	gap: 0.4rem;
}
.back-btn:hover, .back:hover { color: var(--text-primary); }
.container { padding: 2.5rem 2rem; max-width: 1280px; margin: 0 auto; }
h1 { font-size: 1.85rem; font-weight: 700; margin-bottom: 2rem; letter-spacing: -0.02em; color: #38bdf8; }
.stats-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(280px, 1fr)); gap: 1.5rem; margin-bottom: 2.5rem; }
.card {
	background: var(--bg-card);
	border: 1px solid var(--border-color);
	border-radius: 12px;
	padding: 1.5rem;
	position: relative;
	overflow: hidden;
	transition: transform 0.2s ease, border-color 0.2s ease;
	text-align: center;
}
.card:hover { transform: translateY(-2px); border-color: rgba(6, 182, 212, 0.4); }
.card::before { content: ''; position: absolute; top: 0; left: 0; width: 4px; height: 100%; }
.card-pending::before { background: var(--primary-cyan); }
.card-delayed::before { background: var(--accent-yellow); }
.card-failed::before { background: var(--accent-red); }
.card-label { font-size: 0.85rem; font-weight: 600; text-transform: uppercase; letter-spacing: 0.05em; color: var(--text-secondary); margin-bottom: 0.5rem; }
.card-value { font-size: 2.5rem; font-weight: 700; letter-spacing: -0.03em; }
.section-title { font-size: 1.25rem; font-weight: 600; margin-bottom: 1.25rem; color: var(--text-primary); }
.table-container { background: var(--bg-card); border: 1px solid var(--border-color); border-radius: 12px; overflow: hidden; }
table { width: 100%%; border-collapse: collapse; text-align: left; }
th, td { padding: 1rem 1.25rem; border-bottom: 1px solid var(--border-color); font-size: 0.9rem; }
th { background: rgba(15, 23, 42, 0.4); color: var(--text-secondary); font-weight: 600; text-transform: uppercase; font-size: 0.75rem; letter-spacing: 0.05em; }
tr:last-child td { border-bottom: none; }
.empty-state { text-align: center; color: var(--text-secondary); padding: 4rem 2rem; font-size: 0.95rem; }
td a { color: #38bdf8; text-decoration: none; margin-right: 0.5rem; }
td a:hover { text-decoration: underline; }
.inline-form { display: inline; }
.btn-delete-link { background: none; border: none; color: #ef4444; cursor: pointer; padding: 0; font: inherit; text-decoration: underline; }
.payload-box {
	background: rgba(15, 23, 42, 0.6);
	border: 1px solid var(--border-color);
	border-radius: 6px;
	padding: 0.35rem 0.6rem;
	font-family: monospace;
	font-size: 0.8rem;
	color: var(--primary-cyan);
	max-width: 320px;
	display: inline-block;
	white-space: nowrap;
	overflow: hidden;
	text-overflow: ellipsis;
}
.badge { font-size: 0.75rem; font-weight: 600; padding: 0.25rem 0.5rem; border-radius: 6px; }
.badge-attempt { background: rgba(245, 158, 11, 0.1); color: var(--accent-yellow); border: 1px solid rgba(245, 158, 11, 0.2); }
.error-msg { color: #fca5a5; font-family: monospace; font-size: 0.825rem; }
.action-buttons { display: flex; gap: 0.5rem; }
.btn-action { padding: 0.4rem 0.75rem; font-size: 0.8rem; font-weight: 600; border-radius: 6px; text-decoration: none; text-align: center; transition: background-color 0.2s ease, color 0.2s ease; }
.btn-retry { background: rgba(6, 182, 212, 0.1); color: var(--primary-cyan); border: 1px solid rgba(6, 182, 212, 0.2); }
.btn-retry:hover { background: var(--primary-cyan); color: var(--bg-main); }
.btn-delete { background: rgba(239, 68, 68, 0.1); color: var(--accent-red); border: 1px solid rgba(239, 68, 68, 0.2); }
.btn-delete:hover { background: var(--accent-red); color: var(--text-primary); }
th.col-name { width: 20%%; }
th.col-payload { width: 25%%; }
th.col-attempts { width: 15%%; }
th.col-error { width: 25%%; }
th.col-actions { width: 15%%; }
.actions { margin-bottom: 1.5rem; }
.btn { display: inline-block; padding: 0.6rem 1.5rem; border-radius: 6px; background: #38bdf8; color: #0f172a; font-weight: 700; border: none; cursor: pointer; font-size: 1rem; text-decoration: none; }
.btn:hover { background: #7dd3fc; }
.field { margin-bottom: 1rem; }
label { display: block; font-size: 0.85rem; color: var(--text-secondary); margin-bottom: 0.3rem; text-transform: capitalize; }
input { width: 100%%; padding: 0.6rem 0.8rem; background: #1e293b; border: 1px solid #334155; border-radius: 6px; color: #e2e8f0; font-size: 1rem; box-sizing: border-box; }
input:focus { outline: none; border-color: #38bdf8; }
code { background: #0b0f19; color: #38bdf8; padding: 0.2rem 0.4rem; border-radius: 4px; font-family: monospace; }
.code-box { text-align: left; background: #0b0f19; padding: 1rem; border-radius: 8px; border: 1px solid var(--border-color); }
`

func getOrSetAdminCSRF(w http.ResponseWriter, r *http.Request) (string, error) {
	cookie, err := r.Cookie(adminCSRFCookieName)
	if err == nil && cookie.Value != "" {
		return cookie.Value, nil
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("admin: failed to generate secure CSRF token: %w", err)
	}
	token := hex.EncodeToString(b)
	http.SetCookie(w, &http.Cookie{
		Name:     adminCSRFCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
	})
	return token, nil
}

func verifyAdminCSRF(r *http.Request) bool {
	cookie, err := r.Cookie(adminCSRFCookieName)
	if err != nil || cookie.Value == "" {
		return false
	}
	submitted := r.FormValue("_csrf")
	if submitted == "" {
		submitted = r.Header.Get("X-CSRF-Token")
	}
	if submitted == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(submitted)) == 1
}

func isPostgresDB(db *sql.DB) bool {
	if db != nil && db.Driver() != nil {
		t := strings.ToLower(fmt.Sprintf("%T", db.Driver()))
		return strings.Contains(t, "postgres") || strings.Contains(t, "pq") || strings.Contains(t, "pgx")
	}
	return false
}

func sqlPlaceholder(isPg bool, idx int) string {
	if isPg {
		return fmt.Sprintf("$%d", idx)
	}
	return "?"
}

// Mount registers all admin panel routes onto the provided mux.
// db is required for dynamic CRUD queries.
// prefix is the URL prefix (e.g. "/admin").
//
// Access Control (SEC-01):
// In production/staging, mounting without an explicit admin.AuthGuard is strictly forbidden
// and returns an error to prevent accidental public exposure of the CRUD interface behind proxies.
// In local/development environments, LocalOnlyGuard is permitted as a convenience.
func isSensitiveColumn(colName string) bool {
	lower := strings.ToLower(colName)
	return strings.Contains(lower, "password") ||
		strings.Contains(lower, "secret") ||
		strings.Contains(lower, "token") ||
		strings.Contains(lower, "api_key") ||
		strings.Contains(lower, "private_key") ||
		strings.Contains(lower, "totp")
}

func isAutoManagedColumn(colName string) bool {
	lower := strings.ToLower(colName)
	return lower == "id" || lower == "created_at" || lower == "updated_at" || lower == "deleted_at"
}

func Mount(mux *http.ServeMux, db *sql.DB, prefix string) error {
	if db == nil {
		return fmt.Errorf("admin: database connection cannot be nil")
	}
	if prefix == "" {
		prefix = "/admin"
	}

	rawEnv := strings.TrimSpace(os.Getenv("APP_ENV"))
	env := strings.ToLower(rawEnv)

	if AuthGuard == nil {
		if rawEnv == "" || (env != "local" && env != "development" && env != "testing") {
			return fmt.Errorf("[GoStack Security Blocker: SEC-01] admin.Mount rejected: When APP_ENV is unset or set to %q, GoDash admin panel cannot be mounted without an explicit admin.AuthGuard configured", rawEnv)
		}
	}

	// Resolve the active guard: use the configured AuthGuard or fall back to LocalOnlyGuard.
	guard := AuthGuard
	if guard == nil {
		guard = LocalOnlyGuard
	}

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, prefix)
		parts := strings.Split(strings.Trim(path, "/"), "/")

		csrfToken, err := getOrSetAdminCSRF(w, r)
		if err != nil {
			http.Error(w, "500 Internal Server Error: Failed to initialize CSRF token", http.StatusInternalServerError)
			return
		}
		if r.Method == http.MethodPost {
			if !verifyAdminCSRF(r) {
				http.Error(w, "403 Forbidden: Invalid or missing CSRF token", http.StatusForbidden)
				return
			}
		}

		switch {
		case path == "/assets/admin.css" || (len(parts) == 2 && parts[0] == "assets" && parts[1] == "admin.css"):
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
			w.Header().Set("Cache-Control", "public, max-age=86400")
			_, _ = w.Write([]byte(adminCSS))
			return
		case path == "" || path == "/":
			handleDashboard(w, r)
		case path == "/sequence" || path == "sequence" || (len(parts) == 1 && parts[0] == "sequence"):
			handleSequenceDashboard(w, r, csrfToken)
		case len(parts) == 2 && parts[0] == "sequence" && parts[1] == "retry":
			handleSequenceRetry(w, r)
		case len(parts) == 2 && parts[0] == "sequence" && parts[1] == "delete":
			handleSequenceDelete(w, r)
		case len(parts) == 1 && parts[0] != "":
			handleList(w, r, db, parts[0], csrfToken)
		case len(parts) == 2 && parts[1] == "create":
			handleCreate(w, r, db, parts[0], csrfToken)
		case len(parts) == 2:
			handleEdit(w, r, db, parts[0], parts[1], csrfToken)
		case len(parts) == 3 && parts[2] == "delete":
			handleDestroy(w, r, db, parts[0], parts[1])
		default:
			http.NotFound(w, r)
		}
	})

	mux.Handle(prefix+"/", guard(inner))
	return nil
}

func writeTemplate(w http.ResponseWriter, format string, a ...any) {
	if _, err := fmt.Fprintf(w, format, a...); err != nil {
		log.Printf("[GoStack Admin] Render write error: %v", err)
	}
}

func handleDashboard(w http.ResponseWriter, r *http.Request) {
	entries := All()
	var rows string
	for name, entry := range entries {
		rows += fmt.Sprintf(`<tr><td><a href="/admin/%s">%s</a></td><td>%s</td></tr>`, html.EscapeString(name), html.EscapeString(entry.Label), html.EscapeString(entry.TableName))
	}
	w.Header().Set("Content-Type", "text/html")
	writeTemplate(w, dashboardTpl, rows)
}

func handleList(w http.ResponseWriter, r *http.Request, db *sql.DB, modelName, csrfToken string) {
	entry, ok := Find(modelName)
	if !ok {
		http.NotFound(w, r)
		return
	}

	// C19: Paginate with ?page= and COUNT(*)
	page := 1
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	pageSize := 100
	offset := (page - 1) * pageSize

	var totalRows int
	_ = db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", entry.TableName)).Scan(&totalRows)

	query := fmt.Sprintf("SELECT * FROM %s LIMIT %d OFFSET %d", entry.TableName, pageSize, offset)
	rows, err := db.Query(query)
	if err != nil {
		log.Printf("[GoStack Admin] Query error: %v", err)
		http.Error(w, "500 Internal Server Error: Query failed", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		log.Printf("[GoStack Admin] Columns error: %v", err)
		http.Error(w, "500 Internal Server Error: Failed to inspect columns", http.StatusInternalServerError)
		return
	}
	var tableRows string
	escCSRF := html.EscapeString(csrfToken)
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			log.Printf("[GoStack Admin] Scan error: %v", err)
			http.Error(w, fmt.Sprintf("500 Internal Server Error: Failed to scan row: %v", err), http.StatusInternalServerError)
			return
		}
		tableRows += "<tr>"
		var id any
		for i, v := range vals {
			if cols[i] == "id" {
				id = v
			}
			displayVal := fmt.Sprintf("%v", v)
			if isSensitiveColumn(cols[i]) {
				displayVal = "[REDACTED]"
			}
			tableRows += fmt.Sprintf("<td>%s</td>", html.EscapeString(displayVal))
		}
		escModel := html.EscapeString(modelName)
		escID := html.EscapeString(fmt.Sprintf("%v", id))
		tableRows += fmt.Sprintf(`<td><a href="/admin/%s/%s">Edit</a> | <form method="POST" action="/admin/%s/%s/delete" class="inline-form"><input type="hidden" name="_csrf" value="%s"><button type="submit" class="btn-delete-link">Delete</button></form></td></tr>`, escModel, escID, escModel, escID, escCSRF)
	}
	if err := rows.Err(); err != nil {
		log.Printf("[GoStack Admin] Row iteration error: %v", err)
		http.Error(w, fmt.Sprintf("500 Internal Server Error: Row iteration error: %v", err), http.StatusInternalServerError)
		return
	}

	var headers string
	for _, c := range cols {
		headers += fmt.Sprintf("<th>%s</th>", html.EscapeString(c))
	}
	headers += "<th>Actions</th>"

	w.Header().Set("Content-Type", "text/html")
	writeTemplate(w, listTpl, html.EscapeString(entry.Label), html.EscapeString(entry.Label), html.EscapeString(modelName), headers, tableRows)
}

func handleCreate(w http.ResponseWriter, r *http.Request, db *sql.DB, modelName, csrfToken string) {
	entry, ok := Find(modelName)
	if !ok {
		http.NotFound(w, r)
		return
	}

	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "400 Bad Request: Malformed form body", http.StatusBadRequest)
			return
		}
		setClauses := []string{}
		args := []any{}
		isPg := isPostgresDB(db)
		for _, col := range entry.Columns {
			if isAutoManagedColumn(col.Name) {
				continue
			}
			val := r.FormValue(col.Name)
			if val == "" && isSensitiveColumn(col.Name) {
				continue
			}
			setClauses = append(setClauses, col.Name)
			args = append(args, val)
		}
		placeholders := make([]string, len(setClauses))
		for i := range setClauses {
			placeholders[i] = sqlPlaceholder(isPg, i+1)
		}
		query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", entry.TableName, strings.Join(setClauses, ","), strings.Join(placeholders, ","))
		if _, err := db.Exec(query, args...); err != nil {
			log.Printf("[GoStack Admin] Create error: %v", err)
			http.Error(w, "500 Internal Server Error: Failed to create record", http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/admin/"+modelName, http.StatusFound)
		return
	}

	fields := fmt.Sprintf(`<input type="hidden" name="_csrf" value="%s">`, html.EscapeString(csrfToken))
	for _, col := range entry.Columns {
		if col.Name == "id" {
			continue
		}
		fields += fmt.Sprintf(`<div class="field"><label>%s</label><input name="%s" type="text"></div>`, html.EscapeString(col.Name), html.EscapeString(col.Name))
	}
	w.Header().Set("Content-Type", "text/html")
	writeTemplate(w, editTpl, "Create "+html.EscapeString(entry.Label), "Create "+html.EscapeString(entry.Label), "/admin/"+html.EscapeString(modelName)+"/create", fields)
}

func handleEdit(w http.ResponseWriter, r *http.Request, db *sql.DB, modelName, id, csrfToken string) {
	entry, ok := Find(modelName)
	if !ok {
		http.NotFound(w, r)
		return
	}

	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "400 Bad Request: Malformed form body", http.StatusBadRequest)
			return
		}
		setClauses := []string{}
		args := []any{}
		isPg := isPostgresDB(db)
		for _, col := range entry.Columns {
			if col.Name == "id" {
				continue
			}
			val := r.FormValue(col.Name)
			// F-321: If a sensitive column (e.g. password_hash, token, secret) was left blank in the form,
			// omit it from the UPDATE statement to preserve the existing stored credential.
			if isSensitiveColumn(col.Name) && strings.TrimSpace(val) == "" {
				continue
			}
			args = append(args, val)
			setClauses = append(setClauses, fmt.Sprintf("%s = %s", col.Name, sqlPlaceholder(isPg, len(args))))
		}
		if len(setClauses) == 0 {
			http.Redirect(w, r, "/admin/"+modelName, http.StatusFound)
			return
		}
		args = append(args, id)
		whereID := sqlPlaceholder(isPg, len(args))
		query := fmt.Sprintf("UPDATE %s SET %s WHERE id = %s", entry.TableName, strings.Join(setClauses, ", "), whereID)
		if _, err := db.Exec(query, args...); err != nil {
			log.Printf("[GoStack Admin] Update error: %v", err)
			http.Error(w, "500 Internal Server Error: Failed to update record", http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/admin/"+modelName, http.StatusFound)
		return
	}

	// Fetch current record
	isPg := isPostgresDB(db)
	cols := make([]string, 0, len(entry.Columns))
	for _, c := range entry.Columns {
		cols = append(cols, c.Name)
	}
	colList := strings.Join(cols, ", ")
	row := db.QueryRow(fmt.Sprintf("SELECT %s FROM %s WHERE id = %s", colList, entry.TableName, sqlPlaceholder(isPg, 1)), id)
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if err := row.Scan(ptrs...); err != nil {
		log.Printf("[GoStack Admin] Scan error for %s ID %s: %v", entry.TableName, id, err)
		http.Error(w, "500 Internal Server Error: Failed to load record for editing", http.StatusInternalServerError)
		return
	}

	colMap := map[string]any{}
	for i, c := range cols {
		colMap[c] = vals[i]
	}

	fields := fmt.Sprintf(`<input type="hidden" name="_csrf" value="%s">`, html.EscapeString(csrfToken))
	for _, col := range entry.Columns {
		if col.Name == "id" {
			continue
		}
		valStr := fmt.Sprintf("%v", colMap[col.Name])
		if isSensitiveColumn(col.Name) {
			valStr = ""
		}
		fields += fmt.Sprintf(`<div class="field"><label>%s</label><input name="%s" type="text" value="%s"></div>`, html.EscapeString(col.Name), html.EscapeString(col.Name), html.EscapeString(valStr))
	}

	w.Header().Set("Content-Type", "text/html")
	escLabel := html.EscapeString(entry.Label)
	escID := html.EscapeString(id)
	escModel := html.EscapeString(modelName)
	writeTemplate(w, editTpl, "Edit "+escLabel+" #"+escID, "Edit "+escLabel+" #"+escID, "/admin/"+escModel+"/"+escID, fields)
}

func handleDestroy(w http.ResponseWriter, r *http.Request, db *sql.DB, modelName, id string) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "405 Method Not Allowed - Destructive actions require POST", http.StatusMethodNotAllowed)
		return
	}
	entry, ok := Find(modelName)
	if !ok {
		http.NotFound(w, r)
		return
	}
	isPg := isPostgresDB(db)
	query := fmt.Sprintf("DELETE FROM %s WHERE id = %s", entry.TableName, sqlPlaceholder(isPg, 1))
	if _, err := db.Exec(query, id); err != nil {
		log.Printf("[GoStack Admin] Delete error: %v", err)
		http.Error(w, "500 Internal Server Error: Failed to delete record", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/"+modelName, http.StatusFound)
}

func handleSequenceDashboard(w http.ResponseWriter, r *http.Request, csrfToken string) {
	w.Header().Set("Content-Type", "text/html")

	if Queue == nil {
		writeTemplate(w, sequenceUnconfiguredTpl)
		return
	}

	inspector, ok := Queue.(contract.QueueInspector)
	if !ok {
		writeTemplate(w, sequenceUnsupportedTpl)
		return
	}

	stats, err := inspector.GetStats()
	if err != nil {
		log.Printf("[GoStack Admin] Queue stats error: %v", err)
		http.Error(w, "500 Internal Server Error: Failed to load queue stats", http.StatusInternalServerError)
		return
	}

	failedJobs, err := inspector.GetFailedJobs()
	if err != nil {
		log.Printf("[GoStack Admin] Queue failed jobs error: %v", err)
		http.Error(w, "500 Internal Server Error: Failed to load failed jobs", http.StatusInternalServerError)
		return
	}

	var rows string
	escCSRF := html.EscapeString(csrfToken)
	if len(failedJobs) == 0 {
		rows = `<tr><td colspan="5" class="empty-state">No failed jobs found in the dead-letter queue. Everything is running smoothly!</td></tr>`
	} else {
		for _, job := range failedJobs {
			rows += fmt.Sprintf(`
			<tr>
				<td><strong>%s</strong></td>
				<td><code class="payload-box">%s</code></td>
				<td class="badge-col"><span class="badge badge-attempt">%d attempts</span></td>
				<td><span class="error-msg">%s</span></td>
				<td>
					<div class="action-buttons">
						<form method="POST" action="/admin/sequence/retry?id=%s" class="inline-form">
							<input type="hidden" name="_csrf" value="%s">
							<button type="submit" class="btn-action btn-retry">Retry</button>
						</form>
						<form method="POST" action="/admin/sequence/delete?id=%s" class="inline-form">
							<input type="hidden" name="_csrf" value="%s">
							<button type="submit" class="btn-action btn-delete">Delete</button>
						</form>
					</div>
				</td>
			</tr>`,
				html.EscapeString(job.Name),
				html.EscapeString(job.Payload),
				job.Attempts,
				html.EscapeString(job.Error),
				html.EscapeString(job.ID),
				escCSRF,
				html.EscapeString(job.ID),
				escCSRF,
			)
		}
	}

	statsBar := fmt.Sprintf(`
		<div class="card card-pending">
			<div class="card-label">Pending Jobs</div>
			<div class="card-value">%d</div>
		</div>
		<div class="card card-delayed">
			<div class="card-label">Delayed Jobs</div>
			<div class="card-value">%d</div>
		</div>
		<div class="card card-failed">
			<div class="card-label">Failed Jobs</div>
			<div class="card-value">%d</div>
		</div>
	`, stats.Pending, stats.Delayed, stats.Failed)

	writeTemplate(w, sequenceTpl, stats.Driver, statsBar, rows)
}

func handleSequenceRetry(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "405 Method Not Allowed - Retrying jobs requires POST", http.StatusMethodNotAllowed)
		return
	}
	if Queue == nil {
		http.Error(w, "Queue not configured", 400)
		return
	}
	inspector, ok := Queue.(contract.QueueInspector)
	if !ok {
		http.Error(w, "Queue inspection unsupported", 400)
		return
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "Missing job ID", 400)
		return
	}

	err := inspector.RetryJob(id)
	if err != nil {
		log.Printf("[GoStack Admin] Retry error: %v", err)
		http.Error(w, "500 Internal Server Error: Failed to retry job", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin/sequence", http.StatusFound)
}

func handleSequenceDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "405 Method Not Allowed - Deleting jobs requires POST", http.StatusMethodNotAllowed)
		return
	}
	if Queue == nil {
		http.Error(w, "Queue not configured", 400)
		return
	}
	inspector, ok := Queue.(contract.QueueInspector)
	if !ok {
		http.Error(w, "Queue inspection unsupported", 400)
		return
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "Missing job ID", 400)
		return
	}

	err := inspector.DeleteFailedJob(id)
	if err != nil {
		log.Printf("[GoStack Admin] Delete failed job error: %v", err)
		http.Error(w, "500 Internal Server Error: Failed to delete job", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin/sequence", http.StatusFound)
}

// ─────────────────── Embedded HTML Templates ────────────────────────────────

const dashboardTpl = `<!DOCTYPE html><html><head><title>GoStack Admin</title>
<link rel="stylesheet" href="/admin/assets/admin.css">
</head><body>
<div class="header"><div class="logo-area"><span class="logo">⚡ GoStack Admin</span></div></div>
<div class="container">
<h1>Registered Models</h1>
<div class="table-container"><table><thead><tr><th>Model</th><th>Table</th></tr></thead><tbody>%s</tbody></table></div>
</div></body></html>`

const listTpl = `<!DOCTYPE html><html><head><title>GoStack Admin — %s</title>
<link rel="stylesheet" href="/admin/assets/admin.css">
</head><body>
<div class="header"><div class="logo-area"><span class="logo">⚡ GoStack Admin</span></div><a href="/admin" class="back-btn">← Dashboard</a></div>
<div class="container">
<h1>%s</h1>
<div class="actions"><a href="/admin/%s/create" class="btn">+ New Record</a></div>
<div class="table-container"><table><thead><tr>%s</tr></thead><tbody>%s</tbody></table></div>
</div></body></html>`

const editTpl = `<!DOCTYPE html><html><head><title>GoStack Admin — %s</title>
<link rel="stylesheet" href="/admin/assets/admin.css">
</head><body>
<div class="header"><div class="logo-area"><span class="logo">⚡ GoStack Admin</span></div><a href="javascript:history.back()" class="back-btn">← Back</a></div>
<div class="container">
<h1>%s</h1>
<form method="POST" action="%s">%s<button type="submit" class="btn">Save</button></form>
</div></body></html>`

const sequenceTpl = `<!DOCTYPE html><html><head><title>GoStack Sequence Dashboard</title>
<link rel="stylesheet" href="/admin/assets/admin.css">
</head><body>
<div class="header">
	<div class="logo-area">
		<span class="logo">⚡ Sequence Dashboard</span>
		<span class="driver-badge">%s driver</span>
	</div>
	<a href="/admin" class="back-btn">← Back to Admin</a>
</div>
<div class="container">
	<div class="stats-grid">%s</div>
	<div class="section-title">Failed Dead-Letter Jobs</div>
	<div class="table-container">
		<table>
			<thead>
				<tr>
					<th class="col-name">Job Name</th>
					<th class="col-payload">Payload</th>
					<th class="col-attempts">Attempts</th>
					<th class="col-error">Last Error</th>
					<th class="col-actions">Actions</th>
				</tr>
			</thead>
			<tbody>
				%s
			</tbody>
		</table>
	</div>
</div>
</body></html>`

const sequenceUnconfiguredTpl = `<!DOCTYPE html><html><head><title>GoStack Sequence Dashboard</title>
<link rel="stylesheet" href="/admin/assets/admin.css">
</head>
<body>
<div class="card">
	<h1>⚡ Sequence Queue Unconfigured</h1>
	<p>The Sequence background queue dashboard is currently not active because no active queue instance was registered with the admin panel.</p>
	<p>To register it, add this call during your application bootstrap (e.g. in your <code>main.go</code>):</p>
	<p class="code-box">
		<code>admin.SetQueue(myQueueInstance)</code>
	</p>
	<a href="/admin" class="btn">Return to Dashboard</a>
</div>
</body></html>`

const sequenceUnsupportedTpl = `<!DOCTYPE html><html><head><title>GoStack Sequence Dashboard</title>
<link rel="stylesheet" href="/admin/assets/admin.css">
</head>
<body>
<div class="card">
	<h1>⚡ Dashboard Inspection Unsupported</h1>
	<p>The registered queue instance does not support inspection services because it does not implement the required <code>contract.QueueInspector</code> interface.</p>
	<p>Please ensure you are using one of GoStack's built-in queue drivers (e.g. MemoryQueue or RedisQueue).</p>
	<a href="/admin" class="btn">Return to Dashboard</a>
</div>
</body></html>`
