package teams

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"chklst-go/internal/database"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

var connected atomic.Bool

// Connected reports whether the logger currently has a live broker connection.
func Connected() bool { return connected.Load() }

// Start subscribes to teams/# on MQTT_URL (e.g. tcp://localhost:1883) and stores every
// message raw in teams_events. No-op when MQTT_URL is unset.
//
// Each (re)connect writes a _logger/start marker BEFORE subscribing, so the retained
// replays that follow are known to come after a gap. While connected, a _logger/alive
// heartbeat bounds how long an open call can have lasted if the logger dies mid-call.
func Start() {
	url := strings.TrimSpace(os.Getenv("MQTT_URL"))
	if url == "" {
		log.Println("📞 Teams MQTT logger disabled (MQTT_URL unset)")
		return
	}
	host, _ := os.Hostname()
	opts := mqtt.NewClientOptions().
		AddBroker(url).
		SetClientID(fmt.Sprintf("chklst-%s-%s", host, os.Getenv("PORT"))).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetMaxReconnectInterval(time.Minute).
		SetOnConnectHandler(func(c mqtt.Client) {
			connected.Store(true)
			save(TopicStart, "", false)
			if t := c.Subscribe("teams/#", 1, onMessage); t.Wait() && t.Error() != nil {
				log.Printf("📞 Teams MQTT subscribe failed: %v", t.Error())
				return
			}
			log.Printf("📞 Teams MQTT logger connected to %s", url)
		}).
		SetConnectionLostHandler(func(_ mqtt.Client, err error) {
			connected.Store(false)
			log.Printf("📞 Teams MQTT connection lost (auto-reconnecting): %v", err)
		})
	mqtt.NewClient(opts).Connect() // ConnectRetry: keeps retrying in the background

	// ponytail: 2-min heartbeat = a mid-call logger crash overstates the call by <2 min.
	go func() {
		for range time.Tick(2 * time.Minute) {
			if connected.Load() {
				save(TopicAlive, "", false)
			}
		}
	}()
}

func onMessage(_ mqtt.Client, m mqtt.Message) {
	p := m.Payload()
	if len(p) > 4096 {
		p = p[:4096]
	}
	save(m.Topic(), string(p), m.Retained())
}

func save(topic, payload string, retained bool) {
	ev := database.TeamsEvent{TS: time.Now().UTC(), Topic: topic, Payload: payload, Retained: retained}
	if err := database.DB.Create(&ev).Error; err != nil {
		log.Printf("📞 Teams event not saved (%s): %v", topic, err)
	}
}
