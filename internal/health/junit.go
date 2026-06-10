package health

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
)

// JUnit XML schema subset, compatible with the de-facto junit-4 format
// consumed by GitHub Actions annotators, GitLab, Jenkins, and Azure DevOps.
type junitTestsuites struct {
	XMLName  xml.Name         `xml:"testsuites"`
	Tests    int              `xml:"tests,attr"`
	Failures int              `xml:"failures,attr"`
	Suites   []junitTestsuite `xml:"testsuite"`
}

type junitTestsuite struct {
	Name      string          `xml:"name,attr"`
	Tests     int             `xml:"tests,attr"`
	Failures  int             `xml:"failures,attr"`
	Timestamp string          `xml:"timestamp,attr"`
	Cases     []junitTestcase `xml:"testcase"`
}

type junitTestcase struct {
	Name      string        `xml:"name,attr"`
	Classname string        `xml:"classname,attr"`
	Failure   *junitFailure `xml:"failure,omitempty"`
}

type junitFailure struct {
	Message string `xml:"message,attr"`
	Body    string `xml:",chardata"`
}

// JUnit renders the report as a JUnit XML document.
func (r *Report) JUnit() ([]byte, error) {
	suite := junitTestsuite{
		Name:      "azlocal health",
		Tests:     len(r.Services),
		Timestamp: r.Timestamp.Format("2006-01-02T15:04:05"),
	}
	for _, s := range r.Services {
		tc := junitTestcase{Name: s.Service, Classname: "azlocal." + r.Project}
		if !s.Ok {
			suite.Failures++
			tc.Failure = &junitFailure{
				Message: fmt.Sprintf("service %q is not healthy", s.Service),
				Body:    failureMessage(s),
			}
		}
		suite.Cases = append(suite.Cases, tc)
	}
	doc := junitTestsuites{
		Tests:    suite.Tests,
		Failures: suite.Failures,
		Suites:   []junitTestsuite{suite},
	}
	out, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), out...), nil
}

// WriteJUnit writes the JUnit report to path, creating parent directories.
func (r *Report) WriteJUnit(path string) error {
	data, err := r.JUnit()
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0o644)
}
