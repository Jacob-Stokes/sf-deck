// Package verbs is the single source of truth for sf-deck's
// command surface. Every CLI noun.verb and every IPC command is
// declared here as a Spec; CLI dispatch + IPC dispatch + skill
// doc generation all consume this same registry so adding or
// renaming a verb is a one-edit operation.
//
// Strict parity: a Spec that declares a CLI binding MUST have a
// dispatch entry; a Spec that declares an IPC binding MUST have a
// Backend method. The drift test in verbs_test.go fails when this
// invariant breaks.
//
// Layout: one Spec per (noun, verb) operation. CLI and IPC bindings
// hang off the Spec — when both are populated, they represent the
// same logical operation exposed over two transports. When only one
// is populated, it's intentional (e.g. soql.seed is IPC-only since
// it pushes into the TUI editor; instance.list is CLI-only since
// the IPC instance IS the running process).
//
// Stability tiers:
//   - "stable"     : public contract, semver-protected
//   - "experimental": may change without notice; surfaced to agents
//   - "deprecated" : still works, has a replacement
package verbs

import "sort"

// Surface is the discoverability layer; agents can filter to "what
// can I drive over IPC?" or "what CLI nouns exist?" via Specs() +
// these constants.
type Surface string

const (
	SurfaceCLI Surface = "cli"
	SurfaceIPC Surface = "ipc"
	SurfaceTUI Surface = "tui" // verbs that only make sense in the TUI; not reachable via CLI or IPC
)

// Safety mirrors settings.SafetyLevel but lives here so the verbs
// package has zero dependencies on internal/settings. Callers
// translate to settings.SafetyLevel when wiring the gate.
type Safety string

const (
	SafetyReadOnly Safety = "read_only"
	SafetyRecords  Safety = "records"
	SafetyMetadata Safety = "metadata"
	SafetyFull     Safety = "full" // includes execute-anonymous Apex + destructive ops
)

// NOTE: there are exactly FOUR safety levels — mirroring
// settings.SafetyLevel. There is no separate "anonymous" tier;
// anonymous Apex is gated by SafetyFull (see settings/safety.go). The
// verb registry, CLI (org.safety.set), and agent skill must all agree
// on this set, or agents get a false contract.

// Spec describes one logical operation. Bindings may be nil when
// the operation is intentionally not exposed on that surface.
type Spec struct {
	Noun      string // "soql", "bundle", "project", ...
	Verb      string // "run", "create", "list", ...
	Summary   string // one-line description for help text + docs
	Safety    Safety // gate level; empty = read-only
	Stability string // "stable" / "experimental" / "deprecated"
	CLI       *CLIBinding
	IPC       *IPCBinding
	TUIOnly   bool   // when true, no CLI or IPC binding exists by design
	Notes     string // optional longer-form note ("when to drop to CLI", etc.)
}

// CLIBinding describes how the verb is reached via the CLI.
type CLIBinding struct {
	Usage    string // single-line usage string for help
	Flags    []FlagSpec
	Examples []string
}

// IPCBinding describes how the verb is reached via the IPC socket.
type IPCBinding struct {
	Command  string // the command literal sent in the JSON request envelope
	Args     []FieldSpec
	Examples []string
	Async    bool // for long-running ops where the response carries a job id
}

// FlagSpec describes one CLI flag.
type FlagSpec struct {
	Name        string
	Type        string // "string", "int", "bool", "string-list" (for repeated flags)
	Required    bool
	Description string
}

// FieldSpec describes one JSON arg field in an IPC request.
type FieldSpec struct {
	Name        string // JSON key
	Type        string // "string", "int", "bool", "object", "array<string>"
	Required    bool
	Description string
}

// Qualified returns the noun.verb canonical name.
func (s Spec) Qualified() string {
	if s.Verb == "" {
		return s.Noun
	}
	return s.Noun + "." + s.Verb
}

// HasCLI / HasIPC are convenience predicates for filter logic.
func (s Spec) HasCLI() bool { return s.CLI != nil }
func (s Spec) HasIPC() bool { return s.IPC != nil }

// Specs returns a copy of the registry sorted by qualified name so
// every caller sees stable ordering. Doc generation + the
// verbs.list verb both consume this directly.
func Specs() []Spec {
	out := make([]Spec, len(registry))
	copy(out, registry)
	sort.Slice(out, func(i, j int) bool {
		return out[i].Qualified() < out[j].Qualified()
	})
	return out
}

// ByQualified is a tiny lookup helper for "given soql.run, give me
// the Spec". Returns (Spec{}, false) on miss.
func ByQualified(name string) (Spec, bool) {
	for _, s := range registry {
		if s.Qualified() == name {
			return s, true
		}
	}
	return Spec{}, false
}

// SpecsForSurface filters the registry to verbs that have a binding
// on the requested surface. SurfaceTUI returns specs with TUIOnly =
// true.
func SpecsForSurface(surf Surface) []Spec {
	all := Specs()
	out := make([]Spec, 0, len(all))
	for _, s := range all {
		switch surf {
		case SurfaceCLI:
			if s.HasCLI() {
				out = append(out, s)
			}
		case SurfaceIPC:
			if s.HasIPC() {
				out = append(out, s)
			}
		case SurfaceTUI:
			if s.TUIOnly {
				out = append(out, s)
			}
		}
	}
	return out
}
