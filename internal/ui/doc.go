// Package ui hosts every part of the TUI: the Bubble Tea model, the
// per-tab renderers, the modal overlays, the chip system, and the
// chrome around them all.
//
// File-naming conventions
// =======================
//
// The package is flat by design — every renderer is a method on
// Model and accesses package-private state, so subdirectory splitting
// would force most identifiers to become exported. Filenames carry
// the structure instead: a stable prefix tells you which surface a
// file contributes to.
//
//	tab_<name>*.go        Renderer for one top-level Tab (or a subtab
//	                      / drill-in within it). Files for the same
//	                      tab family cluster alphabetically, e.g.
//	                      tab_object_*, tab_perm_*, tab_record_*.
//
//	modal_<name>.go       One overlay (cache settings, choice picker,
//	                      edit dialog, theme picker, …). modal.go
//	                      itself owns the shared infrastructure.
//
//	chip_<name>.go        Chip-strip system: helpers, manager, wizard,
//	                      overflow modal, plus chip_strip.go (the
//	                      view-chip strip rendered above lists).
//
//	render_<name>.go      Chrome around the body — header, status bar,
//	                      tab bar, the main-pane dispatcher.
//
//	update_<name>.go      Bubble Tea Update dispatch: keys, navigation
//	                      cursor / drill-in, openable routing.
//
//	tab.go / tab_registry.go / subtab.go / cursors.go / active_state.go
//	                      Cross-cutting orchestrator state.
//
//	model.go              Model struct + per-org orgData.
//
//	*_actions.go          (legacy — being renamed to tab_<name>_actions.go)
//	                      Action-menu registries surfaced in sidebars.
//
//	leftrail.go / sidebar.go / menu.go
//	                      Persistent panes around the body.
//
//	commands.go / messages.go
//	                      Tea-message and Cmd plumbing.
//
//	util.go / format.go / viewport.go / picker.go / value_picker.go
//	                      Tiny helpers used throughout.
//
// Where the surface gets bigger
// =============================
//
// Self-contained subsystems live under their own packages and are
// imported back here. Today:
//
//	internal/ui/keymap     keybinding configuration + parsing
//	internal/ui/qchip      unified chip catalogue (records / objects / flows)
//	internal/ui/resource   Resource[T] + ListView[T] + cursor primitives
//	internal/ui/uilayout   table / list / search-bar render primitives
//	internal/theme         theme tokens + palette
//
// The rule of thumb: if a piece of code can describe its API in a
// short, type-clean surface, it goes in its own package. The
// renderers themselves can't (they all touch private Model state),
// so they stay here, named consistently.
package ui
