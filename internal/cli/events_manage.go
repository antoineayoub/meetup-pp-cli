package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

const mutationEditEvent = `mutation EditEvent($input: EditEventInput!) { editEvent(input: $input) { event { id title dateTime endTime status eventUrl } } }`
const mutationPublishEventDraft = `mutation PublishEventDraft($eventId: ID!) { publishEventDraft(eventId: $eventId) { event { id title dateTime endTime status eventUrl } } }`
const queryListEventHosts = `query ListEventHosts($urlname: String!) { groupByUrlname(urlname: $urlname) { eventHosts { id name isEligible } } }`

type editEventInput struct {
	EventID                                    string
	Title, Description, StartDateTime, VenueID *string
	Location                                   *string
	DurationMins                               *int
	TopicIDs, HostIDs                          []string
}
type eventHost struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Eligible bool   `json:"eligible"`
}

func buildEditEventVariables(in editEventInput) (map[string]any, error) {
	if strings.TrimSpace(in.EventID) == "" {
		return nil, fmt.Errorf("event ID is required")
	}
	body := map[string]any{"eventId": in.EventID}
	if in.Title != nil {
		body["title"] = *in.Title
	}
	if in.Description != nil {
		body["description"] = *in.Description
	}
	if in.StartDateTime != nil {
		body["startDateTime"] = *in.StartDateTime
	}
	if in.DurationMins != nil {
		body["duration"] = fmt.Sprintf("PT%dM", *in.DurationMins)
	}
	if in.VenueID != nil {
		body["venueId"] = *in.VenueID
	}
	if in.Location != nil {
		body["location"] = map[string]any{"name": *in.Location}
	}
	if len(in.TopicIDs) > 0 {
		topics := make([]map[string]any, 0, len(in.TopicIDs))
		for _, id := range in.TopicIDs {
			topics = append(topics, map[string]any{"id": id})
		}
		body["topics"] = topics
	}
	if len(in.HostIDs) > 0 {
		hosts := make([]map[string]any, 0, len(in.HostIDs))
		for _, id := range in.HostIDs {
			hosts = append(hosts, map[string]any{"id": id})
		}
		body["eventHosts"] = hosts
	}
	return map[string]any{"input": body}, nil
}

func findGroupEventByID(ctx context.Context, gc *gqlClient, group, id string) (*gqlEvent, error) {
	for _, status := range []string{"ACTIVE", "DRAFT", "PAST", "CANCELLED"} {
		events, err := fetchAllGroupEvents(ctx, gc, group, status, 200)
		if err != nil {
			return nil, err
		}
		for _, event := range events {
			if event.ID == id {
				copy := event
				return &copy, nil
			}
		}
	}
	return nil, nil
}

func publishEventDraft(ctx context.Context, gc *gqlClient, group, id string) (*gqlEvent, bool, error) {
	event, err := findGroupEventByID(ctx, gc, group, id)
	if err != nil {
		return nil, false, err
	}
	if event == nil {
		return nil, false, fmt.Errorf("event not found: %s", id)
	}
	if event.Status == "ACTIVE" {
		return event, false, nil
	}
	var data struct {
		PublishEventDraft *struct {
			Event *gqlEvent `json:"event"`
		} `json:"publishEventDraft"`
	}
	if err := gc.do(ctx, mutationPublishEventDraft, map[string]any{"eventId": id}, &data); err != nil {
		return nil, false, err
	}
	verified, err := findGroupEventByID(ctx, gc, group, id)
	if err != nil {
		return nil, false, err
	}
	if verified == nil || verified.Status != "ACTIVE" {
		return nil, false, fmt.Errorf("publish did not reach ACTIVE status")
	}
	return verified, true, nil
}

func listEventHosts(ctx context.Context, gc *gqlClient, group string) ([]eventHost, error) {
	var data struct {
		GroupByUrlname *struct {
			EventHosts []struct {
				ID       string `json:"id"`
				Name     string `json:"name"`
				Eligible bool   `json:"isEligible"`
			} `json:"eventHosts"`
		} `json:"groupByUrlname"`
	}
	if err := gc.do(ctx, queryListEventHosts, map[string]any{"urlname": group}, &data); err != nil {
		return nil, err
	}
	if data.GroupByUrlname == nil {
		return nil, fmt.Errorf("group not found: %s", group)
	}
	hosts := make([]eventHost, 0, len(data.GroupByUrlname.EventHosts))
	for _, h := range data.GroupByUrlname.EventHosts {
		hosts = append(hosts, eventHost{ID: h.ID, Name: h.Name, Eligible: h.Eligible})
	}
	sort.Slice(hosts, func(i, j int) bool { return hosts[i].Name < hosts[j].Name })
	return hosts, nil
}
