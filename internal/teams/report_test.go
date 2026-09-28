package teams

import (
	"testing"
	"time"
)

var dubai = time.FixedZone("GST", 4*3600)

// Monday of 2026-W40 in Dubai.
var monday = time.Date(2026, 9, 28, 0, 0, 0, 0, dubai)

func at(day int, hm string) time.Time {
	t, _ := time.ParseInLocation("15:04", hm, dubai)
	return monday.AddDate(0, 0, day).Add(time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute)
}

func ev(t time.Time, topic, payload string) Event { return Event{TS: t, Topic: "teams/" + topic, Payload: payload} }

func run(events []Event) Report {
	return Compute(events, monday, monday.AddDate(0, 0, 7), 60*time.Second)
}

func TestNormalCall(t *testing.T) {
	r := run([]Event{
		ev(at(0, "09:00"), "connected", "true"),
		ev(at(0, "10:00"), "in-call", "true"),
		ev(at(0, "10:00"), "microphone", "speaking"),
		ev(at(0, "10:10"), "microphone", "muted"),
		ev(at(0, "10:05"), "camera", "true"),
		ev(at(0, "10:20"), "camera", "false"),
		ev(at(0, "10:30"), "in-call", "true"), // repeat: ignored
		ev(at(0, "10:30"), "microphone", "off"),
		ev(at(0, "10:30"), "in-call", "false"),
		ev(at(0, "17:00"), "connected", "false"),
	})
	if len(r.Calls) != 1 || r.Calls[0].Seconds != 1800 || r.Calls[0].ClosedByDisconnect {
		t.Fatalf("calls = %+v", r.Calls)
	}
	c := r.Calls[0]
	if c.SpeakingSeconds != 600 || c.MutedSeconds != 1200 || c.CameraSeconds != 900 {
		t.Fatalf("call breakdown = %+v", c)
	}
	if r.Totals.OpenSeconds != 8*3600 || r.Days[0].Calls != 1 || r.BusiestDay.Date != "2026-09-28" {
		t.Fatalf("totals = %+v busiest=%v", r.Totals, r.BusiestDay)
	}
}

func TestCrashMidCall(t *testing.T) {
	r := run([]Event{
		ev(at(1, "10:00"), "connected", "true"),
		ev(at(1, "10:00"), "in-call", "true"),
		ev(at(1, "10:15"), "connected", "false"), // Last Will
	})
	if len(r.Calls) != 1 || r.Calls[0].Seconds != 900 || !r.Calls[0].ClosedByDisconnect {
		t.Fatalf("calls = %+v", r.Calls)
	}
}

func TestLoggerRestart(t *testing.T) {
	restart := func(at time.Time, v string) []Event {
		return []Event{{TS: at, Topic: TopicStart}, {TS: at, Topic: "teams/in-call", Payload: v, Retained: true}}
	}
	// Short redeploy mid-call, same state replayed: one continuous call.
	evs := append([]Event{ev(at(0, "10:00"), "in-call", "true"), {TS: at(0, "10:02"), Topic: TopicAlive}},
		restart(at(0, "10:03"), "true")...)
	evs = append(evs, ev(at(0, "10:30"), "in-call", "false"))
	if r := run(evs); len(r.Calls) != 1 || r.Calls[0].Seconds != 1800 || r.Calls[0].ClosedByDisconnect {
		t.Fatalf("resume: calls = %+v", r.Calls)
	}
	// Laptop off overnight with a stale retained true: closed at last heartbeat, flagged.
	evs = append([]Event{ev(at(0, "10:00"), "in-call", "true"), {TS: at(0, "10:20"), Topic: TopicAlive}},
		restart(at(1, "08:00"), "true")...)
	evs = append(evs, ev(at(1, "08:01"), "in-call", "false"))
	r := run(evs)
	if len(r.Calls) != 2 || r.Calls[0].Seconds != 1200 || !r.Calls[0].ClosedByDisconnect || r.Calls[1].Seconds != 60 {
		t.Fatalf("stale: calls = %+v", r.Calls)
	}
}

func TestMissedCall(t *testing.T) {
	r := run([]Event{
		ev(at(2, "11:00"), "incoming-call", "true"),
		ev(at(2, "11:00"), "incoming-call", "false"), // rang, never answered
		ev(at(2, "12:00"), "incoming-call", "true"),
		ev(at(2, "12:00"), "in-call", "true"), // answered within the window
		ev(at(2, "12:05"), "in-call", "false"),
		{TS: at(3, "09:00"), Topic: "teams/incoming-call", Payload: "true", Retained: true}, // stale replay
	})
	if r.Totals.Missed != 1 || r.Days[2].Missed != 1 || r.Totals.Calls != 1 {
		t.Fatalf("missed=%d calls=%d", r.Totals.Missed, r.Totals.Calls)
	}
}

func TestBackToBackCalls(t *testing.T) {
	r := run([]Event{
		ev(at(3, "14:00"), "in-call", "true"),
		ev(at(3, "14:30"), "in-call", "false"),
		ev(at(3, "14:30"), "in-call", "true"),
		ev(at(3, "15:30"), "in-call", "false"),
	})
	if len(r.Calls) != 2 || r.Totals.CallSeconds != 5400 || r.AvgCall != 2700 || r.LongestCall.Seconds != 3600 {
		t.Fatalf("calls = %+v avg=%d", r.Calls, r.AvgCall)
	}
}

func TestCallSpansMidnight(t *testing.T) {
	r := run([]Event{
		ev(at(4, "23:30"), "in-call", "true"),
		ev(at(5, "00:45"), "in-call", "false"),
	})
	if r.Days[4].CallSeconds != 1800 || r.Days[5].CallSeconds != 2700 || r.Days[4].Calls != 1 || r.Days[5].Calls != 0 {
		t.Fatalf("days = %+v / %+v", r.Days[4].Stats, r.Days[5].Stats)
	}
	if len(r.Calls) != 1 || r.Calls[0].Seconds != 4500 {
		t.Fatalf("calls = %+v", r.Calls)
	}
}

func TestOngoingCallClippedToNow(t *testing.T) {
	r := Compute([]Event{ev(at(0, "10:00"), "in-call", "true")}, monday, at(0, "10:20"), time.Minute)
	if len(r.Calls) != 1 || !r.Calls[0].Ongoing || r.Calls[0].Seconds != 1200 {
		t.Fatalf("calls = %+v", r.Calls)
	}
}

func TestWeekStart(t *testing.T) {
	for q, want := range map[string]time.Time{
		"current":  monday,
		"last":     monday.AddDate(0, 0, -7),
		"2026-W40": monday,
		"2026-W01": time.Date(2025, 12, 29, 0, 0, 0, 0, dubai),
	} {
		got, err := WeekStart(q, at(3, "12:00"), dubai)
		if err != nil || !got.Equal(want) {
			t.Errorf("%s: got %v %v, want %v", q, got, err, want)
		}
	}
	if _, err := WeekStart("2026-W54", monday, dubai); err == nil {
		t.Error("expected error for W54")
	}
}
