package httpapi

import "strings"

const routeOther = "other"

func routePatternFor(path string) string {
	switch {
	case path == "/healthz", path == "/readyz", path == "/metrics":
		return path
	case path == "/api/v1/bookings", path == "/api/v1/bookings/series", path == "/api/v1/rooms":
		return path
	case strings.HasPrefix(path, "/api/v1/bookings/"):
		return "/api/v1/bookings/{id}"
	case strings.HasPrefix(path, "/api/v1/rooms/"):
		return "/api/v1/rooms/{roomID}"
	default:
		return routeOther
	}
}
