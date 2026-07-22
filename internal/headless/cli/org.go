package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/app"
	"github.com/Jacob-Stokes/sf-deck/internal/headless"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// orgSummary is the headless-facing view of one connected org. The
// underlying sf.Org carries fields that aren't useful in JSON output
// (LastUsed string, internal flags); we project to just what an agent
// or script consumer cares about.
type orgSummary struct {
	Alias       string `json:"alias,omitempty"`
	Username    string `json:"username"`
	OrgID       string `json:"org_id,omitempty"`
	Kind        string `json:"kind"` // Production / Sandbox / Scratch / DevHub
	InstanceURL string `json:"instance_url,omitempty"`
	Status      string `json:"status,omitempty"`
	Safety      string `json:"safety"` // resolved effective safety level
}

func dispatchOrg(a *app.App, args Args, stdout io.Writer, mode headless.WriteMode) int {
	verb := args.Verb
	if verb == "" {
		verb = "list"
	}
	switch verb {
	case "list":
		return orgList(a, args.Rest, stdout, mode)
	case "show":
		return orgShow(a, args.Rest, stdout, mode)
	case "safety":
		// `org safety <subverb>` — nested noun-verb for the safety
		// inspector / setter pair.
		return orgSafety(a, args.Rest, stdout, mode)
	}
	r := headless.Fail("org."+verb, "", headless.ErrInvalidArgument,
		fmt.Sprintf("unknown org verb %q", verb), nil)
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

// summariseOrg builds the public view, attaching the effective safety
// level so callers can see the policy without a second roundtrip.
func summariseOrg(a *app.App, o sf.Org) orgSummary {
	return orgSummary{
		Alias:       o.Alias,
		Username:    o.Username,
		OrgID:       o.OrgID,
		Kind:        o.Kind(),
		InstanceURL: o.InstanceURL,
		Status:      o.Status,
		Safety:      a.SafetyFor(o).String(),
	}
}

func orgList(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("org list")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("org.list", err, stdout, mode)
	}
	out := make([]orgSummary, 0, len(a.Orgs))
	for _, o := range a.Orgs {
		out = append(out, summariseOrg(a, o))
	}
	r := headless.Success("org.list", "", "", false, map[string]any{
		"orgs":  out,
		"count": len(out),
	})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func orgShow(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("org show")
	target := fs.String("org", "", "Alias or username (empty = default)")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("org.show", err, stdout, mode)
	}
	o, err := a.ResolveOrg(*target)
	if err != nil {
		return writeOrgErr("org.show", *target, err, stdout, mode)
	}
	r := headless.Success("org.show", o.Username, app.TargetArg(o), false,
		map[string]any{"org": summariseOrg(a, o)})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

// orgSafety routes `org safety <get|set>`.
func orgSafety(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	subverb := ""
	if len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
		subverb = rest[0]
		rest = rest[1:]
	}
	if subverb == "" {
		subverb = "get"
	}
	switch subverb {
	case "get":
		return orgSafetyGet(a, rest, stdout, mode)
	case "set":
		return orgSafetySet(a, rest, stdout, mode)
	}
	r := headless.Fail("org.safety."+subverb, "", headless.ErrInvalidArgument,
		fmt.Sprintf("unknown safety subverb %q (want get|set)", subverb), nil)
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func orgSafetyGet(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("org safety get")
	target := fs.String("org", "", "Alias or username (empty = default)")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("org.safety.get", err, stdout, mode)
	}
	o, err := a.ResolveOrg(*target)
	if err != nil {
		return writeOrgErr("org.safety.get", *target, err, stdout, mode)
	}
	level := a.SafetyFor(o)
	// Look up whether the value comes from an explicit override or
	// from kind-defaults — useful for callers who want to know "is
	// this user-set or implicit?".
	override, hasOverride := a.Settings.OrgSafetyOverride(o.Username, o.Alias)
	r := headless.Success("org.safety.get", o.Username, app.TargetArg(o), false,
		map[string]any{
			"org":      summariseOrg(a, o),
			"safety":   level.String(),
			"override": override,
			"explicit": hasOverride,
			"source":   safetySource(hasOverride),
		})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func orgSafetySet(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("org safety set")
	target := fs.String("org", "", "Alias or username (empty = default)")
	levelRaw := fs.String("level", "",
		"Safety level: read_only | records | metadata | full")
	clear := fs.Bool("clear", false,
		"Remove per-org override; revert to kind defaults")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("org.safety.set", err, stdout, mode)
	}
	if *clear && *levelRaw != "" {
		return writeArgErr("org.safety.set",
			errors.New("--clear and --level are mutually exclusive"), stdout, mode)
	}
	if !*clear && *levelRaw == "" {
		return writeArgErr("org.safety.set",
			errors.New("--level or --clear is required"), stdout, mode)
	}
	o, err := a.ResolveOrg(*target)
	if err != nil {
		return writeOrgErr("org.safety.set", *target, err, stdout, mode)
	}

	priorLevel := a.SafetyFor(o)
	priorOverride, hadPriorOverride := a.Settings.OrgSafetyOverride(o.Username)
	restorePrior := func() {
		if hadPriorOverride {
			a.Settings.SetOrg(o.Username, settings.ParseSafetyLevel(priorOverride), false)
		} else {
			a.Settings.SetOrg(o.Username, settings.SafetyReadOnly, true)
		}
	}
	if *clear {
		a.Settings.SetOrg(o.Username, settings.SafetyReadOnly, true)
	} else {
		lvl := settings.ParseSafetyLevel(*levelRaw)
		// ParseSafetyLevel is forgiving (unknown → read_only). Reject
		// unknown values explicitly so the JSON shape signals the
		// caller mistake rather than silently downgrading.
		if !isKnownSafetyString(*levelRaw) {
			return writeArgErr("org.safety.set",
				fmt.Errorf("invalid safety level %q (want read_only|records|metadata|full)", *levelRaw),
				stdout, mode)
		}
		a.Settings.SetOrg(o.Username, lvl, false)
	}
	if a.SaveSettings != nil {
		if err := a.SaveSettings(); err != nil {
			restorePrior()
			return writeArgErr("org.safety.set",
				fmt.Errorf("save settings: %w", err), stdout, mode)
		}
	}
	newLevel := a.SafetyFor(o)
	r := headless.Success("org.safety.set", o.Username, app.TargetArg(o),
		newLevel != priorLevel,
		map[string]any{
			"org":          summariseOrg(a, o),
			"safety":       newLevel.String(),
			"prior_safety": priorLevel.String(),
			"cleared":      *clear,
		})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

// safetySource is a small label for org.safety.get's `source` field:
// "override" (explicit per-org entry in settings.toml) vs "default"
// (resolved from kind-defaults or hardcoded ladder). Helps agents
// answer "should I be careful here?" without grepping the toml.
func safetySource(hasOverride bool) string {
	if hasOverride {
		return "override"
	}
	return "default"
}

// isKnownSafetyString gates org.safety.set so unknown strings fail
// loudly rather than silently downgrading to read_only via
// ParseSafetyLevel's forgiving fallback.
func isKnownSafetyString(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "read_only", "readonly", "ro",
		"records", "rec",
		"metadata", "meta",
		"full":
		return true
	}
	return false
}

// writeOrgErr maps app.ResolveOrg errors into typed JSON. Currently
// the only typed error is "not found"; everything else (e.g. "no orgs
// connected") becomes invalid_argument.
func writeOrgErr(command, target string, err error, stdout io.Writer, mode headless.WriteMode) int {
	msg := err.Error()
	if strings.Contains(msg, "not found") {
		r := headless.Fail(command, "", headless.ErrNotFound, msg,
			map[string]any{"target": target})
		_ = r.Write(stdout, mode)
		return headless.ExitCodeFor(r)
	}
	return writeArgErr(command, err, stdout, mode)
}
