// Copyright 2026 Ayoub and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"meetup-pp-cli/internal/config"
)

const meetupGQLEndpoint = "https://api.meetup.com/gql-ext"

type gqlClient struct {
	httpClient *http.Client
	token      string // optional Bearer token from MEETUP_ACCESS_TOKEN or MEETUP_BEARER_AUTH
}

type gqlRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

type gqlResponse struct {
	Data   json.RawMessage  `json:"data"`
	Errors []gqlError       `json:"errors,omitempty"`
}

type gqlError struct {
	Message string `json:"message"`
}

func newGQLClient() *gqlClient {
	token := os.Getenv("MEETUP_ACCESS_TOKEN")
	if token == "" {
		token = os.Getenv("MEETUP_BEARER_AUTH")
	}
	if token == "" {
		if cfg, err := config.Load(""); err == nil {
			if h := cfg.AuthHeader(); strings.HasPrefix(h, "Bearer ") {
				token = strings.TrimPrefix(h, "Bearer ")
			}
		}
	}
	return &gqlClient{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		token:      token,
	}
}

// do executes a GraphQL request and unmarshals the response .data field into out.
func (c *gqlClient) do(ctx context.Context, query string, variables map[string]any, out any) error {
	body, err := json.Marshal(gqlRequest{Query: query, Variables: variables})
	if err != nil {
		return fmt.Errorf("marshaling GraphQL request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, meetupGQLEndpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing GraphQL request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode == 401 {
		return fmt.Errorf("unauthorized: set MEETUP_ACCESS_TOKEN with organizer scope")
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("API error HTTP %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}

	var gqlResp gqlResponse
	if err := json.Unmarshal(raw, &gqlResp); err != nil {
		return fmt.Errorf("parsing GraphQL response: %w", err)
	}
	if len(gqlResp.Errors) > 0 {
		return fmt.Errorf("GraphQL error: %s", gqlResp.Errors[0].Message)
	}

	if out != nil && gqlResp.Data != nil {
		if err := json.Unmarshal(gqlResp.Data, out); err != nil {
			return fmt.Errorf("unmarshaling data: %w", err)
		}
	}
	return nil
}

// ── GraphQL types ────────────────────────────────────────────────────────────

type gqlGroup struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Urlname     string      `json:"urlname"`
	Description string      `json:"description"`
	City        string      `json:"city"`
	State       string      `json:"state"`
	Country     string      `json:"country"`
	Lat         float64     `json:"lat"`
	Lon         float64     `json:"lon"`
	Link        string      `json:"link"`
	Stats       *gqlStats   `json:"stats,omitempty"`
	Topics      []gqlTopic  `json:"activeTopics,omitempty"`
}

type gqlStats struct {
	MemberCounts *gqlMemberCounts `json:"memberCounts,omitempty"`
}

type gqlMemberCounts struct {
	All int `json:"all"`
}

type gqlTopic struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	URLKey  string `json:"urlkey"`
}

type gqlRsvps struct {
	YesCount       int `json:"yesCount"`
	WaitlistCount  int `json:"waitlistCount"`
	TotalCount     int `json:"totalCount"`
}

type gqlEvent struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	DateTime    string     `json:"dateTime"`
	EndTime     string     `json:"endTime,omitempty"`
	Duration    string     `json:"duration"`
	Description string     `json:"description"`
	Status      string     `json:"status"`
	EventType   string     `json:"eventType,omitempty"`
	MaxTickets  int        `json:"maxTickets"`
	EventURL    string     `json:"eventUrl"`
	Rsvps       *gqlRsvps  `json:"rsvps,omitempty"`
	Group       *gqlGroup  `json:"group,omitempty"`
	Venue       *gqlVenue  `json:"venue,omitempty"`
	Topics      []gqlTopic `json:"topics,omitempty"`
}

// Going returns the yes-RSVP count (convenience accessor).
func (e *gqlEvent) Going() int {
	if e.Rsvps == nil {
		return 0
	}
	return e.Rsvps.YesCount
}

// IsOnline returns true when the event type is ONLINE.
func (e *gqlEvent) IsOnline() bool {
	return e.EventType == "ONLINE"
}

type gqlVenue struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Address string  `json:"address"`
	City    string  `json:"city"`
	Lat     float64 `json:"lat"`
	Lon     float64 `json:"lon"`
}

type gqlEdgeEvent struct {
	Node gqlEvent `json:"node"`
}

type gqlEdgeGroup struct {
	Node gqlGroup `json:"node"`
}

type gqlPageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

type gqlEventPage struct {
	Edges    []gqlEdgeEvent `json:"edges"`
	PageInfo gqlPageInfo    `json:"pageInfo"`
	Count    int            `json:"count"`
}

type gqlGroupPage struct {
	Edges    []gqlEdgeGroup `json:"edges"`
	PageInfo gqlPageInfo    `json:"pageInfo"`
	Count    int            `json:"count"`
}

// ── Common queries ───────────────────────────────────────────────────────────

const queryGetGroup = `
query GetGroup($urlname: String!) {
  groupByUrlname(urlname: $urlname) {
    id name urlname description city state country lat lon link
    stats { memberCounts { all } }
    activeTopics { id name urlkey }
  }
}`

const queryListGroupEvents = `
query ListGroupEvents($urlname: String!, $first: Int, $after: String, $status: [EventStatus!]) {
  groupByUrlname(urlname: $urlname) {
    events(first: $first, after: $after, filter: { status: $status }) {
      edges { node {
        id title dateTime endTime duration description status eventType
        maxTickets eventUrl
        rsvps { yesCount waitlistCount totalCount }
        venue { id name address city lat lon }
      }}
      pageInfo { hasNextPage endCursor }
    }
  }
}`

const querySearchEvents = `
query SearchEvents($query: String!, $lat: Float!, $lon: Float!, $radius: Float, $first: Int, $after: String) {
  eventSearch(filter: { query: $query, lat: $lat, lon: $lon, radius: $radius }, first: $first, after: $after) {
    edges { node {
      id title dateTime endTime duration description status eventType
      maxTickets eventUrl
      rsvps { yesCount waitlistCount }
      group { id name urlname city }
      venue { id name address city lat lon }
    }}
    pageInfo { hasNextPage endCursor }
  }
}`

const querySearchGroups = `
query SearchGroups($topic: String!, $lat: Float!, $lon: Float!, $radius: Float, $first: Int, $after: String) {
  groupSearch(filter: { query: $topic, lat: $lat, lon: $lon, radius: $radius }, first: $first, after: $after) {
    edges { node {
      id name urlname description city state country lat lon link
      stats { memberCounts { all } }
      activeTopics { id name urlkey }
    }}
    pageInfo { hasNextPage endCursor }
  }
}`

const queryRecommendedEvents = `
query RecommendedEvents($lat: Float!, $lon: Float!, $first: Int) {
  recommendedEvents(filter: { lat: $lat, lon: $lon }, first: $first) {
    edges { node {
      id title dateTime endTime duration description status eventType
      eventUrl maxTickets
      rsvps { yesCount waitlistCount }
      group { id name urlname city }
      venue { id name address city }
    }}
    pageInfo { hasNextPage endCursor }
  }
}`

// fetchAllGroupEvents fetches all events for a group with cursor pagination.
func fetchAllGroupEvents(ctx context.Context, gc *gqlClient, urlname string, status string, maxItems int) ([]gqlEvent, error) {
	var all []gqlEvent
	var cursor string
	pageSize := 50
	if maxItems > 0 && maxItems < pageSize {
		pageSize = maxItems
	}

	for {
		vars := map[string]any{
			"urlname": urlname,
			"first":   pageSize,
		}
		if cursor != "" {
			vars["after"] = cursor
		}
		if status != "" {
			vars["status"] = []string{status}
		}

		var data struct {
			GroupByUrlname *struct {
				Events gqlEventPage `json:"events"`
			} `json:"groupByUrlname"`
		}
		if err := gc.do(ctx, queryListGroupEvents, vars, &data); err != nil {
			return nil, err
		}
		if data.GroupByUrlname == nil {
			break
		}
		for _, e := range data.GroupByUrlname.Events.Edges {
			all = append(all, e.Node)
			if maxItems > 0 && len(all) >= maxItems {
				return all, nil
			}
		}
		if !data.GroupByUrlname.Events.PageInfo.HasNextPage {
			break
		}
		cursor = data.GroupByUrlname.Events.PageInfo.EndCursor
	}
	return all, nil
}
