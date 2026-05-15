package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseDuration accepts Otis cadence/window aliases plus Go duration strings.
func ParseDuration(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	switch value {
	case "":
		return 0, fmt.Errorf("duration is required")
	case "hourly":
		return time.Hour, nil
	case "daily":
		return 24 * time.Hour, nil
	case "weekly":
		return 7 * 24 * time.Hour, nil
	}
	if strings.HasSuffix(value, "w") {
		n, err := strconv.Atoi(strings.TrimSuffix(value, "w"))
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("invalid week duration %q", value)
		}
		return time.Duration(n) * 7 * 24 * time.Hour, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, err
	}
	if duration <= 0 {
		return 0, fmt.Errorf("duration must be greater than zero")
	}
	return duration, nil
}
