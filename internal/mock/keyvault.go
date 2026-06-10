package mock

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// KeyVault is an in-memory mock of the Azure Key Vault secrets REST API
// (api-version 7.x): set, get, list, versions, and delete. It is enough for
// the common local-dev pattern of reading configuration secrets through the
// azsecrets / SecretClient SDKs.
//
// Authentication: like the real service, requests without an Authorization
// header receive a 401 bearer challenge (which Azure SDK clients require to
// kick off their auth flow); any non-empty bearer token is then accepted.
type KeyVault struct {
	mu      sync.RWMutex
	secrets map[string][]secretVersion // name -> versions, oldest first
}

type secretVersion struct {
	Version     string
	Value       string
	ContentType string
	Tags        map[string]string
	Created     time.Time
}

// NewKeyVault builds a vault pre-populated with the given secrets.
func NewKeyVault(seed map[string]string) *KeyVault {
	kv := &KeyVault{secrets: map[string][]secretVersion{}}
	for name, value := range seed {
		kv.put(name, value, "", nil)
	}
	return kv
}

func (kv *KeyVault) put(name, value, contentType string, tags map[string]string) secretVersion {
	v := secretVersion{
		Version:     newVersionID(),
		Value:       value,
		ContentType: contentType,
		Tags:        tags,
		Created:     time.Now().UTC(),
	}
	kv.mu.Lock()
	kv.secrets[name] = append(kv.secrets[name], v)
	kv.mu.Unlock()
	return v
}

func newVersionID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Names returns all secret names, sorted (used by the web UI).
func (kv *KeyVault) Names() []string {
	kv.mu.RLock()
	defer kv.mu.RUnlock()
	names := make([]string, 0, len(kv.secrets))
	for n := range kv.secrets {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Handler returns the HTTP handler implementing the secrets API.
func (kv *KeyVault) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", kv.route)
	return mux
}

// kvError mirrors Key Vault's error envelope.
func kvError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{"code": code, "message": msg},
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// bundle renders a secret bundle (the GET/PUT/DELETE response shape).
func (kv *KeyVault) bundle(r *http.Request, name string, v secretVersion, includeValue bool) map[string]any {
	id := fmt.Sprintf("https://%s/secrets/%s/%s", r.Host, name, v.Version)
	b := map[string]any{
		"id": id,
		"attributes": map[string]any{
			"enabled":       true,
			"created":       v.Created.Unix(),
			"updated":       v.Created.Unix(),
			"recoveryLevel": "Purgeable",
		},
	}
	if includeValue {
		b["value"] = v.Value
	}
	if v.ContentType != "" {
		b["contentType"] = v.ContentType
	}
	if len(v.Tags) > 0 {
		b["tags"] = v.Tags
	}
	return b
}

func (kv *KeyVault) route(w http.ResponseWriter, r *http.Request) {
	// Bearer challenge: Azure SDK clients send an initial unauthenticated
	// probe and require a 401 + WWW-Authenticate to start their auth flow.
	if r.Header.Get("Authorization") == "" {
		w.Header().Set("WWW-Authenticate",
			fmt.Sprintf(`Bearer authorization="https://login.microsoftonline.com/azlocal", resource="https://%s"`, hostOnly(r.Host)))
		kvError(w, http.StatusUnauthorized, "Unauthorized", "missing bearer token (any token is accepted by this mock)")
		return
	}

	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if parts[0] != "secrets" {
		kvError(w, http.StatusNotFound, "NotFound", "this mock implements the /secrets API only")
		return
	}

	switch {
	case len(parts) == 1 && r.Method == http.MethodGet: // list secrets
		kv.handleList(w, r)
	case len(parts) == 2 && r.Method == http.MethodPut: // set secret
		kv.handleSet(w, r, parts[1])
	case len(parts) == 2 && r.Method == http.MethodGet: // get latest
		kv.handleGet(w, r, parts[1], "")
	case len(parts) == 2 && r.Method == http.MethodDelete: // delete secret
		kv.handleDelete(w, r, parts[1])
	case len(parts) == 3 && parts[2] == "versions" && r.Method == http.MethodGet:
		kv.handleVersions(w, r, parts[1])
	case len(parts) == 3 && r.Method == http.MethodGet: // get specific version
		kv.handleGet(w, r, parts[1], parts[2])
	default:
		kvError(w, http.StatusMethodNotAllowed, "MethodNotAllowed",
			fmt.Sprintf("%s %s is not supported by this mock", r.Method, r.URL.Path))
	}
}

func hostOnly(hostport string) string {
	if i := strings.LastIndex(hostport, ":"); i >= 0 && !strings.Contains(hostport, "]") {
		return hostport[:i]
	}
	return hostport
}

func (kv *KeyVault) handleSet(w http.ResponseWriter, r *http.Request, name string) {
	var body struct {
		Value       string            `json:"value"`
		ContentType string            `json:"contentType"`
		Tags        map[string]string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		kvError(w, http.StatusBadRequest, "BadParameter", "invalid request body: "+err.Error())
		return
	}
	v := kv.put(name, body.Value, body.ContentType, body.Tags)
	writeJSON(w, http.StatusOK, kv.bundle(r, name, v, true))
}

func (kv *KeyVault) handleGet(w http.ResponseWriter, r *http.Request, name, version string) {
	kv.mu.RLock()
	versions := kv.secrets[name]
	kv.mu.RUnlock()
	if len(versions) == 0 {
		kvError(w, http.StatusNotFound, "SecretNotFound", fmt.Sprintf("a secret with name %q was not found", name))
		return
	}
	if version == "" {
		writeJSON(w, http.StatusOK, kv.bundle(r, name, versions[len(versions)-1], true))
		return
	}
	for _, v := range versions {
		if v.Version == version {
			writeJSON(w, http.StatusOK, kv.bundle(r, name, v, true))
			return
		}
	}
	kvError(w, http.StatusNotFound, "SecretNotFound", fmt.Sprintf("secret %q has no version %q", name, version))
}

func (kv *KeyVault) handleDelete(w http.ResponseWriter, r *http.Request, name string) {
	kv.mu.Lock()
	versions := kv.secrets[name]
	delete(kv.secrets, name)
	kv.mu.Unlock()
	if len(versions) == 0 {
		kvError(w, http.StatusNotFound, "SecretNotFound", fmt.Sprintf("a secret with name %q was not found", name))
		return
	}
	b := kv.bundle(r, name, versions[len(versions)-1], true)
	b["recoveryId"] = fmt.Sprintf("https://%s/deletedsecrets/%s", r.Host, name)
	b["deletedDate"] = time.Now().Unix()
	b["scheduledPurgeDate"] = time.Now().Unix()
	writeJSON(w, http.StatusOK, b)
}

func (kv *KeyVault) handleList(w http.ResponseWriter, r *http.Request) {
	kv.mu.RLock()
	items := make([]map[string]any, 0, len(kv.secrets))
	names := make([]string, 0, len(kv.secrets))
	for n := range kv.secrets {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		versions := kv.secrets[n]
		latest := versions[len(versions)-1]
		item := kv.bundle(r, n, latest, false)
		item["id"] = fmt.Sprintf("https://%s/secrets/%s", r.Host, n) // list items use unversioned IDs
		items = append(items, item)
	}
	kv.mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]any{"value": items, "nextLink": nil})
}

func (kv *KeyVault) handleVersions(w http.ResponseWriter, r *http.Request, name string) {
	kv.mu.RLock()
	versions := kv.secrets[name]
	kv.mu.RUnlock()
	if len(versions) == 0 {
		kvError(w, http.StatusNotFound, "SecretNotFound", fmt.Sprintf("a secret with name %q was not found", name))
		return
	}
	items := make([]map[string]any, 0, len(versions))
	for _, v := range versions {
		items = append(items, kv.bundle(r, name, v, false))
	}
	writeJSON(w, http.StatusOK, map[string]any{"value": items, "nextLink": nil})
}
