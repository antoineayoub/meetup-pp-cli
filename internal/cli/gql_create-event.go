// Copyright 2026 Ayoub and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

const meetupOnlineVenueID = "26906060"

const mutationCreateEvent = `
mutation CreateEvent(
  $groupUrlname: String!
  $title: String!
  $startDateTime: String!
  $duration: String
  $description: String!
  $publishStatus: PublishStatus
  $venueId: String
  $howToFindUs: String
  $eventHosts: [Int!]
  $topics: [ID!]
) {
  createEvent(input: {
    groupUrlname: $groupUrlname
    title: $title
    startDateTime: $startDateTime
    duration: $duration
    description: $description
    publishStatus: $publishStatus
    venueId: $venueId
    howToFindUs: $howToFindUs
    eventHosts: $eventHosts
    topics: $topics
  }) {
    event {
      id title dateTime endTime status eventUrl eventType howToFindUs
      venue { id name }
      venues { id name }
    }
    errors { message code field }
  }
}`

const mutationCreateGroupEventPhoto = `
mutation CreateGroupEventPhoto($input: GroupEventPhotoCreateInput!) {
  createGroupEventPhoto(input: $input) {
    uploadUrl
    photo { id baseUrl }
    error { message }
  }
}`

func newNovelEventsCreateCmd(flags *rootFlags) *cobra.Command {
	var flagGroup string
	var flagTitle string
	var flagDate string
	var flagDuration int
	var flagDescription string
	var flagDraft bool
	var flagVenueID string
	var flagPhoto string
	var flagHowToFindUs string
	var flagNotionFormat string
	var flagHostIDs []string
	var flagTopicIDs []string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new event in a Meetup group from the terminal.",
		Long: `Create a new event in a Meetup group. Requires organizer scope — run 'auth login' first.

Use --draft to save as a draft instead of publishing immediately.
Use --photo to attach a cover image (JPEG, PNG, or GIF).
Returns the event ID and URL.`,
		Example: `  meetup-pp-cli events create --group le-wagon-montreal \
    --title "Demo Day" --date 2026-06-15T18:00 --duration 120 \
    --description "Monthly demo night" --photo ./cover.jpg --draft --json`,
		Annotations: map[string]string{"mcp:read-only": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			gc := newGQLClient()
			if gc.token == "" {
				return fmt.Errorf("events create requires authentication — run: meetup-pp-cli auth login")
			}

			// Fail fast on missing photo file before touching the API.
			if flagPhoto != "" {
				if _, err := os.Stat(flagPhoto); err != nil {
					return fmt.Errorf("photo file not found: %s", flagPhoto)
				}
				if _, err := photoContentType(flagPhoto); err != nil {
					return err
				}
			}

			publishStatus := "PUBLISHED"
			if flagDraft {
				publishStatus = "DRAFT"
			}

			resolvedVenueID := strings.TrimSpace(flagVenueID)
			resolvedHowToFindUs := strings.TrimSpace(flagHowToFindUs)
			switch strings.ToLower(strings.TrimSpace(flagNotionFormat)) {
			case "", "onsite", "in-person", "in person", "physical":
				// no override
			case "online":
				if resolvedVenueID == "" {
					resolvedVenueID = meetupOnlineVenueID
				}
				if resolvedHowToFindUs == "" {
					return usageErr(fmt.Errorf("--how-to-find-us is required when --notion-format=online"))
				}
			case "hybrid":
				switch flagGroup {
				case "le-wagon-montreal":
					if resolvedVenueID == "" {
						return usageErr(fmt.Errorf("--venue-id is required for a Hybrid event in Montréal"))
					}
				case "le-wagon-toronto":
					if resolvedVenueID == "" {
						resolvedVenueID = meetupOnlineVenueID
					}
					if resolvedHowToFindUs == "" {
						return usageErr(fmt.Errorf("--how-to-find-us is required for a Hybrid event in Toronto"))
					}
				default:
					return usageErr(fmt.Errorf("--notion-format=hybrid supports le-wagon-montreal (onsite) or le-wagon-toronto (online)"))
				}
			default:
				return usageErr(fmt.Errorf("unsupported --notion-format %q; use online, onsite, or hybrid", flagNotionFormat))
			}

			vars := map[string]any{
				"groupUrlname":  flagGroup,
				"title":         flagTitle,
				"startDateTime": flagDate,
				"duration":      fmt.Sprintf("PT%dM", flagDuration),
				"description":   flagDescription,
				"publishStatus": publishStatus,
			}
			if resolvedVenueID != "" {
				vars["venueId"] = resolvedVenueID
			}
			if resolvedHowToFindUs != "" {
				vars["howToFindUs"] = resolvedHowToFindUs
			}
			if len(flagHostIDs) > 0 {
				hosts := make([]int, 0, len(flagHostIDs))
				for _, id := range flagHostIDs {
					trimmed := strings.TrimSpace(id)
					if trimmed == "" {
						return usageErr(fmt.Errorf("--host-id cannot be empty"))
					}
					var parsed int
					if _, err := fmt.Sscanf(trimmed, "%d", &parsed); err != nil {
						return usageErr(fmt.Errorf("--host-id must be numeric: %q", id))
					}
					hosts = append(hosts, parsed)
				}
				vars["eventHosts"] = hosts
			}
			if len(flagTopicIDs) > 0 {
				topics := make([]string, 0, len(flagTopicIDs))
				for _, id := range flagTopicIDs {
					trimmed := strings.TrimSpace(id)
					if trimmed == "" {
						return usageErr(fmt.Errorf("--topic-id cannot be empty"))
					}
					topics = append(topics, trimmed)
				}
				vars["topics"] = topics
			}

			var data struct {
				CreateEvent *struct {
					Event *gqlEvent `json:"event"`
				} `json:"createEvent"`
			}
			if err := gc.do(cmd.Context(), mutationCreateEvent, vars, &data); err != nil {
				return err
			}
			if data.CreateEvent == nil || data.CreateEvent.Event == nil {
				return fmt.Errorf("event creation returned no event data")
			}
			ev := data.CreateEvent.Event

			// Upload photo after event creation (API requires eventId to exist).
			if flagPhoto != "" {
				if err := uploadEventPhoto(cmd.Context(), gc, flagGroup, ev.ID, flagPhoto); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "Warning: event created but photo upload failed: %v\n", err)
				}
			}

			if !flags.asJSON {
				fmt.Fprintf(cmd.OutOrStdout(), "Created event: %s\nURL: %s\nStatus: %s\n",
					ev.ID, ev.EventURL, ev.Status)
				return nil
			}
			return flags.printJSON(cmd, ev)
		},
	}
	cmd.Flags().StringVar(&flagGroup, "group", "", "Group URL slug (required)")
	cmd.Flags().StringVar(&flagTitle, "title", "", "Event title (required)")
	cmd.Flags().StringVar(&flagDate, "date", "", "Start date/time ISO 8601, e.g. 2026-06-15T18:00 (required)")
	cmd.Flags().IntVar(&flagDuration, "duration", 60, "Duration in minutes")
	cmd.Flags().StringVar(&flagDescription, "description", "", "Event description (required)")
	cmd.Flags().BoolVar(&flagDraft, "draft", false, "Save as draft instead of publishing immediately")
	cmd.Flags().StringVar(&flagVenueID, "venue-id", "", "Meetup venue ID to attach to the event")
	cmd.Flags().StringVar(&flagHowToFindUs, "how-to-find-us", "", "Event access or joining instructions; for online events this can hold the event link")
	cmd.Flags().StringVar(&flagNotionFormat, "notion-format", "", "Format from Notion: online, onsite, or hybrid. Hybrid maps Montréal to onsite and Toronto to online")
	cmd.Flags().StringSliceVar(&flagHostIDs, "host-id", nil, "Eligible Meetup host ID (repeatable; replaces hosts when supplied at creation)")
	cmd.Flags().StringSliceVar(&flagTopicIDs, "topic-id", nil, "Meetup topic ID (repeatable; replaces topics when supplied at creation)")
	cmd.Flags().StringVar(&flagPhoto, "photo", "", "Path to cover image file (JPEG, PNG, or GIF)")
	_ = cmd.MarkFlagRequired("group")
	_ = cmd.MarkFlagRequired("title")
	_ = cmd.MarkFlagRequired("date")
	_ = cmd.MarkFlagRequired("description")
	return cmd
}

// uploadEventPhoto uploads a local image and attaches it as the featured photo for eventID.
// Flow: fetch group ID → createGroupEventPhoto (get pre-signed URL) → PUT image bytes → attach featuredPhotoId.
// PATCH: normalize non-JPEG inputs to JPEG first because Meetup draft rendering proved unreliable with
// PNG/WebP-origin flows even when the API returned a photo id.
func uploadEventPhoto(ctx context.Context, gc *gqlClient, groupUrlname, eventID, filePath string) error {
	normalizedPath, contentType, err := normalizePhotoForMeetup(ctx, filePath)
	if err != nil {
		return err
	}
	imgData, err := os.ReadFile(normalizedPath)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}

	// Fetch group ID (required by createGroupEventPhoto).
	var groupData struct {
		GroupByUrlname *struct {
			ID string `json:"id"`
		} `json:"groupByUrlname"`
	}
	if err := gc.do(ctx, queryGetGroup, map[string]any{"urlname": groupUrlname}, &groupData); err != nil {
		return fmt.Errorf("fetching group: %w", err)
	}
	if groupData.GroupByUrlname == nil {
		return fmt.Errorf("group not found: %s", groupUrlname)
	}

	// Request pre-signed upload URL.
	var photoResp struct {
		CreateGroupEventPhoto *struct {
			UploadURL string `json:"uploadUrl"`
			Photo     *struct {
				ID string `json:"id"`
			} `json:"photo"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		} `json:"createGroupEventPhoto"`
	}
	input := map[string]any{
		"groupId":     groupData.GroupByUrlname.ID,
		"eventId":     eventID,
		"contentType": contentType,
		"photoType":   "EVENT_PHOTO",
		"setAsMain":   true,
	}
	if err := gc.do(ctx, mutationCreateGroupEventPhoto, map[string]any{"input": input}, &photoResp); err != nil {
		return err
	}
	p := photoResp.CreateGroupEventPhoto
	if p == nil {
		return fmt.Errorf("no response from createGroupEventPhoto")
	}
	if p.Error != nil {
		return fmt.Errorf("createGroupEventPhoto: %s", p.Error.Message)
	}
	if p.UploadURL == "" {
		return fmt.Errorf("no upload URL returned")
	}
	if p.Photo == nil || p.Photo.ID == "" {
		return fmt.Errorf("no photo id returned")
	}

	mimeType := meetupPhotoMimeType(contentType)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, p.UploadURL, bytes.NewReader(imgData))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mimeType)
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(imgData)))
	resp, err := gc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("uploading image: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("upload failed: HTTP %d", resp.StatusCode)
	}

	const mutationEditEventFeaturedPhoto = `
mutation EditEventFeaturedPhoto($input: EditEventInput!) {
  editEvent(input: $input) {
    event {
      id
      featuredEventPhoto { id }
    }
    errors { message code field }
  }
}`
	var attachResp struct {
		EditEvent *struct {
			Event *struct {
				ID                 string `json:"id"`
				FeaturedEventPhoto *struct {
					ID string `json:"id"`
				} `json:"featuredEventPhoto"`
			} `json:"event"`
			Errors []struct {
				Message string `json:"message"`
				Code    string `json:"code"`
				Field   string `json:"field"`
			} `json:"errors"`
		} `json:"editEvent"`
	}
	if err := gc.do(ctx, mutationEditEventFeaturedPhoto, map[string]any{"input": map[string]any{
		"eventId":         eventID,
		"featuredPhotoId": atoiOrZero(p.Photo.ID),
	}}, &attachResp); err != nil {
		return fmt.Errorf("attaching featured photo: %w", err)
	}
	if attachResp.EditEvent == nil || attachResp.EditEvent.Event == nil || attachResp.EditEvent.Event.FeaturedEventPhoto == nil || attachResp.EditEvent.Event.FeaturedEventPhoto.ID == "" {
		return fmt.Errorf("featured photo was not attached")
	}
	return nil
}

func photoContentType(filePath string) (string, error) {
	switch strings.ToLower(filepath.Ext(filePath)) {
	case ".jpg", ".jpeg":
		return "JPEG", nil
	case ".png":
		return "PNG", nil
	case ".gif":
		return "GIF", nil
	case ".webp":
		return "WEBP", nil
	default:
		return "", fmt.Errorf("unsupported image format %q — use JPEG, PNG, GIF, or WEBP", filepath.Ext(filePath))
	}
}

func meetupPhotoMimeType(contentType string) string {
	switch strings.ToUpper(contentType) {
	case "JPEG":
		return "image/jpeg"
	case "PNG":
		return "image/png"
	case "GIF":
		return "image/gif"
	default:
		return "application/octet-stream"
	}
}

func normalizePhotoForMeetup(ctx context.Context, filePath string) (string, string, error) {
	contentType, err := photoContentType(filePath)
	if err != nil {
		return "", "", err
	}
	if contentType == "JPEG" {
		return filePath, "JPEG", nil
	}
	outPath := strings.TrimSuffix(filePath, filepath.Ext(filePath)) + ".jpg"
	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", filePath, "-frames:v", "1", "-q:v", "2", outPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", "", fmt.Errorf("converting %s to jpeg: %w: %s", filePath, err, strings.TrimSpace(string(output)))
	}
	if _, err := os.Stat(outPath); err != nil {
		return "", "", fmt.Errorf("converted jpeg not found: %w", err)
	}
	return outPath, "JPEG", nil
}

func atoiOrZero(s string) int {
	var n int
	_, _ = fmt.Sscanf(s, "%d", &n)
	return n
}
