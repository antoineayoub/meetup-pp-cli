// Copyright 2026 Ayoub and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

const mutationCreateEvent = `
mutation CreateEvent(
  $groupUrlname: String!
  $title: String!
  $startDateTime: String!
  $duration: String
  $description: String!
  $publishStatus: PublishStatus
  $venueId: String
) {
  createEvent(input: {
    groupUrlname: $groupUrlname
    title: $title
    startDateTime: $startDateTime
    duration: $duration
    description: $description
    publishStatus: $publishStatus
    venueId: $venueId
  }) {
    event {
      id title dateTime endTime status eventUrl
    }
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

			vars := map[string]any{
				"groupUrlname":  flagGroup,
				"title":         flagTitle,
				"startDateTime": flagDate,
				"duration":      fmt.Sprintf("PT%dM", flagDuration),
				"description":   flagDescription,
				"publishStatus": publishStatus,
			}
			if flagVenueID != "" {
				vars["venueId"] = flagVenueID
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
	cmd.Flags().StringVar(&flagPhoto, "photo", "", "Path to cover image file (JPEG, PNG, or GIF)")
	_ = cmd.MarkFlagRequired("group")
	_ = cmd.MarkFlagRequired("title")
	_ = cmd.MarkFlagRequired("date")
	_ = cmd.MarkFlagRequired("description")
	return cmd
}

// uploadEventPhoto uploads a local image and attaches it as the featured photo for eventID.
// Flow: fetch group ID → createGroupEventPhoto (get pre-signed URL) → PUT image bytes.
func uploadEventPhoto(ctx context.Context, gc *gqlClient, groupUrlname, eventID, filePath string) error {
	imgData, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}

	contentType, err := photoContentType(filePath)
	if err != nil {
		return err
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

	// PUT the image to the pre-signed URL.
	var mimeType string
	switch strings.ToLower(contentType) {
	case "jpeg":
		mimeType = "image/jpeg"
	case "png":
		mimeType = "image/png"
	case "gif":
		mimeType = "image/gif"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, p.UploadURL, bytes.NewReader(imgData))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mimeType)
	resp, err := gc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("uploading image: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("upload failed: HTTP %d", resp.StatusCode)
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
	default:
		return "", fmt.Errorf("unsupported image format %q — use JPEG, PNG, or GIF", filepath.Ext(filePath))
	}
}
