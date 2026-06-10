package mock

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func kvRequest(t *testing.T, srv *httptest.Server, method, path, body string) (*http.Response, map[string]any) {
	t.Helper()
	req, err := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer anything")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var decoded map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatalf("%s %s: decode response: %v", method, path, err)
	}
	return resp, decoded
}

func TestKeyVault_ChallengeWithoutAuth(t *testing.T) {
	srv := httptest.NewServer(NewKeyVault(nil).Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/secrets/foo?api-version=7.4")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	challenge := resp.Header.Get("WWW-Authenticate")
	if !strings.Contains(challenge, "Bearer authorization=") {
		t.Errorf("missing bearer challenge, got %q", challenge)
	}
}

func TestKeyVault_SetGetDeleteFlow(t *testing.T) {
	srv := httptest.NewServer(NewKeyVault(nil).Handler())
	defer srv.Close()

	resp, set := kvRequest(t, srv, http.MethodPut, "/secrets/db-password?api-version=7.4", `{"value":"hunter2","contentType":"text/plain"}`)
	if resp.StatusCode != 200 {
		t.Fatalf("set: status %d: %v", resp.StatusCode, set)
	}
	if set["value"] != "hunter2" || set["contentType"] != "text/plain" {
		t.Fatalf("set response: %v", set)
	}
	id, _ := set["id"].(string)
	if !strings.Contains(id, "/secrets/db-password/") {
		t.Fatalf("unexpected id %q", id)
	}

	_, got := kvRequest(t, srv, http.MethodGet, "/secrets/db-password?api-version=7.4", "")
	if got["value"] != "hunter2" {
		t.Fatalf("get response: %v", got)
	}

	// Specific version fetch via the id returned by set.
	version := id[strings.LastIndex(id, "/")+1:]
	_, byVersion := kvRequest(t, srv, http.MethodGet, "/secrets/db-password/"+version+"?api-version=7.4", "")
	if byVersion["value"] != "hunter2" {
		t.Fatalf("get by version: %v", byVersion)
	}

	resp, _ = kvRequest(t, srv, http.MethodDelete, "/secrets/db-password?api-version=7.4", "")
	if resp.StatusCode != 200 {
		t.Fatalf("delete: status %d", resp.StatusCode)
	}
	resp, _ = kvRequest(t, srv, http.MethodGet, "/secrets/db-password?api-version=7.4", "")
	if resp.StatusCode != 404 {
		t.Fatalf("get after delete: status %d, want 404", resp.StatusCode)
	}
}

func TestKeyVault_VersionsAndList(t *testing.T) {
	kv := NewKeyVault(map[string]string{"seeded": "v0"})
	srv := httptest.NewServer(kv.Handler())
	defer srv.Close()

	kvRequest(t, srv, http.MethodPut, "/secrets/seeded?api-version=7.4", `{"value":"v1"}`)
	kvRequest(t, srv, http.MethodPut, "/secrets/other?api-version=7.4", `{"value":"x"}`)

	// Latest wins.
	_, got := kvRequest(t, srv, http.MethodGet, "/secrets/seeded?api-version=7.4", "")
	if got["value"] != "v1" {
		t.Fatalf("latest = %v, want v1", got["value"])
	}

	_, versions := kvRequest(t, srv, http.MethodGet, "/secrets/seeded/versions?api-version=7.4", "")
	if n := len(versions["value"].([]any)); n != 2 {
		t.Fatalf("got %d versions, want 2", n)
	}
	// Version listings must not leak values.
	first := versions["value"].([]any)[0].(map[string]any)
	if _, leaked := first["value"]; leaked {
		t.Error("version list leaks secret values")
	}

	_, list := kvRequest(t, srv, http.MethodGet, "/secrets?api-version=7.4", "")
	if n := len(list["value"].([]any)); n != 2 {
		t.Fatalf("got %d secrets, want 2", n)
	}

	if names := kv.Names(); len(names) != 2 || names[0] != "other" {
		t.Errorf("Names() = %v", names)
	}
}

func TestKeyVault_NotFoundAndBadRoutes(t *testing.T) {
	srv := httptest.NewServer(NewKeyVault(nil).Handler())
	defer srv.Close()

	resp, body := kvRequest(t, srv, http.MethodGet, "/secrets/nope?api-version=7.4", "")
	if resp.StatusCode != 404 {
		t.Fatalf("status %d, want 404", resp.StatusCode)
	}
	errObj := body["error"].(map[string]any)
	if errObj["code"] != "SecretNotFound" {
		t.Errorf("error code = %v", errObj["code"])
	}

	resp, _ = kvRequest(t, srv, http.MethodGet, "/keys/foo?api-version=7.4", "")
	if resp.StatusCode != 404 {
		t.Errorf("keys API should 404, got %d", resp.StatusCode)
	}
}
