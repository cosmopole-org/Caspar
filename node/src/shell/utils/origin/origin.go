package origin

import "strings"

func FindOrigin(id string) string {
	if id == "" {
		return ""
	} else {
		parts := strings.Split(id, "@")
		if len(parts) < 2 {
			return ""
		} else {
			return parts[len(parts)-1]
		}
	}
}
