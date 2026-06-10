package mock

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/DanielTangnes/azlocal/internal/config"
)

// eventBufferSize is how many recent events are kept per topic for inspection
// via GET /<topic>/events and the web UI.
const eventBufferSize = 200

// EventGrid is an in-memory mock of an Event Grid topic endpoint. Events
// published to POST /<topic>/api/events are recorded and fanned out to the
// topic's configured webhook subscriptions, mirroring push delivery.
//
// Both Event Grid schema and CloudEvents schema payloads are accepted and
// forwarded verbatim, one event per delivery request, as the real service
// does by default. Authentication mirrors the real service loosely: a
// publish must carry an aeg-sas-key, aeg-sas-token, or Authorization header,
// but any value is accepted.
type EventGrid struct {
	topics map[string][]config.EventGridSubscription

	client *http.Client

	mu     sync.RWMutex
	recent map[string][]*ReceivedEvent

	wg sync.WaitGroup // outstanding deliveries (Flush waits on this)
}

// ReceivedEvent is a recorded publish plus its delivery outcomes.
type ReceivedEvent struct {
	Topic      string          `json:"topic"`
	ReceivedAt time.Time       `json:"receivedAt"`
	Event      json.RawMessage `json:"event"`
	Deliveries []*Delivery     `json:"deliveries"`
}

// Delivery records the outcome of pushing one event to one subscription.
type Delivery struct {
	Subscription string `json:"subscription"`
	Endpoint     string `json:"endpoint"`
	Status       int    `json:"status,omitempty"` // HTTP status, 0 if unreachable
	Error        string `json:"error,omitempty"`

	mu sync.Mutex
}

// NewEventGrid builds the mock from the eventgrid config section.
func NewEventGrid(cfg *config.EventGridService) *EventGrid {
	eg := &EventGrid{
		topics: map[string][]config.EventGridSubscription{},
		recent: map[string][]*ReceivedEvent{},
		client: &http.Client{Timeout: 10 * time.Second},
	}
	for _, t := range cfg.Topics {
		eg.topics[t.Name] = t.Subscriptions
	}
	return eg
}

// Topics returns the configured topic names and subscriptions (used by the UI).
func (eg *EventGrid) Topics() map[string][]config.EventGridSubscription { return eg.topics }

// Recent returns up to eventBufferSize most recent events for a topic,
// newest first.
func (eg *EventGrid) Recent(topic string) []*ReceivedEvent {
	eg.mu.RLock()
	defer eg.mu.RUnlock()
	events := eg.recent[topic]
	out := make([]*ReceivedEvent, len(events))
	for i, e := range events {
		out[len(events)-1-i] = e
	}
	return out
}

// Flush blocks until all in-flight webhook deliveries complete.
func (eg *EventGrid) Flush() { eg.wg.Wait() }

// Handler returns the HTTP handler for the mock.
func (eg *EventGrid) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", eg.route)
	return mux
}

func egError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{"code": status, "message": msg},
	})
}

func (eg *EventGrid) route(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")

	switch {
	case len(parts) == 1 && parts[0] == "topics" && r.Method == http.MethodGet:
		eg.handleTopics(w)
	case len(parts) == 3 && parts[1] == "api" && parts[2] == "events" && r.Method == http.MethodPost:
		eg.handlePublish(w, r, parts[0])
	case len(parts) == 2 && parts[1] == "events" && r.Method == http.MethodGet:
		eg.handleRecent(w, parts[0])
	default:
		egError(w, http.StatusNotFound,
			"unknown route; publish to POST /<topic>/api/events, inspect via GET /<topic>/events or GET /topics")
	}
}

func (eg *EventGrid) handleTopics(w http.ResponseWriter) {
	type topicInfo struct {
		Name          string                         `json:"name"`
		Subscriptions []config.EventGridSubscription `json:"subscriptions"`
	}
	out := []topicInfo{}
	for name, subs := range eg.topics {
		if subs == nil {
			subs = []config.EventGridSubscription{}
		}
		out = append(out, topicInfo{Name: name, Subscriptions: subs})
	}
	writeJSON(w, http.StatusOK, out)
}

func (eg *EventGrid) handleRecent(w http.ResponseWriter, topic string) {
	if _, ok := eg.topics[topic]; !ok {
		egError(w, http.StatusNotFound, fmt.Sprintf("topic %q is not configured", topic))
		return
	}
	writeJSON(w, http.StatusOK, eg.Recent(topic))
}

func (eg *EventGrid) handlePublish(w http.ResponseWriter, r *http.Request, topic string) {
	subs, ok := eg.topics[topic]
	if !ok {
		egError(w, http.StatusNotFound, fmt.Sprintf("topic %q is not configured", topic))
		return
	}
	if r.Header.Get("aeg-sas-key") == "" && r.Header.Get("aeg-sas-token") == "" && r.Header.Get("Authorization") == "" {
		egError(w, http.StatusUnauthorized, "missing aeg-sas-key, aeg-sas-token, or Authorization header (any value is accepted by this mock)")
		return
	}

	var body bytes.Buffer
	if _, err := body.ReadFrom(r.Body); err != nil {
		egError(w, http.StatusBadRequest, "read body: "+err.Error())
		return
	}
	events, err := splitEvents(body.Bytes())
	if err != nil {
		egError(w, http.StatusBadRequest, err.Error())
		return
	}

	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	for _, ev := range events {
		received := &ReceivedEvent{
			Topic:      topic,
			ReceivedAt: time.Now().UTC(),
			Event:      ev,
			Deliveries: []*Delivery{},
		}
		for _, sub := range subs {
			d := &Delivery{Subscription: sub.Name, Endpoint: sub.Endpoint}
			received.Deliveries = append(received.Deliveries, d)
			eg.wg.Add(1)
			go eg.deliver(d, ev, contentType)
		}
		eg.record(topic, received)
	}

	// The real service responds 200 with an empty body.
	w.WriteHeader(http.StatusOK)
}

// splitEvents accepts a JSON array of events or a single JSON object.
func splitEvents(data []byte) ([]json.RawMessage, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty body; expected a JSON event or array of events")
	}
	if trimmed[0] == '[' {
		var arr []json.RawMessage
		if err := json.Unmarshal(trimmed, &arr); err != nil {
			return nil, fmt.Errorf("invalid event array: %w", err)
		}
		return arr, nil
	}
	var single json.RawMessage
	if err := json.Unmarshal(trimmed, &single); err != nil {
		return nil, fmt.Errorf("invalid event JSON: %w", err)
	}
	return []json.RawMessage{single}, nil
}

func (eg *EventGrid) record(topic string, ev *ReceivedEvent) {
	eg.mu.Lock()
	defer eg.mu.Unlock()
	buf := append(eg.recent[topic], ev)
	if len(buf) > eventBufferSize {
		buf = buf[len(buf)-eventBufferSize:]
	}
	eg.recent[topic] = buf
}

// deliver pushes one event (as a single-element array, matching Event Grid
// webhook delivery) to a subscription endpoint.
func (eg *EventGrid) deliver(d *Delivery, ev json.RawMessage, contentType string) {
	defer eg.wg.Done()
	payload := append(append([]byte{'['}, ev...), ']')
	req, err := http.NewRequest(http.MethodPost, d.Endpoint, bytes.NewReader(payload))
	if err != nil {
		d.setResult(0, err.Error())
		return
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("aeg-event-type", "Notification")
	resp, err := eg.client.Do(req)
	if err != nil {
		d.setResult(0, err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		d.setResult(resp.StatusCode, fmt.Sprintf("endpoint returned %s", resp.Status))
		return
	}
	d.setResult(resp.StatusCode, "")
}

func (d *Delivery) setResult(status int, errMsg string) {
	d.mu.Lock()
	d.Status = status
	d.Error = errMsg
	d.mu.Unlock()
}

// MarshalJSON locks around the mutable fields so recent-event listings are
// race-free while deliveries are still in flight.
func (d *Delivery) MarshalJSON() ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	type plain struct {
		Subscription string `json:"subscription"`
		Endpoint     string `json:"endpoint"`
		Status       int    `json:"status,omitempty"`
		Error        string `json:"error,omitempty"`
	}
	return json.Marshal(plain{d.Subscription, d.Endpoint, d.Status, d.Error})
}
