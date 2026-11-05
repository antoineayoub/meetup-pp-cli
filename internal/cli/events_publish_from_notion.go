package cli

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

type notionPublishResult struct {
	Group string    `json:"group"`
	Event *gqlEvent `json:"event"`
}

// newNovelEventsPublishFromNotionCmd publishes only existing verified drafts.
// It deliberately does not update event copy, schedules, images, hosts, topics, or venues.
func newNovelEventsPublishFromNotionCmd(flags *rootFlags) *cobra.Command {
	var pageID, city string
	var yes bool

	cmd := &cobra.Command{
		Use:         "publish-from-notion",
		Short:       "Explicitly publish existing verified Meetup drafts from a canonical Notion event.",
		Annotations: map[string]string{"mcp:read-only": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(pageID) == "" {
				return usageErr(fmt.Errorf("--page-id is required"))
			}
			if (flags.agent || flags.noInput) && !yes {
				return fmt.Errorf("events publish-from-notion requires --yes in agent or non-interactive mode")
			}
			if !yes {
				return fmt.Errorf("events publish-from-notion requires explicit --yes")
			}

			event, err := loadNotionEvent(cmd.Context(), pageID)
			if err != nil {
				return err
			}
			if err := requireFinalAbsoluteHTTPSURL(event.WWWLink); err != nil {
				return err
			}
			groups, err := notionTargetGroups(event.Format, city)
			if err != nil {
				return usageErr(err)
			}
			if err := requireNotionCallbackFields(event.Properties, groups); err != nil {
				return err
			}

			targets := make(map[string]string, len(groups))
			for _, group := range groups {
				property := meetupURLPropertyForGroup(group)
				id := meetupID(notionURL(event.Properties[property]))
				if id == "" {
					return fmt.Errorf("%s is empty or invalid; publication requires an existing verified %s draft", property, group)
				}
				targets[group] = id
			}
			if flags.dryRun {
				return flags.printJSON(cmd, map[string]any{
					"page_id":   pageID,
					"operation": "publish-existing-drafts",
					"targets":   targets,
				})
			}

			gc := newGQLClient()
			if gc.token == "" {
				return fmt.Errorf("Meetup authentication required")
			}

			// Preflight every city before publishing either one.
			for _, group := range groups {
				draft, err := findGroupEventByID(cmd.Context(), gc, group, targets[group])
				if err != nil {
					return err
				}
				if draft == nil {
					return fmt.Errorf("Meetup event %s was not found in %s", targets[group], group)
				}
				if draft.Status != "DRAFT" {
					return fmt.Errorf("Meetup event %s in %s is %s, not DRAFT", targets[group], group, draft.Status)
				}
			}

			results := make([]notionPublishResult, 0, len(groups))
			for _, group := range groups {
				published, _, err := publishEventDraft(cmd.Context(), gc, group, targets[group])
				if err != nil {
					return fmt.Errorf("publishing %s: %w", group, err)
				}
				results = append(results, notionPublishResult{Group: group, Event: published})
			}

			if err := writeNotionSyncState(context.Background(), pageID, true); err != nil {
				return fmt.Errorf("events were published but Notion On meetup ? could not be updated: %w", err)
			}
			return flags.printJSON(cmd, results)
		},
	}
	cmd.Flags().StringVar(&pageID, "page-id", "", "Canonical Notion event page ID (required)")
	cmd.Flags().StringVar(&city, "city", "auto", "Target: auto, montreal, toronto, or both")
	cmd.Flags().BoolVar(&yes, "yes", false, "Explicitly approve publication")
	_ = cmd.MarkFlagRequired("page-id")
	return cmd
}

func requireFinalAbsoluteHTTPSURL(raw string) error {
	if strings.ContainsAny(raw, "<>") || strings.Contains(strings.ToLower(raw), "slug-de-l-evenement") {
		return fmt.Errorf("WWW Link contains an unresolved placeholder")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return fmt.Errorf("WWW Link must be a final absolute HTTPS URL")
	}
	return nil
}
