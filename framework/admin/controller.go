package admin

// Purpose: To provide the HTTP handlers that power the GoDash CRUD interface.
// Philosophy: Controllers in GoDash are pure reflection engines — they do not know
// about specific models at compile time. They read from the Registry at runtime, dynamically
// issue queries against the correct table, and render HTML responses with embedded views.
// Architecture:
// - Mount(mux): Registers all admin routes onto the provided http.ServeMux.
// - handleDashboard: Renders the admin home page listing all registered models.
// - handleList: Queries a model's table and renders paginated rows.
// - handleEdit: Renders a form pre-filled with a specific record's data.
// - handleStore: Processes a POST from the edit form and issues an UPDATE.
// - handleDestroy: Processes a DELETE request and redirects back.
// Choice:
// We use the standard library `net/http` only, with no external router dependency, so the
// admin panel can be mounted onto any existing GoStack router or standalone ServeMux without
// coupling. The embedded HTML is written inline to keep the panel fully self-contained.

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"github.com/charledeon77/gostack-framework/framework/contract"
)

// Mount registers all admin panel routes onto the provided mux.
// db is required for dynamic CRUD queries.
// prefix is the URL prefix (e.g. "/admin").
func Mount(mux *http.ServeMux, db *sql.DB, prefix string) {
	if prefix == "" {
		prefix = "/admin"
	}
	mux.HandleFunc(prefix+"/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, prefix)
		parts := strings.Split(strings.Trim(path, "/"), "/")

		switch {
		case path == "" || path == "/":
			handleDashboard(w, r)
		case path == "/sequence" || path == "sequence" || (len(parts) == 1 && parts[0] == "sequence"):
			handleSequenceDashboard(w, r)
		case len(parts) == 2 && parts[0] == "sequence" && parts[1] == "retry":
			handleSequenceRetry(w, r)
		case len(parts) == 2 && parts[0] == "sequence" && parts[1] == "delete":
			handleSequenceDelete(w, r)
		case len(parts) == 1 && parts[0] != "":
			handleList(w, r, db, parts[0])
		case len(parts) == 2 && parts[1] == "create":
			handleCreate(w, r, db, parts[0])
		case len(parts) == 2:
			handleEdit(w, r, db, parts[0], parts[1])
		case len(parts) == 3 && parts[2] == "delete":
			handleDestroy(w, r, db, parts[0], parts[1])
		default:
			http.NotFound(w, r)
		}
	})
}

func handleDashboard(w http.ResponseWriter, r *http.Request) {
	entries := All()
	var rows string
	for name, entry := range entries {
		rows += fmt.Sprintf(`<tr><td><a href="/admin/%s">%s</a></td><td>%s</td></tr>`, name, entry.Label, entry.TableName)
	}
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, dashboardTpl, rows)
}

func handleList(w http.ResponseWriter, r *http.Request, db *sql.DB, modelName string) {
	entry, ok := Find(modelName)
	if !ok {
		http.NotFound(w, r)
		return
	}

	rows, err := db.Query(fmt.Sprintf("SELECT * FROM %s LIMIT 100", entry.TableName))
	if err != nil {
		http.Error(w, "Query failed: "+err.Error(), 500)
		return
	}
	defer rows.Close()

	cols, _ := rows.Columns()
	var tableRows string
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		rows.Scan(ptrs...)
		tableRows += "<tr>"
		var id any
		for i, v := range vals {
			if cols[i] == "id" {
				id = v
			}
			tableRows += fmt.Sprintf("<td>%v</td>", v)
		}
		tableRows += fmt.Sprintf(`<td><a href="/admin/%s/%v">Edit</a> | <a href="/admin/%s/%v/delete" onclick="return confirm('Delete?')">Delete</a></td></tr>`, modelName, id, modelName, id)
	}

	var headers string
	for _, c := range cols {
		headers += fmt.Sprintf("<th>%s</th>", c)
	}
	headers += "<th>Actions</th>"

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, listTpl, entry.Label, entry.Label, modelName, headers, tableRows)
}

func handleCreate(w http.ResponseWriter, r *http.Request, db *sql.DB, modelName string) {
	entry, ok := Find(modelName)
	if !ok {
		http.NotFound(w, r)
		return
	}

	if r.Method == http.MethodPost {
		r.ParseForm()
		setClauses := []string{}
		args := []any{}
		for _, col := range entry.Columns {
			if col.Name == "id" {
				continue
			}
			val := r.FormValue(col.Name)
			setClauses = append(setClauses, col.Name)
			args = append(args, val)
		}
		placeholders := strings.Repeat("?,", len(setClauses))
		placeholders = strings.TrimSuffix(placeholders, ",")
		query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", entry.TableName, strings.Join(setClauses, ","), placeholders)
		db.Exec(query, args...)
		http.Redirect(w, r, "/admin/"+modelName, http.StatusFound)
		return
	}

	var fields string
	for _, col := range entry.Columns {
		if col.Name == "id" {
			continue
		}
		fields += fmt.Sprintf(`<div class="field"><label>%s</label><input name="%s" type="text"></div>`, col.Name, col.Name)
	}
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, editTpl, "Create "+entry.Label, "Create "+entry.Label, "/admin/"+modelName+"/create", fields)
}

func handleEdit(w http.ResponseWriter, r *http.Request, db *sql.DB, modelName, id string) {
	entry, ok := Find(modelName)
	if !ok {
		http.NotFound(w, r)
		return
	}

	if r.Method == http.MethodPost {
		r.ParseForm()
		setClauses := []string{}
		args := []any{}
		for _, col := range entry.Columns {
			if col.Name == "id" {
				continue
			}
			val := r.FormValue(col.Name)
			setClauses = append(setClauses, col.Name+" = ?")
			args = append(args, val)
		}
		args = append(args, id)
		query := fmt.Sprintf("UPDATE %s SET %s WHERE id = ?", entry.TableName, strings.Join(setClauses, ", "))
		db.Exec(query, args...)
		http.Redirect(w, r, "/admin/"+modelName, http.StatusFound)
		return
	}

	// Fetch current record
	row := db.QueryRow(fmt.Sprintf("SELECT * FROM %s WHERE id = ?", entry.TableName), id)
	cols := make([]string, len(entry.Columns))
	for i, c := range entry.Columns {
		cols[i] = c.Name
	}
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	row.Scan(ptrs...)

	colMap := map[string]any{}
	for i, c := range cols {
		colMap[c] = vals[i]
	}

	var fields string
	for _, col := range entry.Columns {
		if col.Name == "id" {
			continue
		}
		fields += fmt.Sprintf(`<div class="field"><label>%s</label><input name="%s" type="text" value="%v"></div>`, col.Name, col.Name, colMap[col.Name])
	}

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, editTpl, "Edit "+entry.Label+" #"+id, "Edit "+entry.Label+" #"+id, "/admin/"+modelName+"/"+id, fields)
}

func handleDestroy(w http.ResponseWriter, r *http.Request, db *sql.DB, modelName, id string) {
	entry, ok := Find(modelName)
	if !ok {
		http.NotFound(w, r)
		return
	}
	db.Exec(fmt.Sprintf("DELETE FROM %s WHERE id = ?", entry.TableName), id)
	http.Redirect(w, r, "/admin/"+modelName, http.StatusFound)
}

func handleSequenceDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")

	if Queue == nil {
		fmt.Fprintf(w, sequenceUnconfiguredTpl)
		return
	}

	inspector, ok := Queue.(contract.QueueInspector)
	if !ok {
		fmt.Fprintf(w, sequenceUnsupportedTpl)
		return
	}

	stats, err := inspector.GetStats()
	if err != nil {
		http.Error(w, "Failed to load queue stats: "+err.Error(), 500)
		return
	}

	failedJobs, err := inspector.GetFailedJobs()
	if err != nil {
		http.Error(w, "Failed to load failed jobs: "+err.Error(), 500)
		return
	}

	var rows string
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
						<a href="/admin/sequence/retry?id=%s" class="btn-action btn-retry">Retry</a>
						<a href="/admin/sequence/delete?id=%s" class="btn-action btn-delete" onclick="return confirm('Are you sure you want to permanently delete this job?')">Delete</a>
					</div>
				</td>
			</tr>`,
				job.Name,
				job.Payload,
				job.Attempts,
				job.Error,
				job.ID,
				job.ID,
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

	fmt.Fprintf(w, sequenceTpl, stats.Driver, statsBar, rows)
}

func handleSequenceRetry(w http.ResponseWriter, r *http.Request) {
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
		http.Error(w, "Failed to retry job: "+err.Error(), 500)
		return
	}

	http.Redirect(w, r, "/admin/sequence", http.StatusFound)
}

func handleSequenceDelete(w http.ResponseWriter, r *http.Request) {
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
		http.Error(w, "Failed to delete job: "+err.Error(), 500)
		return
	}

	http.Redirect(w, r, "/admin/sequence", http.StatusFound)
}

// ─────────────────── Embedded HTML Templates ────────────────────────────────

const dashboardTpl = `<!DOCTYPE html><html><head><title>GoStack Admin</title><style>
body{font-family:sans-serif;background:#0f172a;color:#e2e8f0;margin:0;padding:0}
.header{background:#1e293b;padding:1rem 2rem;border-bottom:1px solid #334155;display:flex;align-items:center;gap:1rem}
.logo{font-size:1.4rem;font-weight:700;color:#38bdf8}
.container{padding:2rem}
h1{color:#38bdf8;font-size:1.8rem;margin-bottom:1.5rem}
table{width:100%%;border-collapse:collapse;background:#1e293b;border-radius:8px;overflow:hidden}
th,td{padding:0.75rem 1rem;text-align:left;border-bottom:1px solid #334155}
th{background:#0f172a;color:#94a3b8;text-transform:uppercase;font-size:0.75rem;letter-spacing:0.05em}
td a{color:#38bdf8;text-decoration:none}
td a:hover{text-decoration:underline}
</style></head><body>
<div class="header"><span class="logo">⚡ GoStack Admin</span></div>
<div class="container">
<h1>Registered Models</h1>
<table><thead><tr><th>Model</th><th>Table</th></tr></thead><tbody>%s</tbody></table>
</div></body></html>`

const listTpl = `<!DOCTYPE html><html><head><title>GoStack Admin — %s</title><style>
body{font-family:sans-serif;background:#0f172a;color:#e2e8f0;margin:0;padding:0}
.header{background:#1e293b;padding:1rem 2rem;border-bottom:1px solid #334155;display:flex;align-items:center;gap:1rem}
.logo{font-size:1.4rem;font-weight:700;color:#38bdf8}
.container{padding:2rem}
h1{color:#38bdf8;font-size:1.8rem;margin-bottom:1rem}
.actions{margin-bottom:1.5rem}
.btn{display:inline-block;padding:0.5rem 1rem;border-radius:6px;background:#38bdf8;color:#0f172a;font-weight:600;text-decoration:none;font-size:0.9rem}
.btn:hover{background:#7dd3fc}
table{width:100%%;border-collapse:collapse;background:#1e293b;border-radius:8px;overflow:hidden}
th,td{padding:0.75rem 1rem;text-align:left;border-bottom:1px solid #334155}
th{background:#0f172a;color:#94a3b8;text-transform:uppercase;font-size:0.75rem;letter-spacing:0.05em}
td a{color:#38bdf8;text-decoration:none;margin-right:0.5rem}
td a:hover{text-decoration:underline}
.back{color:#94a3b8;text-decoration:none;font-size:0.9rem}
.back:hover{color:#e2e8f0}
</style></head><body>
<div class="header"><span class="logo">⚡ GoStack Admin</span><a href="/admin" class="back">← Dashboard</a></div>
<div class="container">
<h1>%s</h1>
<div class="actions"><a href="/admin/%s/create" class="btn">+ New Record</a></div>
<table><thead><tr>%s</tr></thead><tbody>%s</tbody></table>
</div></body></html>`

const editTpl = `<!DOCTYPE html><html><head><title>GoStack Admin — %s</title><style>
body{font-family:sans-serif;background:#0f172a;color:#e2e8f0;margin:0;padding:0}
.header{background:#1e293b;padding:1rem 2rem;border-bottom:1px solid #334155;display:flex;align-items:center;gap:1rem}
.logo{font-size:1.4rem;font-weight:700;color:#38bdf8}
.container{padding:2rem;max-width:640px}
h1{color:#38bdf8;font-size:1.8rem;margin-bottom:1.5rem}
.field{margin-bottom:1rem}
label{display:block;font-size:0.85rem;color:#94a3b8;margin-bottom:0.3rem;text-transform:capitalize}
input{width:100%%;padding:0.6rem 0.8rem;background:#1e293b;border:1px solid #334155;border-radius:6px;color:#e2e8f0;font-size:1rem;box-sizing:border-box}
input:focus{outline:none;border-color:#38bdf8}
.btn{display:inline-block;padding:0.6rem 1.5rem;border-radius:6px;background:#38bdf8;color:#0f172a;font-weight:700;border:none;cursor:pointer;font-size:1rem}
.btn:hover{background:#7dd3fc}
.back{color:#94a3b8;text-decoration:none;font-size:0.9rem}
</style></head><body>
<div class="header"><span class="logo">⚡ GoStack Admin</span><a href="javascript:history.back()" class="back">← Back</a></div>
<div class="container">
<h1>%s</h1>
<form method="POST" action="%s">%s<button type="submit" class="btn">Save</button></form>
</div></body></html>`

const sequenceTpl = `<!DOCTYPE html><html><head><title>GoStack Sequence Dashboard</title>
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Plus+Jakarta+Sans:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
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
	--accent-red-hover: #f87171;
	--accent-green: #10b981;
}
body {
	font-family: 'Plus Jakarta Sans', sans-serif;
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
.logo-area {
	display: flex;
	align-items: center;
	gap: 0.75rem;
}
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
.back-btn {
	color: var(--text-secondary);
	text-decoration: none;
	font-size: 0.9rem;
	font-weight: 500;
	transition: color 0.2s ease;
	display: flex;
	align-items: center;
	gap: 0.4rem;
}
.back-btn:hover {
	color: var(--text-primary);
}
.container {
	padding: 2.5rem 2rem;
	max-width: 1280px;
	margin: 0 auto;
}
h1 {
	font-size: 1.85rem;
	font-weight: 700;
	margin-bottom: 2rem;
	letter-spacing: -0.02em;
}
.stats-grid {
	display: grid;
	grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
	gap: 1.5rem;
	margin-bottom: 2.5rem;
}
.card {
	background: var(--bg-card);
	border: 1px solid var(--border-color);
	border-radius: 12px;
	padding: 1.5rem;
	position: relative;
	overflow: hidden;
	transition: transform 0.2s ease, border-color 0.2s ease;
}
.card:hover {
	transform: translateY(-2px);
	border-color: rgba(6, 182, 212, 0.4);
}
.card::before {
	content: '';
	position: absolute;
	top: 0;
	left: 0;
	width: 4px;
	height: 100%%;
}
.card-pending::before { background: var(--primary-cyan); }
.card-delayed::before { background: var(--accent-yellow); }
.card-failed::before { background: var(--accent-red); }

.card-label {
	font-size: 0.85rem;
	font-weight: 600;
	text-transform: uppercase;
	letter-spacing: 0.05em;
	color: var(--text-secondary);
	margin-bottom: 0.5rem;
}
.card-value {
	font-size: 2.5rem;
	font-weight: 700;
	letter-spacing: -0.03em;
}

.section-title {
	font-size: 1.25rem;
	font-weight: 600;
	margin-bottom: 1.25rem;
	color: var(--text-primary);
}
.table-container {
	background: var(--bg-card);
	border: 1px solid var(--border-color);
	border-radius: 12px;
	overflow: hidden;
}
table {
	width: 100%%;
	border-collapse: collapse;
	text-align: left;
}
th, td {
	padding: 1rem 1.25rem;
	border-bottom: 1px solid var(--border-color);
	font-size: 0.9rem;
}
th {
	background: rgba(15, 23, 42, 0.4);
	color: var(--text-secondary);
	font-weight: 600;
	text-transform: uppercase;
	font-size: 0.75rem;
	letter-spacing: 0.05em;
}
tr:last-child td {
	border-bottom: none;
}
.empty-state {
	text-align: center;
	color: var(--text-secondary);
	padding: 4rem 2rem;
	font-size: 0.95rem;
}
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
.badge {
	font-size: 0.75rem;
	font-weight: 600;
	padding: 0.25rem 0.5rem;
	border-radius: 6px;
}
.badge-attempt {
	background: rgba(245, 158, 11, 0.1);
	color: var(--accent-yellow);
	border: 1px solid rgba(245, 158, 11, 0.2);
}
.error-msg {
	color: #fca5a5;
	font-family: monospace;
	font-size: 0.825rem;
}
.action-buttons {
	display: flex;
	gap: 0.5rem;
}
.btn-action {
	padding: 0.4rem 0.75rem;
	font-size: 0.8rem;
	font-weight: 600;
	border-radius: 6px;
	text-decoration: none;
	text-align: center;
	transition: background-color 0.2s ease, color 0.2s ease;
}
.btn-retry {
	background: rgba(6, 182, 212, 0.1);
	color: var(--primary-cyan);
	border: 1px solid rgba(6, 182, 212, 0.2);
}
.btn-retry:hover {
	background: var(--primary-cyan);
	color: var(--bg-main);
}
.btn-delete {
	background: rgba(239, 68, 68, 0.1);
	color: var(--accent-red);
	border: 1px solid rgba(239, 68, 68, 0.2);
}
.btn-delete:hover {
	background: var(--accent-red);
	color: var(--text-primary);
}
</style></head><body>
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
					<th style="width: 20%%">Job Name</th>
					<th style="width: 25%%">Payload</th>
					<th style="width: 15%%">Attempts</th>
					<th style="width: 25%%">Last Error</th>
					<th style="width: 15%%">Actions</th>
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
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Plus+Jakarta+Sans:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
body {
	font-family: 'Plus Jakarta Sans', sans-serif;
	background: #0b0f19;
	color: #f8fafc;
	margin: 0;
	padding: 0;
	display: flex;
	justify-content: center;
	align-items: center;
	min-height: 100vh;
}
.card {
	background: #151d30;
	border: 1px solid #24324f;
	border-radius: 16px;
	padding: 2.5rem;
	max-width: 500px;
	text-align: center;
}
h1 {
	color: #06b6d4;
	font-size: 1.5rem;
	margin-top: 0;
	margin-bottom: 1rem;
}
p {
	color: #94a3b8;
	line-height: 1.6;
	font-size: 0.95rem;
}
code {
	background: #0b0f19;
	color: #38bdf8;
	padding: 0.2rem 0.4rem;
	border-radius: 4px;
	font-family: monospace;
}
.btn {
	display: inline-block;
	margin-top: 1.5rem;
	padding: 0.6rem 1.2rem;
	background: #06b6d4;
	color: #0b0f19;
	font-weight: 600;
	border-radius: 8px;
	text-decoration: none;
	font-size: 0.9rem;
}
.btn:hover {
	background: #22d3ee;
}
</style></head><body>
<div class="card">
	<h1>⚡ Sequence Queue Unconfigured</h1>
	<p>The Sequence background queue dashboard is currently not active because no active queue instance was registered with the admin panel.</p>
	<p>To register it, add this call during your application bootstrap (e.g. in your <code>main.go</code>):</p>
	<p style="text-align: left; background: #0b0f19; padding: 1rem; border-radius: 8px; border: 1px solid #24324f;">
		<code>admin.SetQueue(myQueueInstance)</code>
	</p>
	<a href="/admin" class="btn">Return to Dashboard</a>
</div>
</body></html>`

const sequenceUnsupportedTpl = `<!DOCTYPE html><html><head><title>GoStack Sequence Dashboard</title>
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Plus+Jakarta+Sans:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
body {
	font-family: 'Plus Jakarta Sans', sans-serif;
	background: #0b0f19;
	color: #f8fafc;
	margin: 0;
	padding: 0;
	display: flex;
	justify-content: center;
	align-items: center;
	min-height: 100vh;
}
.card {
	background: #151d30;
	border: 1px solid #24324f;
	border-radius: 16px;
	padding: 2.5rem;
	max-width: 500px;
	text-align: center;
}
h1 {
	color: #f59e0b;
	font-size: 1.5rem;
	margin-top: 0;
	margin-bottom: 1rem;
}
p {
	color: #94a3b8;
	line-height: 1.6;
	font-size: 0.95rem;
}
.btn {
	display: inline-block;
	margin-top: 1.5rem;
	padding: 0.6rem 1.2rem;
	background: #f59e0b;
	color: #0b0f19;
	font-weight: 600;
	border-radius: 8px;
	text-decoration: none;
	font-size: 0.9rem;
}
.btn:hover {
	background: #fbbf24;
}
</style></head><body>
<div class="card">
	<h1>⚡ Dashboard Inspection Unsupported</h1>
	<p>The registered queue instance does not support inspection services because it does not implement the required <code>contract.QueueInspector</code> interface.</p>
	<p>Please ensure you are using one of GoStack's built-in queue drivers (e.g. MemoryQueue or RedisQueue).</p>
	<a href="/admin" class="btn">Return to Dashboard</a>
</div>
</body></html>`
