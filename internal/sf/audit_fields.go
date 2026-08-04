package sf

func relationName(record map[string]any, field string) string {
	if rel, ok := record[field].(map[string]any); ok {
		return asString(rel["Name"])
	}
	return ""
}
