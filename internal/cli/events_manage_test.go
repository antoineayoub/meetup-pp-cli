package cli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type testRoundTripper func(*http.Request) (*http.Response, error)

func (f testRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func str(v string) *string                                                   { return &v }
func integer(v int) *int                                                     { return &v }

func TestEditEventVariablesOnlyIncludeProvidedFields(t *testing.T) {
	vars, err := buildEditEventVariables(editEventInput{EventID: "315809698", Title: str("New"), DurationMins: integer(90), HowToFindUs: str("https://example.com")})
	if err != nil {
		t.Fatal(err)
	}
	input := vars["input"].(map[string]any)
	if input["eventId"] != "315809698" || input["title"] != "New" || input["duration"] != "PT90M" || input["howToFindUs"] != "https://example.com" {
		t.Fatalf("unexpected variables: %#v", input)
	}
	if _, ok := input["topics"]; ok {
		t.Fatalf("topics must stay absent: %#v", input)
	}
}
func TestEditEventVariablesRepeatTopicsAndHosts(t *testing.T) {
	vars, _ := buildEditEventVariables(editEventInput{EventID: "1", TopicIDs: []string{"10", "42"}, HostIDs: []string{"2", "3"}})
	input := vars["input"].(map[string]any)
	if len(input["topics"].([]string)) != 2 || len(input["eventHosts"].([]int)) != 2 {
		t.Fatalf("expected repeated topics/hosts to be preserved: %#v", input)
	}
}

func TestEditEventVariablesRequireNonEmptyTopicIDs(t *testing.T) {
	if _, err := buildEditEventVariables(editEventInput{EventID: "1", TopicIDs: []string{" "}}); err == nil {
		t.Fatal("expected error for empty topic ID")
	}
}

func TestEditEventVariablesRequireNumericHostIDs(t *testing.T) {
	_, err := buildEditEventVariables(editEventInput{EventID: "1", HostIDs: []string{"abc"}})
	if err == nil {
		t.Fatal("expected numeric host id validation error")
	}
}
func TestPhotoFormats(t *testing.T) {
	for p := range map[string]bool{"x.jpg": true, "x.jpeg": true, "x.png": true, "x.gif": true, "x.webp": true} {
		if _, err := photoContentType(p); err != nil {
			t.Fatal(err)
		}
	}
}
func TestPublishIdempotentWhenAlreadyActive(t *testing.T) {
	calls := 0
	gc := &gqlClient{httpClient: &http.Client{Transport: testRoundTripper(func(r *http.Request) (*http.Response, error) {
		calls++
		var req gqlRequest
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &req)
		if !strings.Contains(req.Query, "groupByUrlname") {
			t.Fatalf("unexpected mutation: %s", req.Query)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":{"groupByUrlname":{"events":{"edges":[{"node":{"id":"1","status":"ACTIVE","eventUrl":"https://meetup.test","title":"Live"}}],"pageInfo":{"hasNextPage":false}}}}}`)), Header: make(http.Header)}, nil
	})}}
	event, changed, err := publishEventDraft(context.Background(), gc, "group", "1")
	if err != nil || changed || event.Status != "ACTIVE" || calls != 1 {
		t.Fatalf("event=%#v changed=%v err=%v calls=%d", event, changed, err, calls)
	}
}
