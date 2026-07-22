package ui

// Process-wide dependencies + stores threaded into every Model.
//
// Extracted from model.go as the pilot for the Model split documented
// in the 2026-05-11 code review (project vault, architecture/). modelServices is embedded into
// Model so existing field access (m.cache, m.settings, …) keeps
// working unchanged.

import (
	"github.com/Jacob-Stokes/sf-deck/internal/cache"
	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/services/apexops"
	"github.com/Jacob-Stokes/sf-deck/internal/services/bundles"
	"github.com/Jacob-Stokes/sf-deck/internal/services/metadataops"
	"github.com/Jacob-Stokes/sf-deck/internal/services/permissionops"
	"github.com/Jacob-Stokes/sf-deck/internal/services/records"
	"github.com/Jacob-Stokes/sf-deck/internal/services/userops"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
)

// modelServices groups process-wide dependencies and stores.
type modelServices struct {
	cache       *cache.Cache
	settings    *settings.Settings // per-org safety policy; never nil
	devProjects *devproject.Store  // SQLite-backed Dev/Org-projects store; nil when open failed
	apex        *apexops.Service   // safety-enforced anonymous Apex execution
	records     *records.Service   // safety-enforced record create/update/delete
	bundleOps   *bundles.Service   // exact-target bundle retrieve/deploy/validate/report
	permissions *permissionops.Service
	metadata    *metadataops.Service
	metaEditors *metadataops.EditorService
	users       *userops.Service
	exports     *exportRegistry
	// control is the live-IPC bridge. Nil when sf-deck is launched
	// without --control. When set, the update loop publishes snapshots
	// to it after each frame and drains inbound write messages from
	// its Writes() channel.
	control *ControlState
	// instanceNumber is the slot picked at startup via
	// internal/instance. Always populated (no -1 sentinel) since the
	// badge ALWAYS renders — even without --control the user wants
	// to see which window they're looking at. Default 1 when registry
	// claim fails (degenerate case; we don't fail to start over a
	// badge bug).
	instanceNumber int
}

// WriteServices are Salesforce-mutating services injected by main. Keeping
// this independent of app avoids a UI↔app package cycle.
type WriteServices struct {
	Apex            *apexops.Service
	Records         *records.Service
	Bundles         *bundles.Service
	Permissions     *permissionops.Service
	Metadata        *metadataops.Service
	MetadataEditors *metadataops.EditorService
	Users           *userops.Service
}

func (m Model) WithWriteServices(services WriteServices) Model {
	m.apex = services.Apex
	m.records = services.Records
	m.bundleOps = services.Bundles
	m.permissions = services.Permissions
	m.metadata = services.Metadata
	m.metaEditors = services.MetadataEditors
	m.users = services.Users
	return m
}
