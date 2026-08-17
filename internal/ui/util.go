package ui

import (
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func (m *Model) flash(msg string) {
	m.flashFor(msg, 3*time.Second)
}

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

func (m *Model) flashFor(msg string, d time.Duration) {
	m.banner = msg
	m.bannerUntil = time.Now().Add(d)
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

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

func resourceFetchErrorMsg(key string, err error) string {
	label := key
	if i := strings.IndexByte(key, ':'); i >= 0 {
		label = key[:i]
	}
	msg := err.Error()
	const maxLen = 140
	if len(msg) > maxLen {
		msg = ansi.Truncate(msg, maxLen, "…")
	}
	return label + ": " + msg
}
