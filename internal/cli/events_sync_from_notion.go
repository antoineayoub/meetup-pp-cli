package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

func newNovelEventsSyncFromNotionCmd(flags *rootFlags) *cobra.Command {
	var pageID, city string
	var publish bool
	cmd := &cobra.Command{Use: "sync-from-notion", Short: "Create or update Meetup event(s) from a canonical Notion page.", Annotations: map[string]string{"mcp:read-only": "false"}, RunE: func(cmd *cobra.Command, _ []string) error {
		if pageID == "" {
			return usageErr(fmt.Errorf("--page-id is required"))
		}
		e, err := loadNotionEvent(cmd.Context(), pageID)
		if err != nil {
			return err
		}
		groups, err := notionTargetGroups(e.Format, city)
		if err != nil {
			return usageErr(err)
		}
		if err = requireNotionCallbackFields(e.Properties, groups); err != nil {
			return err
		}
		if flags.dryRun {
			return flags.printJSON(cmd, map[string]any{"page_id": pageID, "groups": groups, "mode": "create-or-update", "publish": publish})
		}
		photo, err := downloadNotionImage(cmd.Context(), e.ImageURL)
		if err != nil {
			return err
		}
		defer os.Remove(photo)
		gc := newGQLClient()
		if gc.token == "" {
			return fmt.Errorf("Meetup authentication required")
		}
		out := []notionCreateResult{}
		for _, group := range groups {
			prop := meetupURLPropertyForGroup(group)
			id := meetupID(notionURL(e.Properties[prop]))
			var ev *gqlEvent
			if id == "" {
				ev, err = createMeetupFromNotion(cmd.Context(), gc, e, group, !publish)
			} else {
				ev, err = syncUpdate(ctxOr(cmd.Context()), gc, e, group, id)
			}
			if err != nil {
				return fmt.Errorf("syncing %s: %w", group, err)
			}
			if err = uploadEventPhoto(cmd.Context(), gc, group, ev.ID, photo); err != nil {
				return err
			}
			if err = writeNotionURL(cmd.Context(), pageID, prop, ev.EventURL); err != nil {
				return err
			}
			if publish && ev.Status != "ACTIVE" {
				ev, _, err = publishEventDraft(cmd.Context(), gc, group, ev.ID)
				if err != nil {
					return err
				}
			}
			if err = writeNotionSyncState(cmd.Context(), pageID, ev.Status == "ACTIVE"); err != nil {
				return err
			}
			out = append(out, notionCreateResult{Group: group, Event: ev, PhotoOK: true, Callback: true})
		}
		return flags.printJSON(cmd, out)
	}}
	cmd.Flags().StringVar(&pageID, "page-id", "", "Canonical Notion event page ID (required)")
	cmd.Flags().StringVar(&city, "city", "auto", "Target: auto, montreal, toronto, or both")
	cmd.Flags().BoolVar(&publish, "publish", false, "Publish after sync (requires explicit approval)")
	_ = cmd.MarkFlagRequired("page-id")
	return cmd
}
func ctxOr(ctx context.Context) context.Context { return ctx }
func meetupID(s string) string {
	m := regexp.MustCompile(`/events/(\d+)`).FindStringSubmatch(s)
	if len(m) == 2 {
		return m[1]
	}
	return ""
}
func syncUpdate(ctx context.Context, gc *gqlClient, e *notionEvent, group, id string) (*gqlEvent, error) {
	d := e.Description + "\n\nRegister [Here](" + e.WWWLink + ")"
	in := editEventInput{EventID: id, Title: &e.Title, Description: &d, StartDateTime: &e.Start, DurationMins: &e.Duration, TopicIDs: e.TopicIDs, HostIDs: e.HostIDs}
	format, err := meetupFormatForGroup(e.Format, group)
	if err != nil {
		return nil, err
	}
	if format == "online" {
		v := meetupOnlineVenueID
		h := e.WWWLink
		in.VenueID = &v
		in.HowToFindUs = &h
	} else if format == "onsite" {
		if e.VenueID == "" {
			return nil, fmt.Errorf("Venues needs a Meetup ID")
		}
		in.VenueID = &e.VenueID
	}
	vars, err := buildEditEventVariables(in)
	if err != nil {
		return nil, err
	}
	var data struct {
		EditEvent *struct {
			Event  *gqlEvent `json:"event"`
			Errors []struct {
				Message string `json:"message"`
				Code    string `json:"code"`
				Field   string `json:"field"`
			} `json:"errors"`
		} `json:"editEvent"`
	}
	if err = gc.do(ctx, mutationEditEvent, vars, &data); err != nil {
		return nil, err
	}
	if data.EditEvent != nil && len(data.EditEvent.Errors) > 0 {
		first := data.EditEvent.Errors[0]
		if first.Code != "" || first.Field != "" {
			return nil, fmt.Errorf("Meetup update: %s (code=%s field=%s)", first.Message, first.Code, first.Field)
		}
		return nil, fmt.Errorf("Meetup update: %s", first.Message)
	}
	if data.EditEvent == nil || data.EditEvent.Event == nil {
		return nil, fmt.Errorf("Meetup update returned no event")
	}
	return data.EditEvent.Event, nil
}
func writeNotionSyncState(ctx context.Context, page string, active bool) error {
	// Published? and Published at describe the Le Wagon website lifecycle.
	// Meetup publication is tracked independently by On meetup ?.
	p := map[string]any{"On meetup ?": map[string]any{"checkbox": active}}
	b, _ := json.Marshal(map[string]any{"properties": p})
	r, e := http.NewRequestWithContext(ctx, http.MethodPatch, "https://api.notion.com/v1/pages/"+page, strings.NewReader(string(b)))
	if e != nil {
		return e
	}
	r.Header.Set("Authorization", "Bearer "+os.Getenv("NOTION_API_KEY"))
	r.Header.Set("Notion-Version", notionAPIVersion)
	r.Header.Set("Content-Type", "application/json")
	resp, e := http.DefaultClient.Do(r)
	if e != nil {
		return e
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("Notion state callback HTTP %d", resp.StatusCode)
	}
	return nil
}
