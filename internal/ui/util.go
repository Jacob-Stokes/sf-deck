package ui

// Tiny string / model utilities that don't fit anywhere else.

import (
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// flash sets a short-lived banner that shows in the status bar for a
// few seconds. Overwrites any previous flash.
func (m *Model) flash(msg string) {
	m.flashFor(msg, 3*time.Second)
}

// saveSettings persists settings changes and reports failures through
// the status flash. When successMsg is non-empty it is flashed only
// after the write succeeds, so user-facing toggles do not promise a
// persisted change that failed on disk.
func (m *Model) saveSettings(successMsg string) bool {
	if m.settings == nil {
		return false
	}
	if err := m.settings.Save(); err != nil {
		m.flash("settings save failed: " + err.Error())
		return false
	}
	if successMsg != "" {
		m.flash(successMsg)
	}
	return true
}

// anyModalActive reports whether any overlay/modal currently owns
// the foreground. Used to route mouse wheel events into the modal's
// list (via synthesized arrow keys) instead of the surface behind
// it. Mirrors the precedence chain in handleKey — anything that's
// listed there as taking input precedence belongs here too.
func (m Model) anyModalActive() bool {
	return m.picker != nil ||
		m.soqlModal != nil ||
		m.themePicker != nil ||
		m.editModal != nil ||
		m.cacheSettings != nil ||
		m.compareEdit != nil ||
		m.compareScope != nil ||
		m.chipWizard != nil ||
		m.openMenu != nil ||
		m.orgPicker != nil ||
		m.deepCollect != nil ||
		m.choiceModal != nil ||
		m.commandPalette != nil ||
		m.keybindingsModal != nil ||
		m.tagPicker != nil ||
		m.tagEditor != nil ||
		m.globalSearch != nil ||
		m.downloadsModal != nil ||
		m.infoModal != nil ||
		m.orgManageModal != nil ||
		m.exportSave != nil
}

// flashFor is flash with an explicit duration — longer for messages
// the user actually needs to read (multi-sentence errors, paths to
// dump files) instead of the default 3s tick.
func (m *Model) flashFor(msg string, d time.Duration) {
	m.banner = msg
	m.bannerUntil = time.Now().Add(d)
}

// onOff formats a bool as "on"/"off" for user-facing status lines.
func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// itoa is a tiny allocation-free int-to-string for positive integers.
// Used in hot paths (breadcrumbs, list row labels) where the extra
// cost of fmt.Sprintf isn't warranted.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	sign := ""
	if n < 0 {
		sign = "-"
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return sign + string(buf[i:])
}

// resourceFetchErrorMsg formats a fetch error for the status flash.
// key is the resource Key — e.g. "apex_triggers_flat", "describe:Account",
// "flowversions:0EZ…" — which we trim to the human-friendly bit before
// the colon so it reads as "triggers: <error>" rather than "apex_triggers_flat: <…>".
func resourceFetchErrorMsg(key string, err error) string {
	label := key
	if i := strings.IndexByte(key, ':'); i >= 0 {
		label = key[:i]
	}
	// Salesforce errors can be very long; cap to fit a status flash.
	msg := err.Error()
	const maxLen = 140
	if len(msg) > maxLen {
		msg = ansi.Truncate(msg, maxLen, "…")
	}
	return label + ": " + msg
}
