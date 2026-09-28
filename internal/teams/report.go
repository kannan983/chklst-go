package teams

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Marker topics written by the logger itself (not by Teams).
const (
	TopicStart = "_logger/start" // logger (re)connected; retained replays follow
	TopicAlive = "_logger/alive" // heartbeat while connected
)

// resumeGap: a logger restart shorter than this, after which Teams replays the same
// state, continues the session instead of splitting it (e.g. redeploying mid-call).
const resumeGap = 10 * time.Minute

// Event is one stored MQTT message, in receive order.
type Event struct {
	TS       time.Time
	Topic    string
	Payload  string
	Retained bool
}

var tracked = map[string]bool{
	"teams/connected": true, "teams/in-call": true, "teams/incoming-call": true,
	"teams/camera": true, "teams/microphone": true, "teams/screen-sharing": true, "teams/status": true,
}

type interval struct {
	start, end time.Time
	value      string
	cut        bool // closed by disconnect / restart, not a clean transition
	replayed   bool // opened by a retained replay (stale, not a live change)
	ongoing    bool // still open at the report end
}

// Value normalizes a payload: the "status" field for teams/status JSON, else the
// trimmed lowercase payload.
func Value(topic, payload string) string {
	p := strings.TrimSpace(payload)
	if topic == "teams/status" {
		var s struct {
			Status string `json:"status"`
		}
		if json.Unmarshal([]byte(p), &s) == nil && s.Status != "" {
			p = s.Status
		}
	}
	return strings.ToLower(p)
}

// buildIntervals turns events into per-topic state intervals, closing anything
// still open at end. Repeated values (incl. retained replays of the current value)
// are no-ops.
func buildIntervals(events []Event, end time.Time) map[string][]interval {
	out := map[string][]interval{}
	open := map[string]*interval{}
	pending := map[string]*interval{} // closed by a logger restart; may resume
	var lastSeen time.Time

	closeAll := func(t time.Time, pendingOnly bool) {
		for tp, iv := range open {
			iv.end, iv.cut = t, true
			if iv.end.Before(iv.start) {
				iv.end = iv.start
			}
			if pendingOnly {
				pending[tp] = iv
			} else {
				out[tp] = append(out[tp], *iv)
			}
			delete(open, tp)
		}
	}

	for _, e := range events {
		switch {
		case e.Topic == TopicStart:
			closeAll(lastSeen, true)
		case tracked[e.Topic]:
			v := Value(e.Topic, e.Payload)
			if p := pending[e.Topic]; p != nil {
				delete(pending, e.Topic)
				if p.value == v && e.TS.Sub(p.end) <= resumeGap {
					p.cut = false
					open[e.Topic] = p
					break
				}
				out[e.Topic] = append(out[e.Topic], *p)
			}
			if cur := open[e.Topic]; cur != nil {
				if cur.value == v {
					break
				}
				cur.end, cur.cut = e.TS, e.Retained
				out[e.Topic] = append(out[e.Topic], *cur)
				delete(open, e.Topic)
			}
			if e.Topic == "teams/connected" && v == "false" {
				closeAll(e.TS, false) // app quit/crashed: every other state ends here
			}
			open[e.Topic] = &interval{start: e.TS, value: v, replayed: e.Retained}
		}
		lastSeen = e.TS
	}
	for tp, p := range pending {
		out[tp] = append(out[tp], *p)
	}
	for tp, iv := range open {
		iv.end, iv.ongoing = end, true
		if iv.end.Before(iv.start) {
			iv.end = iv.start
		}
		out[tp] = append(out[tp], *iv)
	}
	for tp := range out {
		sort.Slice(out[tp], func(i, j int) bool { return out[tp][i].start.Before(out[tp][j].start) })
	}
	return out
}

func overlap(iv interval, a, b time.Time) int64 {
	s, e := iv.start, iv.end
	if s.Before(a) {
		s = a
	}
	if e.After(b) {
		e = b
	}
	if !e.After(s) {
		return 0
	}
	return int64(e.Sub(s).Seconds())
}

// Stats are durations in seconds over a window.
type Stats struct {
	CallSeconds   int64            `json:"call_seconds"`
	Calls         int              `json:"calls"` // calls starting in the window
	Missed        int              `json:"missed"`
	CameraSeconds int64            `json:"camera_seconds"`
	ScreenSeconds int64            `json:"screen_seconds"`
	OpenSeconds   int64            `json:"open_seconds"` // Teams connected
	Mic           map[string]int64 `json:"mic"`          // speaking / silent / muted
	Status        map[string]int64 `json:"status"`       // raw status strings
}

type Day struct {
	Date    string `json:"date"`    // 2026-09-28
	Weekday string `json:"weekday"` // Mon
	Stats
}

type Call struct {
	Start              time.Time `json:"start"`
	End                time.Time `json:"end"`
	Seconds            int64     `json:"seconds"`
	SpeakingSeconds    int64     `json:"speaking_seconds"`
	MutedSeconds       int64     `json:"muted_seconds"`
	CameraSeconds      int64     `json:"camera_seconds"`
	ScreenSeconds      int64     `json:"screen_seconds"`
	ClosedByDisconnect bool      `json:"closed_by_disconnect"`
	Ongoing            bool      `json:"ongoing"`
}

type Report struct {
	Week        string      `json:"week"` // 2026-W40
	Prev        string      `json:"prev"`
	Next        string      `json:"next"`
	From        string      `json:"from"`
	To          string      `json:"to"` // inclusive last day
	Totals      Stats       `json:"totals"`
	AvgCall     int64       `json:"avg_call_seconds"`
	LongestCall *Call       `json:"longest_call"`
	BusiestDay  *Day        `json:"busiest_day"`
	Days        []Day       `json:"days"`
	Calls       []Call      `json:"calls"`
	MissedAt    []time.Time `json:"missed_at"`
	HasData     bool        `json:"has_data"`
}

func weekName(t time.Time) string {
	y, w := t.ISOWeek()
	return fmt.Sprintf("%d-W%02d", y, w)
}

// WeekStart resolves "current" | "last" | "YYYY-Www" to that ISO week's Monday 00:00 in loc.
func WeekStart(q string, now time.Time, loc *time.Location) (time.Time, error) {
	monday := func(t time.Time) time.Time {
		y, m, d := t.In(loc).Date()
		t0 := time.Date(y, m, d, 0, 0, 0, 0, loc)
		return t0.AddDate(0, 0, -((int(t0.Weekday()) + 6) % 7))
	}
	switch q {
	case "", "current":
		return monday(now), nil
	case "last":
		return monday(now).AddDate(0, 0, -7), nil
	}
	var y, w int
	if _, err := fmt.Sscanf(q, "%d-W%d", &y, &w); err != nil || w < 1 || w > 53 {
		return time.Time{}, fmt.Errorf("invalid week %q (use current, last or YYYY-Www)", q)
	}
	start := monday(time.Date(y, 1, 4, 0, 0, 0, 0, loc)).AddDate(0, 0, (w-1)*7)
	if weekName(start) != fmt.Sprintf("%d-W%02d", y, w) {
		return time.Time{}, fmt.Errorf("%d has no week %d", y, w)
	}
	return start, nil
}

// Compute builds the report for [from, from+7d). now clips the current week;
// missWindow is how soon an in-call must follow a ring for it not to be missed.
func Compute(events []Event, from, now time.Time, missWindow time.Duration) Report {
	to := from.AddDate(0, 0, 7)
	end := to
	if now.Before(end) {
		end = now
	}
	ivs := buildIntervals(events, end)
	r := Report{
		Week: weekName(from), Prev: weekName(from.AddDate(0, 0, -7)), Next: weekName(to),
		From: from.Format("2006-01-02"), To: to.AddDate(0, 0, -1).Format("2006-01-02"),
		Totals:  Stats{Mic: map[string]int64{}, Status: map[string]int64{}},
		Calls:   []Call{},
		HasData: len(events) > 0,
	}

	calls := []interval{}
	for _, c := range ivs["teams/in-call"] {
		if c.value == "true" && c.end.After(from) && c.start.Before(to) {
			calls = append(calls, c)
		}
	}
	missed := []time.Time{}
	for _, ring := range ivs["teams/incoming-call"] {
		if ring.value != "true" || ring.replayed || ring.start.Before(from) || !ring.start.Before(to) {
			continue
		}
		answered := false
		for _, c := range ivs["teams/in-call"] {
			if c.value == "true" && !c.start.Before(ring.start) && c.start.Sub(ring.start) <= missWindow {
				answered = true
				break
			}
		}
		if !answered {
			missed = append(missed, ring.start)
		}
	}
	r.MissedAt = missed

	sumTrue := func(topic string, a, b time.Time) (n int64) {
		for _, iv := range ivs[topic] {
			if iv.value == "true" {
				n += overlap(iv, a, b)
			}
		}
		return
	}

	for d := from; d.Before(to); d = d.AddDate(0, 0, 1) {
		next := d.AddDate(0, 0, 1)
		day := Day{Date: d.Format("2006-01-02"), Weekday: d.Format("Mon"),
			Stats: Stats{Mic: map[string]int64{}, Status: map[string]int64{}}}
		for _, c := range calls {
			day.CallSeconds += overlap(c, d, next)
			if !c.start.Before(d) && c.start.Before(next) {
				day.Calls++
			}
		}
		for _, t := range missed {
			if !t.Before(d) && t.Before(next) {
				day.Missed++
			}
		}
		day.CameraSeconds = sumTrue("teams/camera", d, next)
		day.ScreenSeconds = sumTrue("teams/screen-sharing", d, next)
		day.OpenSeconds = sumTrue("teams/connected", d, next)
		for _, iv := range ivs["teams/microphone"] {
			if iv.value == "speaking" || iv.value == "silent" || iv.value == "muted" {
				if s := overlap(iv, d, next); s > 0 {
					day.Mic[iv.value] += s
				}
			}
		}
		for _, iv := range ivs["teams/status"] {
			if s := overlap(iv, d, next); s > 0 && iv.value != "" {
				day.Status[iv.value] += s
			}
		}

		t := &r.Totals
		t.CallSeconds += day.CallSeconds
		t.Calls += day.Calls
		t.Missed += day.Missed
		t.CameraSeconds += day.CameraSeconds
		t.ScreenSeconds += day.ScreenSeconds
		t.OpenSeconds += day.OpenSeconds
		for k, v := range day.Mic {
			t.Mic[k] += v
		}
		for k, v := range day.Status {
			t.Status[k] += v
		}
		r.Days = append(r.Days, day)
	}

	for _, c := range calls {
		span := func(topic, value string) (n int64) {
			for _, iv := range ivs[topic] {
				if iv.value == value {
					n += overlap(iv, c.start, c.end)
				}
			}
			return
		}
		r.Calls = append(r.Calls, Call{
			Start: c.start, End: c.end, Seconds: int64(c.end.Sub(c.start).Seconds()),
			SpeakingSeconds: span("teams/microphone", "speaking"), MutedSeconds: span("teams/microphone", "muted"),
			CameraSeconds: span("teams/camera", "true"), ScreenSeconds: span("teams/screen-sharing", "true"),
			ClosedByDisconnect: c.cut, Ongoing: c.ongoing && now.Before(to),
		})
	}
	for i := range r.Calls {
		if r.LongestCall == nil || r.Calls[i].Seconds > r.LongestCall.Seconds {
			r.LongestCall = &r.Calls[i]
		}
	}
	if len(r.Calls) > 0 {
		var sum int64
		for _, c := range r.Calls {
			sum += c.Seconds
		}
		r.AvgCall = sum / int64(len(r.Calls))
	}
	for i := range r.Days {
		if r.Days[i].CallSeconds > 0 && (r.BusiestDay == nil || r.Days[i].CallSeconds > r.BusiestDay.CallSeconds) {
			r.BusiestDay = &r.Days[i]
		}
	}
	return r
}
