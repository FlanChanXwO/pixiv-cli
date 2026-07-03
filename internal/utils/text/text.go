package text

func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func DefaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
