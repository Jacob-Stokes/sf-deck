// Command newsurface scaffolds a new list-backed org surface for
// sf-deck, emitting the boilerplate that can't be generated at runtime.
//
// The generic list-resource registry (internal/ui/list_resource_registry.go)
// already collapses the sync / routing / refresh plumbing to a single
// registration. This tool generates the remaining per-surface pieces so
// a new surface is a fill-in-the-blanks job rather than 15 hand edits.
//
// Usage:
//
//	go run ./cmd/newsurface \
//	    -name CronJob \        # Go identifier stem (PascalCase)
//	    -row CronJobRow \      # sf row type (in package sf)
//	    -key cron_jobs_v1 \    # resource cache key
//	    -fetch ListCronJobs \  # sf fetch func: func(alias string) ([]sf.CronJobRow, error)
//	    -title "SCHEDULED JOBS"
//
// It prints, grouped by destination file:
//   - internal/sf/<snake>.go            (new file: row + Field + fetch stub)
//   - orgdata_groups.go   snippets      (Resource + ListView + TableState fields)
//   - orgdata_resources.go snippet      (Resource declaration)
//   - list_resource_registrations.go    (the one registration)
//   - list_column_schemas_extra.go      (column schema stub)
//   - list_surface_misc.go              (table spec + list surface)
//
// Nothing is written except the new sf file (with -write); everything
// else is printed so you paste it at the right insertion point.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"text/template"
)

type data struct {
	Name   string // PascalCase stem, e.g. "CronJob"
	Row    string // sf row type, e.g. "CronJobRow"
	Key    string // cache key, e.g. "cron_jobs_v1"
	Fetch  string // sf fetch func name, e.g. "ListCronJobs"
	Title  string // list title, e.g. "SCHEDULED JOBS"
	Snake  string // snake_case file name, e.g. "cron_job"
	Camel  string // camelCase for var names, e.g. "cronJob"
	TTLMin int    // resource TTL in minutes
}

func main() {
	var d data
	flag.StringVar(&d.Name, "name", "", "PascalCase stem (e.g. CronJob)")
	flag.StringVar(&d.Row, "row", "", "sf row type (e.g. CronJobRow)")
	flag.StringVar(&d.Key, "key", "", "resource cache key (e.g. cron_jobs_v1)")
	flag.StringVar(&d.Fetch, "fetch", "", "sf fetch func (e.g. ListCronJobs)")
	flag.StringVar(&d.Title, "title", "", "list title (e.g. SCHEDULED JOBS)")
	flag.IntVar(&d.TTLMin, "ttl", 5, "resource TTL in minutes")
	write := flag.Bool("write", false, "write the new internal/sf/<snake>.go file")
	flag.Parse()

	if d.Name == "" || d.Row == "" || d.Key == "" || d.Fetch == "" {
		fmt.Fprintln(os.Stderr, "newsurface: -name, -row, -key and -fetch are required")
		flag.Usage()
		os.Exit(2)
	}
	if d.Title == "" {
		d.Title = strings.ToUpper(camelToWords(d.Name))
	}
	d.Snake = pascalToSnake(d.Name)
	d.Camel = lowerFirst(d.Name)

	render := func(name, tmpl string) string {
		t := template.Must(template.New(name).Parse(tmpl))
		var b strings.Builder
		if err := t.Execute(&b, d); err != nil {
			panic(err)
		}
		return b.String()
	}

	sfFile := render("sf", sfTemplate)
	if *write {
		path := fmt.Sprintf("internal/sf/%s.go", d.Snake)
		if _, err := os.Stat(path); err == nil {
			fmt.Fprintf(os.Stderr, "refusing to overwrite existing %s\n", path)
		} else if err := os.WriteFile(path, []byte(sfFile), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", path, err)
		} else {
			fmt.Printf("wrote %s\n\n", path)
		}
	} else {
		section("internal/sf/"+d.Snake+".go  (new file — pass -write to create)", sfFile)
	}

	section("internal/ui/orgdata_groups.go  (add these 3 fields)", render("fields", fieldsTemplate))
	section("internal/ui/orgdata_resources.go  (add in initOrgDataResources)", render("res", resTemplate))
	section("internal/ui/list_resource_registrations.go  (add in init)", render("reg", regTemplate))
	section("internal/ui/list_column_schemas_extra.go  (add — customise columns)", render("cols", colsTemplate))
	section("internal/ui/list_surface_misc.go  (add)", render("surf", surfTemplate))
	fmt.Print(nextSteps)
}

func section(title, body string) {
	fmt.Printf("── %s ──\n%s\n\n", title, body)
}

// --- templates -------------------------------------------------------

const sfTemplate = `package sf

import "fmt"

// {{.Row}} is one row of the {{.Name}} surface.
// TODO: replace the placeholder fields with the real SOQL projection.
type {{.Row}} struct {
	ID   string
	Name string
}

// Field implements query.Row so rows flow through the generic
// list/search/sort engine. TODO: expose the fields your columns render.
func (r {{.Row}}) Field(name string) (any, bool) {
	switch name {
	case "Id":
		return r.ID, true
	case "Name":
		return r.Name, true
	}
	return nil, false
}

// {{.Fetch}} returns the {{.Name}} rows for the org. Read-only.
// TODO: implement the SOQL / API call.
func {{.Fetch}}(target string) ([]{{.Row}}, error) {
	c, err := RESTClient(target)
	if err != nil {
		return nil, err
	}
	soql := "SELECT Id, Name FROM TODO ORDER BY Name"
	q, err := c.QueryREST(soql, false)
	if err != nil {
		return nil, fmt.Errorf("list {{.Snake}}: %w", err)
	}
	out := make([]{{.Row}}, 0, len(q.Records))
	for _, r := range q.Records {
		out = append(out, {{.Row}}{
			ID:   asString(r["Id"]),
			Name: asString(r["Name"]),
		})
	}
	return out, nil
}
`

const fieldsTemplate = `	{{.Name}}      Resource[[]sf.{{.Row}}]        // near the other Resource fields
	{{.Name}}List  ListView[sf.{{.Row}}]          // near the other ListView fields
	{{.Name}}TableState uilayout.ListTableState   // near the other TableState fields`

const resTemplate = `	d.{{.Name}} = Resource[[]sf.{{.Row}}]{
		Scope: username, Key: "{{.Key}}", TTL: ttl("{{.Snake}}", {{.TTLMin}}*time.Minute),
		Fetch: func() ([]sf.{{.Row}}, error) { return sf.{{.Fetch}}(alias) },
	}`

const regTemplate = `	registerListResource(listResourceSpec[sf.{{.Row}}]{
		Key:  "{{.Key}}",
		Res:  func(d *orgData) *Resource[[]sf.{{.Row}}] { return &d.{{.Name}} },
		List: func(d *orgData) *ListView[sf.{{.Row}}] { return &d.{{.Name}}List },
	})`

const colsTemplate = `func {{.Camel}}ColumnSchema() tablemodel.Schema[sf.{{.Row}}] {
	return tablemodel.Schema[sf.{{.Row}}]{
		DefaultColumns: func(scope string) []string { return []string{"Name"} },
		Columns: map[string]tablemodel.ColumnDef[sf.{{.Row}}]{
			"Name": textColumnDef[sf.{{.Row}}]("NAME", tablemodel.Width{Min: 16, Ideal: 32}, func(r sf.{{.Row}}) string { return r.Name }),
			// TODO: add the rest of your columns.
		},
	}
}`

const surfTemplate = `var {{.Camel}}TableSpec = ListViewTableSpec[sf.{{.Row}}]{
	Schema:   {{.Camel}}ColumnSchema(),
	ListPtr:  func(d *orgData) *ListView[sf.{{.Row}}] { return &d.{{.Name}}List },
	StatePtr: func(d *orgData) *uilayout.ListTableState { return &d.{{.Name}}TableState },
	Title: func(m Model, d *orgData, items []sf.{{.Row}}) string {
		return "{{.Title}} · " + fmt.Sprintf("%d", d.{{.Name}}List.Len()) + " · " +
			humanAge(d.{{.Name}}.FetchedAt()) + stateSuffix(d.{{.Name}}.Busy(), d.{{.Name}}.Err())
	},
	Empty: "  no {{.Snake}} rows.",
}

var {{.Camel}}ListSurface = listSurfaceFromSpec({{.Camel}}TableSpec)`

const nextSteps = `── next steps (the parts that are genuinely per-surface) ──
  1. Paste the snippets above at the noted insertion points.
  2. Add a subtab / tab entry in tab_registry.go pointing List: &<camel>ListSurface.
     Add an Open surface + Identity (or a NoCollectReason) — see the
     TestListOpenSurfacesHaveIdentity drift test.
  3. Add a render func + wire it into the parent tab's render dispatch.
  4. Add an EnsureData case so the resource fetches on subtab visit.
  5. (optional) a sidebar func for the row detail.
  6. go build ./... && go test ./internal/ui/   (drift tests confirm wiring).
`

// --- string helpers --------------------------------------------------

func pascalToSnake(s string) string {
	var b strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(r - 'A' + 'a')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

func camelToWords(s string) string {
	var b strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' && i > 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}
