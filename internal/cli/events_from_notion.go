package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const notionAPIVersion = "2025-09-03"

// notionEvent is the minimum canonical payload needed to create a Le Wagon Meetup event.
type notionEvent struct {
	PageID      string
	Title       string
	Intro       string
	Start       string
	Duration    int
	Description string
	WWWLink     string
	Format      string
	HostIDs     []string
	TopicIDs    []string
	ImageURL    string
	VenueID     string
	Properties  map[string]json.RawMessage
}

type notionCreateResult struct {
	Group    string    `json:"group"`
	Event    *gqlEvent `json:"event"`
	PhotoOK  bool      `json:"photo_attached"`
	Callback bool      `json:"notion_callback_written"`
}

func newNovelEventsCreateFromNotionCmd(flags *rootFlags) *cobra.Command {
	var pageID, city string
	var draft, publish bool
	cmd := &cobra.Command{
		Use:   "create-from-notion",
		Short: "Create Le Wagon Meetup draft(s) from a canonical Notion event page.",
		Long: `Reads title, date, description, WWW Link, Format, Host ID, Topics and Image from Notion.

Online events create Montréal and Toronto drafts by default; onsite events create Montréal only. Hybrid events create Montréal onsite and Toronto online drafts. The command writes returned Meetup URLs back to Notion after successful creation and cover-photo attachment. It creates drafts by default; pass --publish only after explicit publication approval.`,
		Annotations: map[string]string{"mcp:read-only": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(pageID) == "" {
				return usageErr(fmt.Errorf("--page-id is required"))
			}
			if publish && cmd.Flags().Changed("draft") {
				return usageErr(fmt.Errorf("--publish and --draft cannot be combined"))
			}
			if publish {
				draft = false
			}
			event, err := loadNotionEvent(cmd.Context(), pageID)
			if err != nil {
				return err
			}
			groups, err := notionTargetGroups(event.Format, city)
			if err != nil {
				return usageErr(err)
			}
			if err := requireNotionCallbackFields(event.Properties, groups); err != nil {
				return err
			}
			if flags.dryRun {
				return flags.printJSON(cmd, map[string]any{"page_id": event.PageID, "title": event.Title, "groups": groups, "format": event.Format, "topics": event.TopicIDs, "hosts": event.HostIDs, "draft": draft})
			}
			photoPath, err := downloadNotionImage(cmd.Context(), event.ImageURL)
			if err != nil {
				return fmt.Errorf("Notion Image is required before creating Meetup event(s): %w", err)
			}
			defer os.Remove(photoPath)
			gc := newGQLClient()
			if gc.token == "" {
				return fmt.Errorf("events create-from-notion requires Meetup authentication — run: meetup-pp-cli auth login")
			}
			results := make([]notionCreateResult, 0, len(groups))
			for _, group := range groups {
				ev, err := createMeetupFromNotion(cmd.Context(), gc, event, group, draft)
				if err != nil {
					return fmt.Errorf("creating %s: %w", group, err)
				}
				if err := uploadEventPhoto(cmd.Context(), gc, group, ev.ID, photoPath); err != nil {
					return fmt.Errorf("event %s was created but its mandatory photo failed to attach: %w", ev.ID, err)
				}
				property := meetupURLPropertyForGroup(group)
				if err := writeNotionURL(cmd.Context(), event.PageID, property, ev.EventURL); err != nil {
					return fmt.Errorf("event %s was created but Notion callback failed: %w", ev.ID, err)
				}
				results = append(results, notionCreateResult{Group: group, Event: ev, PhotoOK: true, Callback: true})
			}
			return flags.printJSON(cmd, results)
		},
	}
	cmd.Flags().StringVar(&pageID, "page-id", "", "Canonical Notion event page ID (required)")
	cmd.Flags().StringVar(&city, "city", "auto", "Target: auto, montreal, toronto, or both")
	cmd.Flags().BoolVar(&draft, "draft", true, "Create drafts (default true)")
	cmd.Flags().BoolVar(&publish, "publish", false, "Publish immediately instead of creating drafts (requires explicit approval)")
	_ = cmd.MarkFlagRequired("page-id")
	return cmd
}

func loadNotionEvent(ctx context.Context, pageID string) (*notionEvent, error) {
	key := strings.TrimSpace(os.Getenv("NOTION_API_KEY"))
	if key == "" {
		return nil, fmt.Errorf("NOTION_API_KEY is required")
	}
	pageURL := "https://api.notion.com/v1/pages/" + pageID
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Notion-Version", notionAPIVersion)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("reading Notion page: HTTP %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	var page struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, err
	}
	markdown, err := readNotionMarkdown(ctx, key, pageID)
	if err != nil {
		return nil, err
	}
	description, err := normalizeNotionDescription(markdown)
	if err != nil {
		return nil, fmt.Errorf("formatting Notion description: %w", err)
	}
	e := &notionEvent{PageID: pageID, Properties: page.Properties, Description: description}
	e.Title = notionTitle(page.Properties["Event name"])
	e.Intro = notionRichText(page.Properties["Short event blurb"])
	if e.Intro == "" {
		e.Intro = e.Title
	}
	e.Start, e.Duration = notionDateDuration(page.Properties["Dates"])
	e.WWWLink = notionURL(page.Properties["WWW Link"])
	e.Format = notionSelect(page.Properties["Format"])
	e.HostIDs = notionNumericPrefixList(page.Properties["Host ID"])
	e.TopicIDs = notionNumericPrefixList(page.Properties["Topics"])
	e.ImageURL = notionFirstFileURL(page.Properties["Image"])
	// Venues is canonical: values use "<Meetup venue ID> - <venue name>".
	// A missing numeric prefix intentionally means this venue does not yet exist in Meetup.
	e.VenueID = notionNumericPrefix(notionSelect(page.Properties["Venues"]))
	if e.Title == "" || e.Start == "" || e.Duration <= 0 || e.Description == "" || e.WWWLink == "" {
		return nil, fmt.Errorf("Notion page is missing required Event name, Dates, description, or WWW Link")
	}
	if e.Format == "" {
		return nil, fmt.Errorf("Notion page is missing Format")
	}
	return e, nil
}

func readNotionMarkdown(ctx context.Context, key, pageID string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.notion.com/v1/pages/"+pageID+"/markdown", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Notion-Version", notionAPIVersion)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("reading Notion markdown: HTTP %d", resp.StatusCode)
	}
	var body struct {
		Markdown string `json:"markdown"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	return body.Markdown, nil
}

// normalizeNotionDescription turns Notion's line-oriented Markdown export into
// predictable Meetup Markdown. Notion text blocks become paragraphs; consecutive
// bullet-list blocks remain a single list.
func normalizeNotionDescription(markdown string) (string, error) {
	text := strings.ReplaceAll(markdown, "\r\n", "\n")
	text = strings.ReplaceAll(text, "<empty-block/>", "")
	text = strings.ReplaceAll(text, `\n`, "\n")
	if strings.Contains(text, "```") {
		return "", fmt.Errorf("code fences are not supported; use native Notion blocks")
	}

	var out []string
	previousKind := ""
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		kind := "paragraph"
		if strings.HasPrefix(line, "#") {
			kind = "heading"
			line = "**" + strings.TrimSpace(strings.TrimLeft(line, "#")) + "**"
		} else if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") || strings.HasPrefix(line, "+ ") {
			kind = "list"
			line = "• " + strings.TrimSpace(line[2:])
		}
		if len(out) > 0 {
			if kind == "list" && previousKind == "list" {
				out = append(out, "\n")
			} else {
				out = append(out, "\n\n")
			}
		}
		out = append(out, line)
		previousKind = kind
	}
	return strings.TrimSpace(strings.Join(out, "")), nil
}

func createMeetupFromNotion(ctx context.Context, gc *gqlClient, e *notionEvent, group string, draft bool) (*gqlEvent, error) {
	description := e.Description + "\n\nRegister [Here](" + e.WWWLink + ")"
	vars := map[string]any{"groupUrlname": group, "title": e.Title, "startDateTime": e.Start, "duration": fmt.Sprintf("PT%dM", e.Duration), "description": description, "publishStatus": "DRAFT", "eventHosts": numericInts(e.HostIDs), "topics": e.TopicIDs}
	if !draft {
		vars["publishStatus"] = "PUBLISHED"
	}
	format, err := meetupFormatForGroup(e.Format, group)
	if err != nil {
		return nil, err
	}
	if format == "online" {
		vars["venueId"] = meetupOnlineVenueID
		vars["howToFindUs"] = e.WWWLink
	}
	if format == "onsite" {
		if e.VenueID == "" {
			return nil, fmt.Errorf("Notion Venues must contain '<Meetup venue ID> - <venue name>' for Onsite events; no Meetup venue exists yet")
		}
		vars["venueId"] = e.VenueID
	}
	var data struct {
		CreateEvent *struct {
			Event  *gqlEvent `json:"event"`
			Errors []struct {
				Message string `json:"message"`
				Code    string `json:"code"`
				Field   string `json:"field"`
			} `json:"errors"`
		} `json:"createEvent"`
	}
	if err := gc.do(ctx, mutationCreateEvent, vars, &data); err != nil {
		return nil, err
	}
	if data.CreateEvent != nil && len(data.CreateEvent.Errors) > 0 {
		return nil, fmt.Errorf("Meetup create error: %s (%s on %s)", data.CreateEvent.Errors[0].Message, data.CreateEvent.Errors[0].Code, data.CreateEvent.Errors[0].Field)
	}
	if data.CreateEvent == nil || data.CreateEvent.Event == nil {
		return nil, fmt.Errorf("Meetup creation returned no event")
	}
	return data.CreateEvent.Event, nil
}

func notionTargetGroups(format, city string) ([]string, error) {
	c := strings.ToLower(strings.TrimSpace(city))
	f := strings.ToLower(strings.TrimSpace(format))
	if c == "auto" || c == "" {
		if f == "online" || f == "hybrid" {
			return []string{"le-wagon-montreal", "le-wagon-toronto"}, nil
		}
		return []string{"le-wagon-montreal"}, nil
	}
	switch c {
	case "montreal":
		return []string{"le-wagon-montreal"}, nil
	case "toronto":
		return []string{"le-wagon-toronto"}, nil
	case "both":
		return []string{"le-wagon-montreal", "le-wagon-toronto"}, nil
	default:
		return nil, fmt.Errorf("unsupported --city %q", city)
	}
}

// meetupFormatForGroup maps Hybrid events to their city-specific delivery
// mode. Both city listings retain the same canonical registration URL.
func meetupFormatForGroup(format, group string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "online", "onsite":
		return strings.ToLower(strings.TrimSpace(format)), nil
	case "hybrid":
		switch group {
		case "le-wagon-montreal":
			return "onsite", nil
		case "le-wagon-toronto":
			return "online", nil
		default:
			return "", fmt.Errorf("Hybrid events support only le-wagon-montreal and le-wagon-toronto")
		}
	default:
		return "", fmt.Errorf("unsupported Notion Format %q", format)
	}
}
func meetupURLPropertyForGroup(group string) string {
	if group == "le-wagon-toronto" {
		return "Meetup URL Toronto"
	}
	return "Meetup URL Montreal"
}
func requireNotionCallbackFields(props map[string]json.RawMessage, groups []string) error {
	for _, g := range groups {
		p := meetupURLPropertyForGroup(g)
		if _, ok := props[p]; !ok {
			return fmt.Errorf("Notion page is missing required callback property %q", p)
		}
	}
	return nil
}
func writeNotionURL(ctx context.Context, pageID, property, value string) error {
	key := os.Getenv("NOTION_API_KEY")
	payload, _ := json.Marshal(map[string]any{"properties": map[string]any{property: map[string]any{"url": value}}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, "https://api.notion.com/v1/pages/"+pageID, strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Notion-Version", notionAPIVersion)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Notion URL callback HTTP %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	return nil
}
func downloadNotionImage(ctx context.Context, rawURL string) (string, error) {
	if rawURL == "" {
		return "", fmt.Errorf("Image is empty")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("image download HTTP %d", resp.StatusCode)
	}
	ext := ".jpg"
	if strings.Contains(resp.Header.Get("Content-Type"), "png") {
		ext = ".png"
	}
	f, err := os.CreateTemp("", "meetup-notion-*"+ext)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err = io.Copy(f, resp.Body); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}
func numericInts(ids []string) []int {
	out := make([]int, 0, len(ids))
	for _, s := range ids {
		var n int
		if _, err := fmt.Sscanf(s, "%d", &n); err == nil {
			out = append(out, n)
		}
	}
	return out
}
func notionTitle(raw json.RawMessage) string {
	var p struct {
		Title []struct {
			PlainText string `json:"plain_text"`
		} `json:"title"`
	}
	_ = json.Unmarshal(raw, &p)
	if len(p.Title) > 0 {
		return p.Title[0].PlainText
	}
	return ""
}
func notionURL(raw json.RawMessage) string {
	var p struct {
		URL string `json:"url"`
	}
	_ = json.Unmarshal(raw, &p)
	return p.URL
}
func notionSelect(raw json.RawMessage) string {
	var p struct {
		Select *struct {
			Name string `json:"name"`
		} `json:"select"`
	}
	_ = json.Unmarshal(raw, &p)
	if p.Select != nil {
		return p.Select.Name
	}
	return ""
}
func notionDateDuration(raw json.RawMessage) (string, int) {
	var p struct {
		Date *struct {
			Start string `json:"start"`
			End   string `json:"end"`
		} `json:"date"`
	}
	_ = json.Unmarshal(raw, &p)
	if p.Date == nil {
		return "", 0
	}
	start, err := time.Parse(time.RFC3339, p.Date.Start)
	if err != nil {
		return "", 0
	}
	end, err := time.Parse(time.RFC3339, p.Date.End)
	if err != nil {
		return "", 0
	}
	return p.Date.Start, int(end.Sub(start).Minutes())
}
func notionNumericPrefixList(raw json.RawMessage) []string {
	var p struct {
		Values []struct {
			Name string `json:"name"`
		} `json:"multi_select"`
	}
	_ = json.Unmarshal(raw, &p)
	re := regexp.MustCompile(`^\s*(\d+)\s*(?:-|$)`)
	out := []string{}
	for _, v := range p.Values {
		m := re.FindStringSubmatch(v.Name)
		if len(m) != 2 {
			continue
		}
		out = append(out, m[1])
	}
	return out
}
func notionNumericPrefix(value string) string {
	m := regexp.MustCompile(`^\s*(\d+)\s*-`).FindStringSubmatch(value)
	if len(m) == 2 {
		return m[1]
	}
	return ""
}
func notionRichText(raw json.RawMessage) string {
	var p struct {
		RichText []struct {
			PlainText string `json:"plain_text"`
		} `json:"rich_text"`
	}
	_ = json.Unmarshal(raw, &p)
	if len(p.RichText) == 0 {
		return ""
	}
	return strings.TrimSpace(p.RichText[0].PlainText)
}
func notionFirstFileURL(raw json.RawMessage) string {
	var p struct {
		Files []struct {
			Type string `json:"type"`
			File *struct {
				URL string `json:"url"`
			} `json:"file"`
			External *struct {
				URL string `json:"url"`
			} `json:"external"`
		} `json:"files"`
	}
	_ = json.Unmarshal(raw, &p)
	if len(p.Files) == 0 {
		return ""
	}
	if p.Files[0].File != nil {
		return p.Files[0].File.URL
	}
	if p.Files[0].External != nil {
		return p.Files[0].External.URL
	}
	return ""
}
