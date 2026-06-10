package health

import (
	"strings"
	"testing"
)

const ndjson = `{"Name":"azlocal-azurite-1","Service":"azurite","State":"running","Health":"healthy","ExitCode":0}
{"Name":"azlocal-cosmos-1","Service":"cosmos","State":"running","Health":"starting","ExitCode":0}
{"Name":"azlocal-servicebus-1","Service":"servicebus","State":"exited","Health":"","ExitCode":1}
`

func TestBuildReport_NDJSON(t *testing.T) {
	r, err := buildReport("azlocal", []byte(ndjson), []string{"azurite", "cosmos", "servicebus", "servicebus-sql"})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Services) != 4 {
		t.Fatalf("got %d services, want 4 (including absent)", len(r.Services))
	}
	if r.Ok {
		t.Error("report should not be ok")
	}

	byName := map[string]ServiceHealth{}
	for _, s := range r.Services {
		byName[s.Service] = s
	}
	if !byName["azurite"].Ok {
		t.Error("azurite should be ok")
	}
	if byName["cosmos"].Ok {
		t.Error("cosmos (starting) should not be ok")
	}
	if byName["servicebus"].Ok {
		t.Error("exited servicebus should not be ok")
	}
	if got := byName["servicebus-sql"].State; got != "absent" {
		t.Errorf("missing service state = %q, want absent", got)
	}
}

func TestBuildReport_JSONArray(t *testing.T) {
	arr := `[{"Name":"azlocal-azurite-1","Service":"azurite","State":"running","Health":"healthy","ExitCode":0}]`
	r, err := buildReport("azlocal", []byte(arr), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Services) != 1 || !r.Ok {
		t.Fatalf("unexpected report: %+v", r)
	}
}

func TestBuildReport_NoHealthcheckRunningIsOk(t *testing.T) {
	line := `{"Name":"x","Service":"servicebus","State":"running","Health":"","ExitCode":0}`
	r, err := buildReport("azlocal", []byte(line), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Services[0].Ok {
		t.Error("running service without healthcheck should be ok")
	}
}

func TestJUnit(t *testing.T) {
	r, err := buildReport("azlocal", []byte(ndjson), nil)
	if err != nil {
		t.Fatal(err)
	}
	out, err := r.JUnit()
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{
		`tests="3"`,
		`failures="2"`,
		`<testcase name="azurite"`,
		`<failure message="service &#34;cosmos&#34; is not healthy"`,
		"state=exited exitCode=1",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("junit output missing %q\n%s", want, s)
		}
	}
}

func TestSummary(t *testing.T) {
	r, _ := buildReport("azlocal", []byte(ndjson), nil)
	if got := r.Summary(); got != "1/3 services healthy" {
		t.Errorf("summary = %q", got)
	}
}
