package sf

// Write surfaces — a uniform way to describe how one metadata entity
// is persisted. Callers use this when they want to express "edit
// this key on this entity" without caring whether it lands via
// Tooling REST PATCH, a Metadata API deploy, or a Tooling composite
// request.
//
// Two concrete surfaces today:
//
//   ToolingEntitySurface — direct PATCH against /sobjects/<type>/<id>
//     with {FullName, Metadata} envelope. Fast (sub-second), atomic,
//     used by CustomField / ValidationRule / RecordType / ApexTrigger.
//
//   (future) MetadataDeploySurface — zip + deploy + poll. Slower
//     (2-5s), required for CustomObject edits and anything else
//     Tooling can't PATCH. The existing DeployCustomObjectPatch
//     path is a specialized version of this.
//
// A surface doesn't know *what* field is being changed — that's the
// caller's job. It knows how to fetch current Metadata, apply a
// patch, and commit. Keeps the UI's editModal save/preview closures
// free of Tooling-vs-Metadata plumbing.

// ToolingEntity is one concrete record the Tooling API can PATCH
// via the generic UpdateToolingMetadata flow. Binds the sobject
// type name + the record Id together so callers don't have to
// repeat them at every use site.
type ToolingEntity struct {
	// Target is the org alias or username — forwarded to every sf
	// helper that does the actual HTTP call.
	Target string
	// Type is the Tooling sobject name: "CustomField",
	// "ValidationRule", "RecordType", "ApexTrigger", ….
	Type string
	// ID is the 18-char Tooling record Id.
	ID string
}

// GetMetaString reads a single string key from the entity's Metadata
// via Tooling. Returns "" when the key is absent or not a string.
func (e ToolingEntity) GetMetaString(key string) (string, error) {
	meta, err := GetToolingMetadata(e.Target, e.Type, e.ID)
	if err != nil {
		return "", err
	}
	if v, ok := meta[key].(string); ok {
		return v, nil
	}
	return "", nil
}
