package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/Jacob-Stokes/sf-deck/internal/app"
	"github.com/Jacob-Stokes/sf-deck/internal/headless"
	"github.com/Jacob-Stokes/sf-deck/internal/services/notificationops"
	"github.com/Jacob-Stokes/sf-deck/internal/services/orgwrite"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// dispatchNotification routes `sf-deck notification <verb>`. The
// `list` verb is read-only; `mark-read` is a write that flips state on
// the user's notification feed and is gated at WriteRecord — it's
// per-user state, not metadata, but it's still a write.
func dispatchNotification(a *app.App, args Args, stdout io.Writer, mode headless.WriteMode) int {
	verb := args.Verb
	if verb == "" {
		verb = "list"
	}
	switch verb {
	case "list":
		return notificationList(a, args.Rest, stdout, mode)
	case "mark-read":
		return notificationMarkRead(a, args.Rest, stdout, mode)
	}
	r := headless.Fail("notification."+verb, "", headless.ErrInvalidArgument,
		fmt.Sprintf("unknown notification verb %q", verb), nil)
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func notificationList(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("notification list")
	target := fs.String("org", "", "Alias or username (empty = default)")
	limit := fs.Int("limit", 50, "Max rows to return")
	unreadOnly := fs.Bool("unread-only", false,
		"Return only notifications where read=false")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("notification.list", err, stdout, mode)
	}
	if *limit <= 0 {
		return writeArgErr("notification.list",
			errors.New("--limit must be positive"), stdout, mode)
	}
	o, err := a.ResolveOrg(*target)
	if err != nil {
		return writeOrgErr("notification.list", *target, err, stdout, mode)
	}
	list, err := sf.ListNotifications(app.TargetArg(o), *limit)
	if err != nil {
		return writeSOQLErr("notification.list", o.Username, err, stdout, mode)
	}
	out := list.Notifications
	if *unreadOnly {
		filtered := make([]sf.Notification, 0, len(out))
		for _, n := range out {
			if !n.Read {
				filtered = append(filtered, n)
			}
		}
		out = filtered
	}
	r := headless.Success("notification.list", o.Username, app.TargetArg(o), false,
		map[string]any{
			"notifications": out,
			"count":         len(out),
			"total":         len(list.Notifications),
		})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func notificationMarkRead(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("notification mark-read")
	target := fs.String("org", "", "Alias or username (empty = default)")
	id := fs.String("id", "", "Notification id")
	all := fs.Bool("all", false, "Mark every notification read in one call")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("notification.mark-read", err, stdout, mode)
	}
	if *id == "" && !*all {
		return writeArgErr("notification.mark-read",
			errors.New("--id or --all is required"), stdout, mode)
	}
	if *id != "" && *all {
		return writeArgErr("notification.mark-read",
			errors.New("--id and --all are mutually exclusive"), stdout, mode)
	}
	result, err := a.NotificationWrites().MarkRead(context.Background(), notificationops.MarkReadInput{
		Target: *target, ID: *id, All: *all,
	})
	if err != nil {
		return writeNotificationOperationErr(*target, result.Target, err, stdout, mode)
	}
	r := headless.Success("notification.mark-read", result.Target.Username, result.Target.CLIArg, true,
		map[string]any{
			"id":  *id,
			"all": *all,
		})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func writeNotificationOperationErr(requestedTarget string, target orgwrite.Target, err error,
	stdout io.Writer, mode headless.WriteMode) int {
	var blocked app.BlockedError
	if errors.As(err, &blocked) {
		return writeSafetyBlocked("notification.mark-read", blocked.Username, blocked, stdout, mode)
	}
	var resolveErr orgwrite.ResolutionError
	if errors.As(err, &resolveErr) {
		return writeOrgErr("notification.mark-read", resolveErr.Target, resolveErr.Err, stdout, mode)
	}
	orgUser := target.Username
	if orgUser == "" {
		orgUser = requestedTarget
	}
	return writeSOQLErr("notification.mark-read", orgUser, err, stdout, mode)
}
