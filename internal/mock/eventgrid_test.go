package mock

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/DanielTangnes/azlocal/internal/config"
)

func newTestEG(t *testing.T, endpoints ...string) (*EventGrid, *httptest.Server) {
	t.Helper()
	subs := make([]config.EventGridSubscription, len(endpoints))
	for i, e := range endpoints {
		subs[i] = config.EventGridSubscription{Name: "sub" + string(rune('a'+i)), Endpoint: e}
	}
	eg := NewEventGrid(&config.EventGridService{Topics: []config.EventGridTopic{
		{Name: "events", Subscriptions: subs},
	}})
	srv := httptest.NewServer(eg.Handler())
	t.Cleanup(srv.Close)
	return eg, srv
}

func publish(t *testing.T, srv *httptest.Server, topic, body string, headers map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/"+topic+"/api/events", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp
}

const sampleEvent = `{"id":"1","eventType":"user.created","subject":"users/1","data":{"name":"ada"},"eventTime":"2026-06-10T00:00:00Z","dataVersion":"1.0"}`

func TestEventGrid_PublishAndFanOut(t *testing.T) {
	var (
		mu       sync.Mutex
		received []string
	)
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var events []map[string]any
		if err := json.NewDecoder(r.Body).Decode(&events); err != nil || len(events) != 1 {
			t.Errorf("webhook expected single-event array, err=%v n=%d", err, len(events))
		}
		if got := r.Header.Get("aeg-event-type"); got != "Notification" {
			t.Errorf("aeg-event-type = %q", got)
		}
		mu.Lock()
		received = append(received, events[0]["id"].(string))
		mu.Unlock()
	}))
	defer hook.Close()

	eg, srv := newTestEG(t, hook.URL)

	resp := publish(t, srv, "events", "["+sampleEvent+"]", map[string]string{"aeg-sas-key": "x"})
	if resp.StatusCode != 200 {
		t.Fatalf("publish status = %d", resp.StatusCode)
	}
	eg.Flush()

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 || received[0] != "1" {
		t.Fatalf("webhook received %v", received)
	}

	recent := eg.Recent("events")
	if len(recent) != 1 {
		t.Fatalf("recent = %d events", len(recent))
	}
	if d := recent[0].Deliveries[0]; d.Status != 200 || d.Error != "" {
		t.Errorf("delivery = %+v", d)
	}
}

func TestEventGrid_AuthRequired(t *testing.T) {
	_, srv := newTestEG(t)
	if resp := publish(t, srv, "events", "["+sampleEvent+"]", nil); resp.StatusCode != 401 {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestEventGrid_UnknownTopic(t *testing.T) {
	_, srv := newTestEG(t)
	if resp := publish(t, srv, "nope", "["+sampleEvent+"]", map[string]string{"aeg-sas-key": "x"}); resp.StatusCode != 404 {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestEventGrid_SingleObjectAndBadJSON(t *testing.T) {
	eg, srv := newTestEG(t)
	if resp := publish(t, srv, "events", sampleEvent, map[string]string{"aeg-sas-key": "x"}); resp.StatusCode != 200 {
		t.Fatalf("single object publish status = %d", resp.StatusCode)
	}
	eg.Flush()
	if len(eg.Recent("events")) != 1 {
		t.Fatal("single object not recorded")
	}
	if resp := publish(t, srv, "events", "{not json", map[string]string{"aeg-sas-key": "x"}); resp.StatusCode != 400 {
		t.Fatalf("bad json status = %d, want 400", resp.StatusCode)
	}
}

func TestEventGrid_FailedDeliveryRecorded(t *testing.T) {
	eg, srv := newTestEG(t, "http://127.0.0.1:1/unreachable")
	publish(t, srv, "events", "["+sampleEvent+"]", map[string]string{"aeg-sas-key": "x"})
	eg.Flush()
	d := eg.Recent("events")[0].Deliveries[0]
	if d.Error == "" || d.Status != 0 {
		t.Errorf("expected recorded failure, got %+v", d)
	}
}

func TestEventGrid_TopicsEndpoint(t *testing.T) {
	_, srv := newTestEG(t)
	resp, err := http.Get(srv.URL + "/topics")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var topics []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&topics); err != nil {
		t.Fatal(err)
	}
	if len(topics) != 1 || topics[0]["name"] != "events" {
		t.Fatalf("topics = %v", topics)
	}
}
