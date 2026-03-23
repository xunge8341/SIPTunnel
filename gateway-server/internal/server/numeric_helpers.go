package server

func maxOrDefault(value, fallback int64) int64 {
	if value > 0 {
		return value
	}
	return fallback
}
