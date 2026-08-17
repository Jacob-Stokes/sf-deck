package sf

// Write surfaces — a uniform way to describe how one metadata entity
// is persisted. Callers use this when they want to express "edit
// this key on this entity" without caring whether it lands via
// Tooling REST PATCH, a Metadata API deploy, or a Tooling composite
// request.

// ToolingEntity is one concrete record the Tooling API can PATCH
// via the generic UpdateToolingMetadata flow. Binds the sobject
// type name + the record Id together so callers don't have to
// repeat them at every use site.
type ToolingEntity struct {
	Target string
	Type   string
	ID     string
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
