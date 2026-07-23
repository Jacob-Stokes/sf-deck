package verbs

// registry is the canonical list of every sf-deck verb. Sorted by
// hand into noun groups for legibility; Specs() returns it sorted
// alphabetically by qualified name.
//
// Adding a verb:
//
//   1. Append a Spec to this slice.
//   2. Implement the CLI handler (when CLI != nil) and add it to
//      the CLI dispatch switch.
//   3. Implement the IPC handler (when IPC != nil) and add it to
//      the listener dispatch + Backend interface.
//   4. Run `go test ./internal/verbs/...` — the drift test checks
//      that every CLI binding has a dispatch arm and every IPC
//      binding has a Backend method.
//
// Keep summaries to one line. Long-form notes go in the Notes
// field; that's what skill docs render under each verb.

var registry = []Spec{
	// ===== state / nav (mostly IPC-only) ============================
	{
		Noun: "state", Verb: "get",
		Summary:   "Read the live TUI state snapshot (tab, org, drilldown ids).",
		Stability: "stable",
		IPC: &IPCBinding{
			Command: "state.get",
		},
	},
	{
		Noun: "state", Verb: "subscribe",
		Summary:   "Subscribe to live TUI state updates over the socket.",
		Stability: "stable",
		IPC: &IPCBinding{
			Command: "state.subscribe",
		},
	},
	{
		Noun: "tab", Verb: "open",
		Summary:   "Navigate the live TUI to the named tab.",
		Stability: "stable",
		IPC: &IPCBinding{
			Command: "tab.open",
			Args: []FieldSpec{
				{Name: "tab", Type: "string", Required: true, Description: "tab id (home/records/flows/apex/...)"},
				{Name: "sobject", Type: "string", Description: "drill into this sobject (records tab only)"},
				{Name: "org_user", Type: "string", Description: "switch org before opening tab"},
			},
		},
	},

	// ===== org ======================================================
	{
		Noun: "org", Verb: "list",
		Summary:   "List sf CLI-known orgs with their connection status.",
		Stability: "stable",
		CLI: &CLIBinding{
			Usage: "sf-deck org list --json",
		},
	},
	{
		Noun: "org", Verb: "switch",
		Summary:   "Switch the active org in the running TUI.",
		Stability: "stable",
		IPC: &IPCBinding{
			Command: "org.switch",
			Args: []FieldSpec{
				{Name: "org_user", Type: "string", Description: "canonical username; either this or alias"},
				{Name: "alias", Type: "string", Description: "alias; either this or org_user"},
			},
		},
	},
	{
		Noun: "org", Verb: "safety.get",
		Summary:   "Read effective + override safety level for an org.",
		Stability: "stable",
		CLI: &CLIBinding{
			Usage: "sf-deck org safety get --org <alias> --json",
			Flags: []FlagSpec{
				{Name: "org", Type: "string", Description: "alias or username"},
			},
		},
		IPC: &IPCBinding{
			Command: "org.safety.get",
			Args: []FieldSpec{
				{Name: "org_alias", Type: "string"},
				{Name: "org_user", Type: "string"},
			},
		},
	},
	{
		Noun: "org", Verb: "safety.set",
		Summary:   "Set or clear per-org safety override.",
		Stability: "stable",
		CLI: &CLIBinding{
			Usage: "sf-deck org safety set --org <alias> --level metadata --json",
			Flags: []FlagSpec{
				{Name: "org", Type: "string", Description: "alias or username"},
				{Name: "level", Type: "string", Description: "read_only|records|metadata|full"},
				{Name: "clear", Type: "bool", Description: "remove override, revert to defaults"},
			},
		},
		IPC: &IPCBinding{
			Command: "org.safety.set",
			Args: []FieldSpec{
				{Name: "org_alias", Type: "string"},
				{Name: "org_user", Type: "string"},
				{Name: "level", Type: "string"},
				{Name: "clear", Type: "bool"},
			},
		},
	},

	// ===== project ==================================================
	{
		Noun: "project", Verb: "list",
		Summary:   "List all DevProjects.",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck project list --json"},
		IPC:       &IPCBinding{Command: "project.list"},
	},
	{
		Noun: "project", Verb: "show",
		Summary:   "Show one DevProject by id or name.",
		Stability: "stable",
		CLI: &CLIBinding{
			Usage: "sf-deck project show --id <id> --json",
			Flags: []FlagSpec{
				{Name: "id", Type: "string"},
				{Name: "name", Type: "string"},
			},
		},
		IPC: &IPCBinding{
			Command: "project.show",
			Args: []FieldSpec{
				{Name: "id", Type: "string"},
				{Name: "name", Type: "string"},
			},
		},
	},
	{
		Noun: "project", Verb: "create",
		Summary:   "Create a new DevProject.",
		Stability: "stable",
		CLI: &CLIBinding{
			Usage: "sf-deck project create --name <n> --description <d> --json",
			Flags: []FlagSpec{
				{Name: "name", Type: "string", Required: true},
				{Name: "description", Type: "string"},
			},
		},
		IPC: &IPCBinding{
			Command: "project.create",
			Args: []FieldSpec{
				{Name: "name", Type: "string", Required: true},
				{Name: "description", Type: "string"},
			},
		},
	},
	{
		Noun: "project", Verb: "update",
		Summary:   "Rename or re-describe a DevProject.",
		Stability: "stable",
		CLI: &CLIBinding{
			Usage: "sf-deck project update --id <id> [--name <n>] [--description <d>] --json",
		},
		IPC: &IPCBinding{Command: "project.update"},
	},
	{
		Noun: "project", Verb: "delete",
		Summary:   "Delete a DevProject (with --force to cascade items).",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck project delete --id <id> [--force] --json"},
		IPC:       &IPCBinding{Command: "project.delete"},
	},
	{
		Noun: "project", Verb: "items",
		Summary:   "List items in a DevProject (optionally filtered to one org).",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck project items --id <id> [--org-user <u>] --json"},
		IPC:       &IPCBinding{Command: "project.items"},
	},
	{
		Noun: "project", Verb: "add-item",
		Summary:   "Add a single item (flow/field/class/etc.) to a DevProject.",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck project add-item --project-id <id> --kind flow --ref <name> [--org-user <u>] --json"},
		IPC:       &IPCBinding{Command: "project.add-item"},
	},
	{
		Noun: "project", Verb: "remove-item",
		Summary:   "Remove an item from a DevProject.",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck project remove-item --project-id <id> --kind flow --ref <name> --json"},
		IPC:       &IPCBinding{Command: "project.remove-item"},
	},
	{
		Noun: "project", Verb: "import-bundle",
		Summary:   "Parse a package.xml + add each member as a DevProject item.",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck project import-bundle --project-id <id> --path <dir> [--org <a>] --json"},
		IPC:       &IPCBinding{Command: "project.import-bundle"},
	},
	{
		Noun: "project", Verb: "load",
		Summary:   "Make a DevProject the active context in the TUI.",
		Stability: "stable",
		IPC:       &IPCBinding{Command: "project.load"},
	},
	{
		Noun: "project", Verb: "unload",
		Summary:   "Clear the active DevProject context in the TUI.",
		Stability: "stable",
		IPC:       &IPCBinding{Command: "project.unload"},
	},

	// ===== bundle ===================================================
	{
		Noun: "bundle", Verb: "list",
		Summary:   "List bundles (optionally for one DevProject).",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck bundle list [--project-id <id>] --json"},
		IPC:       &IPCBinding{Command: "bundle.list"},
	},
	{
		Noun: "bundle", Verb: "show",
		Summary:   "Show one bundle.",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck bundle show --id <bundle-id> --json"},
		IPC:       &IPCBinding{Command: "bundle.show"},
	},
	{
		Noun: "bundle", Verb: "create",
		Summary:   "Scaffold a new sfdx project + (optionally) retrieve metadata from an org.",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck bundle create --project-id <id> --org <alias> [--path <dir>] [--retrieve=false] --json"},
		IPC:       &IPCBinding{Command: "bundle.create"},
	},
	{
		Noun: "bundle", Verb: "link",
		Summary:   "Register an existing sfdx project directory as a bundle without overwriting it.",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck bundle link --project-id <id> --path <dir> [--org <a>] --json"},
		IPC:       &IPCBinding{Command: "bundle.link"},
	},
	{
		Noun: "bundle", Verb: "retrieve",
		Summary:   "Pull source from the org into the bundle's working directory.",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck bundle retrieve --id <bundle-id> --org <alias> --json"},
		IPC:       &IPCBinding{Command: "bundle.retrieve"},
	},
	{
		Noun: "bundle", Verb: "validate",
		Summary:   "Check-only deploy (validation rules + Apex tests).",
		Safety:    SafetyMetadata,
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck bundle validate --id <bundle-id> --org <alias> [--async] [--tests <level>] --json"},
		IPC: &IPCBinding{
			Command: "bundle.validate",
			Async:   true,
		},
	},
	{
		Noun: "bundle", Verb: "deploy",
		Summary:   "Real deploy. Same async/tests flags as validate.",
		Safety:    SafetyMetadata,
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck bundle deploy --id <bundle-id> --org <alias> [--async] [--tests <level>] --json"},
		IPC: &IPCBinding{
			Command: "bundle.deploy",
			Async:   true,
		},
	},
	{
		Noun: "bundle", Verb: "report",
		Summary:   "Poll an async validate/deploy job by DeployRequest.Id.",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck bundle report --id <bundle-id> --org <alias> --deploy-id <0Af...> --json"},
		IPC:       &IPCBinding{Command: "bundle.report"},
	},
	{
		Noun: "bundle", Verb: "delete",
		Summary:   "Unlink a bundle row (does not touch the on-disk directory).",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck bundle delete --id <bundle-id> --json"},
		IPC:       &IPCBinding{Command: "bundle.delete"},
	},

	// ===== soql =====================================================
	{
		Noun: "soql", Verb: "run",
		Summary:   "Execute a SOQL query and return records.",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck soql run --org <alias> --query <q> [--tooling] [--limit N] --json"},
		IPC: &IPCBinding{
			Command: "soql.run",
			Args: []FieldSpec{
				{Name: "org_alias", Type: "string", Description: "target org alias"},
				{Name: "org_user", Type: "string", Description: "target org username (alternative to org_alias)"},
				{Name: "query", Type: "string", Description: "SOQL string (or use query_file)"},
				{Name: "query_file", Type: "string", Description: "path to a file with the SOQL ('-' for stdin)"},
				{Name: "tooling", Type: "bool", Description: "run against the Tooling API"},
				{Name: "limit", Type: "int", Description: "max rows (0 = default cap)"},
			},
		},
	},
	{
		Noun: "soql", Verb: "seed",
		Summary:   "Push a query into the TUI editor (optional auto-run).",
		Stability: "stable",
		IPC: &IPCBinding{
			Command: "soql.seed",
			Args: []FieldSpec{
				{Name: "query", Type: "string", Required: true},
				{Name: "open", Type: "bool", Description: "navigate to /soql first (default true)"},
				{Name: "run", Type: "bool", Description: "also fire the query immediately"},
			},
		},
		Notes: "IPC-only — pushes into the live TUI's textarea.",
	},
	{
		Noun: "soql", Verb: "export",
		Summary:   "Run a query + export the result to CSV/XLSX/JSON.",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck soql export --org <a> --query <q> --output <path> --format csv|xlsx|json [--force] --json"},
		Notes:     "Refuses to overwrite an existing output unless --force is supplied.",
	},
	{
		Noun: "soql", Verb: "history.list",
		Summary:   "Recent SOQL runs from soql_history.",
		Stability: "stable",
		IPC:       &IPCBinding{Command: "soql.history.list"},
		Notes:     "IPC-only — no CLI counterpart yet.",
	},
	{
		Noun: "soql", Verb: "saved.list",
		Summary:   "List saved queries.",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck soql saved list --json"},
		IPC:       &IPCBinding{Command: "soql.saved.list"},
	},
	{
		Noun: "soql", Verb: "saved.show",
		Summary:   "Show a saved query by id or name.",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck soql saved show --id <id> --json"},
		IPC:       &IPCBinding{Command: "soql.saved.show"},
	},
	{
		Noun: "soql", Verb: "saved.create",
		Summary:   "Persist a new saved query.",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck soql saved create --name <n> --query <q> [--description <d>] --json"},
		IPC:       &IPCBinding{Command: "soql.saved.create"},
	},
	{
		Noun: "soql", Verb: "saved.update",
		Summary:   "Patch a saved query (name/body/description).",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck soql saved update --id <id> [--name <n>] [--query <q>] [--description <d>] --json"},
		IPC:       &IPCBinding{Command: "soql.saved.update"},
	},
	{
		Noun: "soql", Verb: "saved.delete",
		Summary:   "Remove a saved query.",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck soql saved delete --id <id> --json"},
		IPC:       &IPCBinding{Command: "soql.saved.delete"},
	},

	// ===== apex =====================================================
	{
		Noun: "apex", Verb: "execute",
		Summary:   "Run anonymous Apex.",
		Safety:    SafetyFull,
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck apex execute --org <a> --body \"<apex>\" --json"},
	},
	{
		Noun: "apex", Verb: "run",
		Summary:   "Run anonymous Apex (IPC verb name; same as CLI's apex execute).",
		Safety:    SafetyFull,
		Stability: "stable",
		IPC:       &IPCBinding{Command: "apex.run"},
	},
	{
		Noun: "apex", Verb: "snippet",
		Summary:   "Manage saved Apex snippets (list/show/create/update/delete/run).",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck apex snippet list --json"},
	},

	// ===== record ===================================================
	{
		Noun: "record", Verb: "get",
		Summary:   "Fetch one record by sobject + id.",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck record get --org <a> --object Account --id <id> --json"},
		IPC:       &IPCBinding{Command: "record.get"},
	},
	{
		Noun: "record", Verb: "recent",
		Summary:   "Recent records for an sobject (read-only).",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck record recent --org <a> --object Account [--limit 50] --json"},
		IPC:       &IPCBinding{Command: "record.recent"},
	},
	{
		Noun: "record", Verb: "create",
		Summary:   "Insert a new record.",
		Safety:    SafetyRecords,
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck record create --org <a> --object Account --field Name=Acme --json"},
		IPC: &IPCBinding{
			Command: "record.create",
			Args: []FieldSpec{
				{Name: "org_alias", Type: "string", Description: "target org alias"},
				{Name: "org_user", Type: "string", Description: "target org username (alternative to org_alias)"},
				{Name: "sobject", Type: "string", Required: true, Description: "sObject API name"},
				{Name: "fields", Type: "object", Required: true, Description: "field name -> value map for the new record"},
			},
		},
	},
	{
		Noun: "record", Verb: "update",
		Summary:   "Patch a record's fields.",
		Safety:    SafetyRecords,
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck record update --org <a> --id <id> --field Phone=555-1234 --json"},
		IPC: &IPCBinding{
			Command: "record.update",
			Args: []FieldSpec{
				{Name: "org_alias", Type: "string", Description: "target org alias"},
				{Name: "org_user", Type: "string", Description: "target org username (alternative to org_alias)"},
				{Name: "sobject", Type: "string", Required: true, Description: "sObject API name"},
				{Name: "id", Type: "string", Required: true, Description: "record id to patch"},
				{Name: "fields", Type: "object", Required: true, Description: "field name -> value map of changes"},
			},
		},
	},
	{
		Noun: "record", Verb: "delete",
		Summary:   "Delete a record by id.",
		Safety:    SafetyRecords,
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck record delete --org <a> --id <id> --json"},
		IPC: &IPCBinding{
			Command: "record.delete",
			Args: []FieldSpec{
				{Name: "org_alias", Type: "string", Description: "target org alias"},
				{Name: "org_user", Type: "string", Description: "target org username (alternative to org_alias)"},
				{Name: "sobject", Type: "string", Required: true, Description: "sObject API name"},
				{Name: "id", Type: "string", Required: true, Description: "record id to delete"},
			},
		},
	},

	// ===== metadata =================================================
	{
		Noun: "metadata", Verb: "get",
		Summary:   "Read a Tooling sobject row's Metadata map.",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck metadata get --org <a> --type CustomField --id <id> --json"},
		IPC:       &IPCBinding{Command: "metadata.get"},
	},
	{
		Noun: "metadata", Verb: "create",
		Summary:   "Create a new Tooling sobject row.",
		Safety:    SafetyMetadata,
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck metadata create --org <a> --type ValidationRule --full-name <n> --patch <json> --json"},
		IPC:       &IPCBinding{Command: "metadata.create"},
	},
	{
		Noun: "metadata", Verb: "update",
		Summary:   "Patch the Metadata of an existing Tooling row.",
		Safety:    SafetyMetadata,
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck metadata update --org <a> --type <t> --id <id> --patch <json> --json"},
		IPC:       &IPCBinding{Command: "metadata.update"},
	},
	{
		Noun: "metadata", Verb: "delete",
		Summary:   "Delete a Tooling row by id.",
		Safety:    SafetyFull,
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck metadata delete --org <a> --type <t> --id <id> --json"},
		IPC:       &IPCBinding{Command: "metadata.delete"},
	},

	// ===== object ===================================================
	{
		Noun: "object", Verb: "describe",
		Summary:   "Return the cached SObjectDescribe for an sobject.",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck object describe --org <a> --sobject Account --json"},
		IPC:       &IPCBinding{Command: "object.describe"},
	},

	// ===== report ===================================================
	{
		Noun: "report", Verb: "list",
		Summary:   "List reports (with optional name/folder filters).",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck report list --org <a> [--contains <s>] [--folder <f>] --json"},
		IPC:       &IPCBinding{Command: "report.list"},
	},
	{
		Noun: "report", Verb: "run",
		Summary:   "Execute a report (synchronous; cached result by default).",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck report run --org <a> --id <report-id> [--force-rerun] --json"},
		IPC:       &IPCBinding{Command: "report.run"},
	},
	{
		Noun: "report", Verb: "export",
		Summary:   "Export a report to XLSX.",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck report export --org <a> --id <report-id> --output <path.xlsx> [--view formatted|details] [--force] --json"},
		Notes:     "Refuses to overwrite an existing output unless --force is supplied.",
	},

	// ===== chip =====================================================
	{
		Noun: "chip", Verb: "list",
		Summary:   "List all defined chips.",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck chip list --json"},
	},
	{
		Noun: "chip", Verb: "show",
		Summary:   "Show one chip by id.",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck chip show --id <chip-id> --json"},
	},
	{
		Noun: "chip", Verb: "create",
		Summary:   "Create a new chip (filter view) in settings.toml.",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck chip create --id <id> --domain <d> --columns <cols> --clauses <c> --json"},
	},
	{
		Noun: "chip", Verb: "update",
		Summary:   "Patch a chip's columns/clauses/label.",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck chip update --id <id> [--columns <c>] [--clauses <c>] --json"},
	},
	{
		Noun: "chip", Verb: "delete",
		Summary:   "Remove a chip from settings.toml.",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck chip delete --id <id> --json"},
	},
	{
		Noun: "chip", Verb: "favourite",
		Summary:   "Toggle a chip's favourite flag.",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck chip favourite --id <id> --value true --json"},
	},
	{
		Noun: "chip", Verb: "columns",
		Summary:   "Update a chip's column ordering.",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck chip columns --id <id> --columns A,B,C --json"},
	},
	{
		Noun: "chip", Verb: "apply",
		Summary:   "Apply a chip in the live TUI (set as active view).",
		Stability: "stable",
		IPC:       &IPCBinding{Command: "chip.apply"},
		Notes:     "IPC-only — applies to a running TUI's view state.",
	},
	{
		Noun: "chip", Verb: "preview",
		Summary:   "Drop a session-only chip (ephemeral) onto the strip.",
		Stability: "stable",
		IPC:       &IPCBinding{Command: "chip.preview"},
	},
	{
		Noun: "chip", Verb: "preview.save",
		Summary:   "Promote a previewed chip to a persistent settings.toml entry.",
		Stability: "stable",
		IPC:       &IPCBinding{Command: "chip.preview.save"},
	},
	{
		Noun: "chip", Verb: "preview.dismiss",
		Summary:   "Drop a previewed chip without saving.",
		Stability: "stable",
		IPC:       &IPCBinding{Command: "chip.preview.dismiss"},
	},

	// ===== tag ======================================================
	{
		Noun: "tag", Verb: "list",
		Summary:   "List all tags (optionally only ones in use).",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck tag list [--usage-only] --json"},
		IPC:       &IPCBinding{Command: "tag.list"},
	},
	{
		Noun: "tag", Verb: "show",
		Summary:   "Show a tag by id or name.",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck tag show --id <id> --json"},
		IPC:       &IPCBinding{Command: "tag.show"},
	},
	{
		Noun: "tag", Verb: "create",
		Summary:   "Create a new tag (name + optional color/icon).",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck tag create --name <n> [--color <c>] [--icon <i>] --json"},
		IPC:       &IPCBinding{Command: "tag.create"},
	},
	{
		Noun: "tag", Verb: "update",
		Summary:   "Patch a tag's name/color/icon.",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck tag update --id <id> [--name <n>] [--color <c>] [--icon <i>] --json"},
		IPC:       &IPCBinding{Command: "tag.update"},
	},
	{
		Noun: "tag", Verb: "delete",
		Summary:   "Remove a tag (cascades unbindings).",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck tag delete --id <id> --json"},
		IPC:       &IPCBinding{Command: "tag.delete"},
	},
	{
		Noun: "tag", Verb: "apply",
		Summary:   "Bind a tag to one (kind, ref, org_user) item.",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck tag apply --id <tag-id> --kind flow --ref <name> [--org-user <u>] --json"},
		IPC:       &IPCBinding{Command: "tag.apply"},
	},
	{
		Noun: "tag", Verb: "remove",
		Summary:   "Unbind one tag from one item.",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck tag remove --id <tag-id> --kind flow --ref <name> --json"},
		IPC:       &IPCBinding{Command: "tag.remove"},
	},
	{
		Noun: "tag", Verb: "set",
		Summary:   "Replace the full tag set on one item with the supplied list.",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck tag set --kind flow --ref <name> --ids 1,3,5 --json"},
		IPC:       &IPCBinding{Command: "tag.set"},
	},
	{
		Noun: "tag", Verb: "items",
		Summary:   "List items currently tagged with the supplied tag.",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck tag items --id <tag-id> --json"},
	},
	{
		Noun: "tag", Verb: "of",
		Summary:   "List tags applied to one (kind, ref, org_user) item.",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck tag of --kind flow --ref <name> --json"},
	},

	// ===== instance =================================================
	{
		Noun: "instance", Verb: "list",
		Summary:   "List running sf-deck instances + their control sockets.",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck instance list --json"},
	},
	{
		Noun: "instance", Verb: "kill",
		Summary:   "Send SIGTERM to a running sf-deck instance.",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck instance kill --number <n> --json"},
	},

	// ===== update ===================================================
	{
		Noun: "update", Verb: "check",
		Summary:   "Check GitHub Releases for a newer stable sf-deck version.",
		Stability: "stable",
		CLI: &CLIBinding{
			Usage: "sf-deck update check [--force] --json",
			Flags: []FlagSpec{
				{Name: "force", Type: "bool", Description: "bypass the 24-hour release cache"},
			},
		},
		Notes: "Read-only and notification-only: it never downloads or installs a release.",
	},

	// ===== notification =============================================
	{
		Noun: "notification", Verb: "send",
		Summary:   "Send a desktop notification via the configured backend.",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck notification send --title <t> --body <b> --json"},
	},

	// ===== verbs (introspection) ====================================
	{
		Noun: "verbs", Verb: "list",
		Summary:   "Return the full verb registry — single source of truth.",
		Stability: "stable",
		CLI:       &CLIBinding{Usage: "sf-deck verbs list [--surface cli|ipc|tui] --json"},
		IPC: &IPCBinding{
			Command: "verbs.list",
			Args: []FieldSpec{
				{Name: "surface", Type: "string", Description: "filter to cli/ipc/tui (empty = all)"},
			},
		},
		Notes: "Agents call this to discover what sf-deck can do without parsing docs.",
	},
}
