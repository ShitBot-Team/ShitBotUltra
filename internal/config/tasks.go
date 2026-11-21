package config

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	_ "time/tzdata" // Daily schedules must also work on hosts without zoneinfo.
	"unicode"
	"unicode/utf8"
)

// Task is a single scheduled effect. A task never fans out or invokes a shell.
type Task struct {
	ID       string       `yaml:"id" json:"id"`
	Name     string       `yaml:"name" json:"name"`
	Enabled  bool         `yaml:"enabled" json:"enabled"`
	Schedule TaskSchedule `yaml:"schedule" json:"schedule"`
	Action   TaskAction   `yaml:"action" json:"action"`
}

type TaskSchedule struct {
	Kind         string `yaml:"kind" json:"kind"`
	EverySeconds int    `yaml:"every_seconds,omitempty" json:"every_seconds,omitempty"`
	At           string `yaml:"at,omitempty" json:"at,omitempty"`
	Timezone     string `yaml:"timezone,omitempty" json:"timezone,omitempty"`
}

type TaskAction struct {
	Kind    string `yaml:"kind" json:"kind"`
	RouteID string `yaml:"route_id,omitempty" json:"route_id,omitempty"`
	NodeID  string `yaml:"node_id,omitempty" json:"node_id,omitempty"`
	Text    string `yaml:"text" json:"text"`
}

func (c Config) validateTasks() error {
	if len(c.Tasks) > 128 {
		return errors.New("at most 128 scheduled tasks are allowed")
	}
	ids := map[string]bool{}
	for _, task := range c.Tasks {
		if !localID.MatchString(task.ID) || ids[task.ID] {
			return errors.New("invalid or duplicate task ID")
		}
		ids[task.ID] = true
		if strings.TrimSpace(task.Name) == "" || len(task.Name) > 128 || !utf8.ValidString(task.Name) || strings.ContainsFunc(task.Name, unicode.IsControl) {
			return fmt.Errorf("task %s requires a name of at most 128 bytes", task.ID)
		}
		if err := task.Schedule.Validate(); err != nil {
			return fmt.Errorf("task %s: %w", task.ID, err)
		}
		a := task.Action
		if strings.TrimSpace(a.Text) == "" || len(a.Text) > 2000 || !utf8.ValidString(a.Text) || strings.ContainsFunc(a.Text, unicode.IsControl) {
			return fmt.Errorf("task %s text must be 1-2000 UTF-8 bytes without control characters", task.ID)
		}
		switch a.Kind {
		case "bot.message":
			if a.NodeID != "" {
				return fmt.Errorf("task %s bot action cannot specify node_id", task.ID)
			}
			found := false
			for _, route := range c.Routes {
				if route.ID != a.RouteID {
					continue
				}
				found = true
				for _, conn := range c.Connections {
					if conn.ID == route.ConnectionID && task.Enabled && !conn.Enabled {
						return fmt.Errorf("task %s requires an enabled bot connection", task.ID)
					}
				}
			}
			if !found {
				return fmt.Errorf("task %s references an unknown route", task.ID)
			}
		case "chat.send", "command.execute":
			node, found := c.Node(a.NodeID)
			if a.RouteID != "" || !found || !slices.Contains(node.Actions, a.Kind) {
				return fmt.Errorf("task %s requires a node permitting %s and no route_id", task.ID, a.Kind)
			}
			if a.Kind == "command.execute" && (strings.HasPrefix(a.Text, "/") || strings.TrimSpace(a.Text) != a.Text) {
				return fmt.Errorf("task %s command must not start with / or have outer whitespace", task.ID)
			}
		default:
			return fmt.Errorf("task %s has an unsupported action", task.ID)
		}
	}
	return nil
}

func (s TaskSchedule) Validate() error {
	switch s.Kind {
	case "interval":
		if s.EverySeconds < 60 || s.EverySeconds > 604800 || s.At != "" || s.Timezone != "" {
			return errors.New("interval requires every_seconds from 60 to 604800 and no at/timezone")
		}
	case "daily":
		if s.EverySeconds != 0 || len(s.At) != 5 || s.Timezone == "" || s.Timezone == "Local" || len(s.Timezone) > 128 {
			return errors.New("daily requires HH:MM and an explicit IANA timezone, without every_seconds")
		}
		if _, err := time.Parse("15:04", s.At); err != nil {
			return errors.New("daily at must be HH:MM")
		}
		if _, err := time.LoadLocation(s.Timezone); err != nil {
			return errors.New("unknown task timezone")
		}
	default:
		return errors.New("schedule kind must be interval or daily")
	}
	return nil
}
