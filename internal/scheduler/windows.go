package scheduler

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/michaelquigley/otis/internal/config"
)

type Window struct {
	StartMinute int
	EndMinute   int
}

func ParseWindow(value string) (Window, error) {
	parts := strings.Split(value, "-")
	if len(parts) != 2 {
		return Window{}, fmt.Errorf("expected HH:MM-HH:MM")
	}
	start, err := parseEndpoint(parts[0], false)
	if err != nil {
		return Window{}, fmt.Errorf("start: %w", err)
	}
	end, err := parseEndpoint(parts[1], true)
	if err != nil {
		return Window{}, fmt.Errorf("end: %w", err)
	}
	return Window{StartMinute: start, EndMinute: end}, nil
}

func (w Window) Contains(now time.Time) bool {
	local := now.Local()
	minute := local.Hour()*60 + local.Minute()
	if w.StartMinute == w.EndMinute {
		return false
	}
	if w.EndMinute < w.StartMinute {
		return minute >= w.StartMinute || minute < w.EndMinute
	}
	return minute >= w.StartMinute && minute < w.EndMinute
}

func InWindow(value string, now time.Time) (bool, error) {
	window, err := ParseWindow(value)
	if err != nil {
		return false, err
	}
	return window.Contains(now), nil
}

func ReviewerWindowOpen(global *config.GlobalConfig, reviewerKind string, now time.Time) (bool, error) {
	if global == nil {
		return false, fmt.Errorf("global config is required")
	}
	reviewer := global.Reviewers[reviewerKind]
	if reviewer == nil {
		return false, fmt.Errorf("reviewer %q is not configured", reviewerKind)
	}
	window := global.Windows[reviewer.Window]
	if window == nil {
		return false, fmt.Errorf("reviewer %q references unknown window %q", reviewerKind, reviewer.Window)
	}
	return InWindow(window.Hours, now)
}

func parseEndpoint(value string, allowEndOfDay bool) (int, error) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 2 {
		return 0, fmt.Errorf("expected HH:MM")
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, err
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, err
	}
	if allowEndOfDay && hour == 24 && minute == 0 {
		return 24 * 60, nil
	}
	if hour < 0 || hour > 23 {
		return 0, fmt.Errorf("hour must be 00-23")
	}
	if minute < 0 || minute > 59 {
		return 0, fmt.Errorf("minute must be 00-59")
	}
	return hour*60 + minute, nil
}
