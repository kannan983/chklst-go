package handlers

import (
	"strconv"
	"strings"
	"time"

	"chklst-go/internal/database"
	"chklst-go/internal/teams"

	"github.com/gofiber/fiber/v3"
)

// GetTeamsReport returns call/presence analytics for an ISO week.
// ?week=current|last|YYYY-Www  ?miss_window=60 (seconds)
func GetTeamsReport(c fiber.Ctx) error {
	now := time.Now()
	from, err := teams.WeekStart(c.Query("week", "current"), now, time.Local)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	to := from.AddDate(0, 0, 7)
	missSecs, _ := strconv.Atoi(c.Query("miss_window"))
	if missSecs <= 0 {
		missSecs = 60
	}
	missWindow := time.Duration(missSecs) * time.Second

	// State at `from` is rebuilt from the last logger (re)connect at or before it: the
	// retained replays after that marker carry every topic's value.
	var start database.TeamsEvent
	database.DB.Where("topic = ? AND ts <= ?", teams.TopicStart, from.UTC()).Order("ts desc").Limit(1).Find(&start)
	var rows []database.TeamsEvent
	if err := database.DB.Where("ts >= ? AND ts < ?", start.TS.UTC(), to.UTC()).Order("ts, id").Find(&rows).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to load Teams events"})
	}

	events := make([]teams.Event, len(rows))
	for i, r := range rows {
		events[i] = teams.Event{TS: r.TS.In(time.Local), Topic: r.Topic, Payload: r.Payload, Retained: r.Retained}
	}
	return c.JSON(teams.Compute(events, from, now, missWindow))
}

// GetTeamsLive returns the latest value of every teams/* topic and logger state.
func GetTeamsLive(c fiber.Ctx) error {
	var rows []database.TeamsEvent
	database.DB.Where("id IN (SELECT MAX(id) FROM teams_events WHERE topic LIKE 'teams/%' GROUP BY topic)").Find(&rows)
	state := map[string]string{}
	var updated time.Time
	for _, r := range rows {
		state[strings.TrimPrefix(r.Topic, "teams/")] = teams.Value(r.Topic, r.Payload)
		if r.TS.After(updated) {
			updated = r.TS
		}
	}
	return c.JSON(fiber.Map{"logger_connected": teams.Connected(), "state": state, "updated_at": updated})
}
