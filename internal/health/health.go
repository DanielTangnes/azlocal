// Package health inspects the running azlocal compose project and renders
// machine-readable health reports (JSON, JUnit XML) for CI pipelines.
package health

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// ServiceHealth describes the observed state of one compose service.
type ServiceHealth struct {
	Service  string `json:"service"`
	Name     string `json:"name,omitempty"`
	State    string `json:"state"`            // running, exited, ...
	Health   string `json:"health,omitempty"` // healthy, unhealthy, starting, "" (no healthcheck)
	ExitCode int    `json:"exitCode"`
	Ok       bool   `json:"ok"`
}

// Report is the full health report for the project.
type Report struct {
	Project   string          `json:"project"`
	Timestamp time.Time       `json:"timestamp"`
	Services  []ServiceHealth `json:"services"`
	Ok        bool            `json:"ok"`
}

// composePS mirrors the fields we need from `docker compose ps --format json`.
type composePS struct {
	Name     string `json:"Name"`
	Service  string `json:"Service"`
	State    string `json:"State"`
	Health   string `json:"Health"`
	ExitCode int    `json:"ExitCode"`
}

// Check shells out to docker compose and builds a Report. expected lists
// service names that must be present; any that are missing are reported as
// state "absent" (e.g. the container never started or was removed).
func Check(ctx context.Context, project string, expected []string) (*Report, error) {
	out, err := exec.CommandContext(ctx, "docker", "compose", "-p", project, "ps", "-a", "--format", "json").Output()
	if err != nil {
		return nil, fmt.Errorf("docker compose ps: %w", err)
	}
	return buildReport(project, out, expected)
}

// buildReport parses compose ps output (NDJSON in compose >= 2.21, a JSON
// array in older releases) and merges in expected-but-absent services.
func buildReport(project string, out []byte, expected []string) (*Report, error) {
	entries, err := parsePS(out)
	if err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	r := &Report{Project: project, Timestamp: time.Now().UTC(), Ok: true}
	for _, e := range entries {
		sh := ServiceHealth{
			Service:  e.Service,
			Name:     e.Name,
			State:    e.State,
			Health:   e.Health,
			ExitCode: e.ExitCode,
		}
		sh.Ok = e.State == "running" && (e.Health == "" || e.Health == "healthy")
		seen[e.Service] = true
		r.Services = append(r.Services, sh)
	}
	for _, svc := range expected {
		if !seen[svc] {
			r.Services = append(r.Services, ServiceHealth{Service: svc, State: "absent"})
		}
	}
	sort.Slice(r.Services, func(i, j int) bool { return r.Services[i].Service < r.Services[j].Service })
	for _, s := range r.Services {
		if !s.Ok {
			r.Ok = false
			break
		}
	}
	return r, nil
}

func parsePS(out []byte) ([]composePS, error) {
	trimmed := bytes.TrimSpace(out)
	if len(trimmed) == 0 {
		return nil, nil
	}
	if trimmed[0] == '[' {
		var arr []composePS
		if err := json.Unmarshal(trimmed, &arr); err != nil {
			return nil, fmt.Errorf("parse compose ps output: %w", err)
		}
		return arr, nil
	}
	var entries []composePS
	sc := bufio.NewScanner(bytes.NewReader(trimmed))
	sc.Buffer(make([]byte, 0, 1<<16), 1<<22)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var e composePS
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, fmt.Errorf("parse compose ps line %q: %w", line, err)
		}
		entries = append(entries, e)
	}
	return entries, sc.Err()
}

// JSON renders the report as indented JSON.
func (r *Report) JSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

// Summary returns a one-line human summary, e.g. "5/6 services healthy".
func (r *Report) Summary() string {
	ok := 0
	for _, s := range r.Services {
		if s.Ok {
			ok++
		}
	}
	return fmt.Sprintf("%d/%d services healthy", ok, len(r.Services))
}

// failureMessage describes why a service is considered unhealthy.
func failureMessage(s ServiceHealth) string {
	var b strings.Builder
	fmt.Fprintf(&b, "state=%s", s.State)
	if s.Health != "" {
		fmt.Fprintf(&b, " health=%s", s.Health)
	}
	if s.State == "exited" {
		fmt.Fprintf(&b, " exitCode=%d", s.ExitCode)
	}
	return b.String()
}
