// Package ui serves the unified azlocal web dashboard: browse blobs, peek
// queues, scan tables, query Cosmos, peek Service Bus, and inspect the
// Key Vault / Event Grid mocks — all against the running local suite.
package ui

import (
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"
	"github.com/Azure/azure-sdk-for-go/sdk/data/aztables"
	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azqueue"

	"github.com/DanielTangnes/azlocal/internal/config"
	"github.com/DanielTangnes/azlocal/internal/provision"
)

//go:embed static
var staticFS embed.FS

// requestTimeout bounds every emulator call made on behalf of the dashboard.
const requestTimeout = 15 * time.Second

// maxItems caps list/query/peek results per request.
const maxItems = 100

// Server is the dashboard HTTP handler.
type Server struct {
	cfg *config.Config
	mux *http.ServeMux
}

// New builds the dashboard server for a config.
func New(cfg *config.Config) *Server {
	s := &Server{cfg: cfg, mux: http.NewServeMux()}

	static, _ := fs.Sub(staticFS, "static")
	s.mux.Handle("/", http.FileServer(http.FS(static)))

	s.mux.HandleFunc("GET /api/overview", s.handleOverview)
	s.mux.HandleFunc("GET /api/blob/containers", s.handleBlobContainers)
	s.mux.HandleFunc("GET /api/blob/list", s.handleBlobList)
	s.mux.HandleFunc("GET /api/queue/list", s.handleQueues)
	s.mux.HandleFunc("GET /api/queue/peek", s.handleQueuePeek)
	s.mux.HandleFunc("GET /api/table/list", s.handleTables)
	s.mux.HandleFunc("GET /api/table/rows", s.handleTableRows)
	s.mux.HandleFunc("GET /api/cosmos/meta", s.handleCosmosMeta)
	s.mux.HandleFunc("POST /api/cosmos/query", s.handleCosmosQuery)
	s.mux.HandleFunc("GET /api/servicebus/meta", s.handleServiceBusMeta)
	s.mux.HandleFunc("GET /api/servicebus/peek", s.handleServiceBusPeek)
	s.mux.HandleFunc("GET /api/keyvault/secrets", s.handleKeyVaultSecrets)
	s.mux.HandleFunc("GET /api/keyvault/secret", s.handleKeyVaultSecret)
	s.mux.HandleFunc("GET /api/eventgrid/topics", s.handleEventGridTopics)
	s.mux.HandleFunc("GET /api/eventgrid/events", s.handleEventGridEvents)

	return s
}

// Handler returns the root handler.
func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) ctx(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), requestTimeout)
}

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func jsonErr(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

// ---- overview ----

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	svc := s.cfg.Services
	jsonOK(w, map[string]any{
		"services": map[string]bool{
			"blob":       svc.Blob != nil,
			"queue":      svc.Queue != nil,
			"table":      svc.Table != nil,
			"cosmos":     svc.Cosmos != nil,
			"servicebus": svc.ServiceBus != nil,
			"keyvault":   svc.KeyVault != nil,
			"eventgrid":  svc.EventGrid != nil,
		},
		"connectionStrings": provision.ConnectionStrings(s.cfg),
	})
}

// ---- blob ----

func (s *Server) handleBlobContainers(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := s.ctx(r)
	defer cancel()
	client, err := azblob.NewClientFromConnectionString(provision.StorageConnString(s.cfg), nil)
	if err != nil {
		jsonErr(w, 500, err)
		return
	}
	var names []string
	pager := client.NewListContainersPager(nil)
	for pager.More() && len(names) < maxItems {
		page, err := pager.NextPage(ctx)
		if err != nil {
			jsonErr(w, 502, fmt.Errorf("list containers: %w", err))
			return
		}
		for _, c := range page.ContainerItems {
			names = append(names, *c.Name)
		}
	}
	jsonOK(w, names)
}

func (s *Server) handleBlobList(w http.ResponseWriter, r *http.Request) {
	container := r.URL.Query().Get("container")
	if container == "" {
		jsonErr(w, 400, fmt.Errorf("container parameter is required"))
		return
	}
	prefix := r.URL.Query().Get("prefix")
	ctx, cancel := s.ctx(r)
	defer cancel()
	client, err := azblob.NewClientFromConnectionString(provision.StorageConnString(s.cfg), nil)
	if err != nil {
		jsonErr(w, 500, err)
		return
	}
	type blobInfo struct {
		Name         string    `json:"name"`
		Size         int64     `json:"size"`
		ContentType  string    `json:"contentType,omitempty"`
		LastModified time.Time `json:"lastModified"`
	}
	blobs := []blobInfo{}
	opts := &azblob.ListBlobsFlatOptions{}
	if prefix != "" {
		opts.Prefix = &prefix
	}
	pager := client.NewListBlobsFlatPager(container, opts)
	for pager.More() && len(blobs) < maxItems {
		page, err := pager.NextPage(ctx)
		if err != nil {
			jsonErr(w, 502, fmt.Errorf("list blobs in %q: %w", container, err))
			return
		}
		for _, b := range page.Segment.BlobItems {
			info := blobInfo{Name: *b.Name}
			if p := b.Properties; p != nil {
				if p.ContentLength != nil {
					info.Size = *p.ContentLength
				}
				if p.ContentType != nil {
					info.ContentType = *p.ContentType
				}
				if p.LastModified != nil {
					info.LastModified = *p.LastModified
				}
			}
			blobs = append(blobs, info)
		}
	}
	jsonOK(w, blobs)
}

// ---- queue ----

func (s *Server) handleQueues(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := s.ctx(r)
	defer cancel()
	svc, err := azqueue.NewServiceClientFromConnectionString(provision.StorageConnString(s.cfg), nil)
	if err != nil {
		jsonErr(w, 500, err)
		return
	}
	names := []string{}
	pager := svc.NewListQueuesPager(nil)
	for pager.More() && len(names) < maxItems {
		page, err := pager.NextPage(ctx)
		if err != nil {
			jsonErr(w, 502, fmt.Errorf("list queues: %w", err))
			return
		}
		for _, q := range page.Queues {
			names = append(names, *q.Name)
		}
	}
	jsonOK(w, names)
}

func (s *Server) handleQueuePeek(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		jsonErr(w, 400, fmt.Errorf("name parameter is required"))
		return
	}
	ctx, cancel := s.ctx(r)
	defer cancel()
	svc, err := azqueue.NewServiceClientFromConnectionString(provision.StorageConnString(s.cfg), nil)
	if err != nil {
		jsonErr(w, 500, err)
		return
	}
	n := int32(32) // queue service peek maximum
	resp, err := svc.NewQueueClient(name).PeekMessages(ctx, &azqueue.PeekMessagesOptions{NumberOfMessages: &n})
	if err != nil {
		jsonErr(w, 502, fmt.Errorf("peek queue %q: %w", name, err))
		return
	}
	type msg struct {
		ID    string `json:"id"`
		Text  string `json:"text"`
		Count int64  `json:"dequeueCount"`
	}
	msgs := []msg{}
	for _, m := range resp.Messages {
		out := msg{}
		if m.MessageID != nil {
			out.ID = *m.MessageID
		}
		if m.MessageText != nil {
			out.Text = decodeQueueText(*m.MessageText)
		}
		if m.DequeueCount != nil {
			out.Count = *m.DequeueCount
		}
		msgs = append(msgs, out)
	}
	jsonOK(w, msgs)
}

// decodeQueueText shows base64-encoded queue payloads (the SDK default for
// many Azure SDKs) as text when they decode to valid UTF-8.
func decodeQueueText(s string) string {
	if decoded, err := base64.StdEncoding.DecodeString(s); err == nil && utf8.Valid(decoded) {
		return string(decoded)
	}
	return s
}

// ---- table ----

func (s *Server) handleTables(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := s.ctx(r)
	defer cancel()
	svc, err := aztables.NewServiceClientFromConnectionString(provision.StorageConnString(s.cfg), nil)
	if err != nil {
		jsonErr(w, 500, err)
		return
	}
	names := []string{}
	pager := svc.NewListTablesPager(nil)
	for pager.More() && len(names) < maxItems {
		page, err := pager.NextPage(ctx)
		if err != nil {
			jsonErr(w, 502, fmt.Errorf("list tables: %w", err))
			return
		}
		for _, t := range page.Tables {
			names = append(names, *t.Name)
		}
	}
	jsonOK(w, names)
}

func (s *Server) handleTableRows(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		jsonErr(w, 400, fmt.Errorf("name parameter is required"))
		return
	}
	ctx, cancel := s.ctx(r)
	defer cancel()
	svc, err := aztables.NewServiceClientFromConnectionString(provision.StorageConnString(s.cfg), nil)
	if err != nil {
		jsonErr(w, 500, err)
		return
	}
	top := int32(maxItems)
	pager := svc.NewClient(name).NewListEntitiesPager(&aztables.ListEntitiesOptions{Top: &top})
	rows := []map[string]any{}
	for pager.More() && len(rows) < maxItems {
		page, err := pager.NextPage(ctx)
		if err != nil {
			jsonErr(w, 502, fmt.Errorf("list rows in %q: %w", name, err))
			return
		}
		for _, e := range page.Entities {
			var row map[string]any
			if err := json.Unmarshal(e, &row); err == nil {
				rows = append(rows, row)
			}
		}
	}
	jsonOK(w, rows)
}

// ---- cosmos ----

func (s *Server) handleCosmosMeta(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Services.Cosmos == nil {
		jsonOK(w, []any{})
		return
	}
	jsonOK(w, s.cfg.Services.Cosmos.Databases)
}

func (s *Server) handleCosmosQuery(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Database  string `json:"database"`
		Container string `json:"container"`
		Query     string `json:"query"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, fmt.Errorf("invalid request: %w", err))
		return
	}
	if req.Database == "" || req.Container == "" || req.Query == "" {
		jsonErr(w, 400, fmt.Errorf("database, container, and query are required"))
		return
	}
	ctx, cancel := s.ctx(r)
	defer cancel()
	client, err := provision.CosmosClient(s.cfg)
	if err != nil {
		jsonErr(w, 500, err)
		return
	}
	cc, err := client.NewContainer(req.Database, req.Container)
	if err != nil {
		jsonErr(w, 500, err)
		return
	}
	// An empty partition key makes the query cross-partition.
	pager := cc.NewQueryItemsPager(req.Query, azcosmos.NewPartitionKey(), nil)
	docs := []json.RawMessage{}
	for pager.More() && len(docs) < maxItems {
		page, err := pager.NextPage(ctx)
		if err != nil {
			jsonErr(w, 502, fmt.Errorf("query %s/%s: %w", req.Database, req.Container, err))
			return
		}
		for _, item := range page.Items {
			docs = append(docs, json.RawMessage(item))
		}
	}
	jsonOK(w, docs)
}

// ---- service bus ----

func (s *Server) handleServiceBusMeta(w http.ResponseWriter, r *http.Request) {
	sb := s.cfg.Services.ServiceBus
	if sb == nil {
		jsonOK(w, map[string]any{"queues": []string{}, "topics": []any{}})
		return
	}
	queues := sb.Queues
	if queues == nil {
		queues = []string{}
	}
	topics := sb.Topics
	if topics == nil {
		topics = []config.ServiceBusTopic{}
	}
	jsonOK(w, map[string]any{"queues": queues, "topics": topics})
}

func (s *Server) handleServiceBusPeek(w http.ResponseWriter, r *http.Request) {
	entity := r.URL.Query().Get("entity")
	if entity == "" {
		jsonErr(w, 400, fmt.Errorf("entity parameter is required"))
		return
	}
	subscription := r.URL.Query().Get("subscription")
	max := 20
	if v := r.URL.Query().Get("max"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= maxItems {
			max = n
		}
	}

	ctx, cancel := s.ctx(r)
	defer cancel()
	client, err := azservicebus.NewClientFromConnectionString(provision.ServiceBusConnString(), nil)
	if err != nil {
		jsonErr(w, 500, err)
		return
	}
	defer client.Close(ctx)

	var receiver *azservicebus.Receiver
	if subscription != "" {
		receiver, err = client.NewReceiverForSubscription(entity, subscription, nil)
	} else {
		receiver, err = client.NewReceiverForQueue(entity, nil)
	}
	if err != nil {
		jsonErr(w, 502, fmt.Errorf("open receiver for %q: %w", entity, err))
		return
	}
	defer receiver.Close(ctx)

	peeked, err := receiver.PeekMessages(ctx, max, nil)
	if err != nil {
		jsonErr(w, 502, fmt.Errorf("peek %q: %w", entity, err))
		return
	}
	type msg struct {
		MessageID string `json:"messageId"`
		Body      string `json:"body"`
		Sequence  int64  `json:"sequenceNumber"`
	}
	msgs := []msg{}
	for _, m := range peeked {
		out := msg{MessageID: m.MessageID, Body: string(m.Body)}
		if m.SequenceNumber != nil {
			out.Sequence = *m.SequenceNumber
		}
		msgs = append(msgs, out)
	}
	jsonOK(w, msgs)
}

// ---- mock proxies (key vault / event grid run in the mocks daemon) ----

// proxyGet fetches a mock endpoint and relays the JSON response.
func proxyGet(w http.ResponseWriter, url string, auth bool) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		jsonErr(w, 500, err)
		return
	}
	if auth {
		req.Header.Set("Authorization", "Bearer azlocal-ui")
	}
	resp, err := provision.InsecureHTTPClient().Do(req)
	if err != nil {
		jsonErr(w, 502, fmt.Errorf("mock not reachable (is \"azlocal up\" running?): %w", err))
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (s *Server) handleKeyVaultSecrets(w http.ResponseWriter, r *http.Request) {
	proxyGet(w, provision.KeyVaultEndpoint(s.cfg)+"/secrets?api-version=7.4", true)
}

func (s *Server) handleKeyVaultSecret(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		jsonErr(w, 400, fmt.Errorf("name parameter is required"))
		return
	}
	proxyGet(w, provision.KeyVaultEndpoint(s.cfg)+"/secrets/"+name+"?api-version=7.4", true)
}

func (s *Server) handleEventGridTopics(w http.ResponseWriter, r *http.Request) {
	proxyGet(w, provision.EventGridEndpoint(s.cfg)+"/topics", false)
}

func (s *Server) handleEventGridEvents(w http.ResponseWriter, r *http.Request) {
	topic := r.URL.Query().Get("topic")
	if topic == "" {
		jsonErr(w, 400, fmt.Errorf("topic parameter is required"))
		return
	}
	proxyGet(w, provision.EventGridEndpoint(s.cfg)+"/"+topic+"/events", false)
}
