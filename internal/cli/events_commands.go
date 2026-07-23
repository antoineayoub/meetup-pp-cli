package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func newNovelEventsUpdateCmd(flags *rootFlags) *cobra.Command {
	var eventID, group, title, description, date, venueID, location, photo string
	var duration int
	var topicIDs, hostIDs []string
	cmd := &cobra.Command{Use: "update", Short: "Update only fields explicitly provided for an existing Meetup event.", Annotations: map[string]string{"mcp:read-only": "false"}, RunE: func(cmd *cobra.Command, _ []string) error {
		if dryRunOK(flags) {
			return nil
		}
		if eventID == "" {
			return usageErr(fmt.Errorf("--event-id is required"))
		}
		if photo != "" {
			if group == "" {
				return usageErr(fmt.Errorf("--group is required with --photo"))
			}
			if _, err := os.Stat(photo); err != nil {
				return fmt.Errorf("photo file not found: %s", photo)
			}
			if _, err := photoContentType(photo); err != nil {
				return err
			}
		}
		if len(hostIDs) > 0 && group == "" {
			return usageErr(fmt.Errorf("--group is required with --host-id"))
		}
		in := editEventInput{EventID: eventID, TopicIDs: topicIDs, HostIDs: hostIDs}
		if cmd.Flags().Changed("title") {
			in.Title = &title
		}
		if cmd.Flags().Changed("description") {
			in.Description = &description
		}
		if cmd.Flags().Changed("date") {
			in.StartDateTime = &date
		}
		if cmd.Flags().Changed("duration") {
			in.DurationMins = &duration
		}
		if cmd.Flags().Changed("venue-id") {
			in.VenueID = &venueID
		}
		if cmd.Flags().Changed("location") {
			if strings.TrimSpace(location) == "" {
				return usageErr(fmt.Errorf("--location cannot be empty"))
			}
			in.Location = &location
		}
		if in.VenueID != nil && in.Location != nil {
			return usageErr(fmt.Errorf("--venue-id and --location cannot be combined"))
		}
		vars, err := buildEditEventVariables(in)
		if err != nil {
			return err
		}
		gc := newGQLClient()
		if len(hostIDs) > 0 {
			hosts, err := listEventHosts(cmd.Context(), gc, group)
			if err != nil {
				return fmt.Errorf("validating event hosts: %w", err)
			}
			valid := map[string]bool{}
			for _, h := range hosts {
				valid[h.ID] = h.Eligible
			}
			for _, id := range hostIDs {
				if !valid[id] {
					return fmt.Errorf("host %s is not eligible for group %s", id, group)
				}
			}
		}
		var data struct {
			EditEvent *struct {
				Event *gqlEvent `json:"event"`
			} `json:"editEvent"`
		}
		if err := gc.do(cmd.Context(), mutationEditEvent, vars, &data); err != nil {
			return err
		}
		if data.EditEvent == nil || data.EditEvent.Event == nil {
			return fmt.Errorf("event update returned no event data")
		}
		if photo != "" {
			if err := uploadEventPhoto(cmd.Context(), gc, group, eventID, photo); err != nil {
				return fmt.Errorf("event updated but photo upload failed: %w", err)
			}
		}
		return flags.printJSON(cmd, data.EditEvent.Event)
	}}
	cmd.Flags().StringVar(&eventID, "event-id", "", "Meetup event ID (required)")
	cmd.Flags().StringVar(&group, "group", "", "Group URL slug (required with --photo or --host-id)")
	cmd.Flags().StringVar(&title, "title", "", "Updated title")
	cmd.Flags().StringVar(&description, "description", "", "Updated description")
	cmd.Flags().StringVar(&date, "date", "", "Updated ISO 8601 start date/time")
	cmd.Flags().IntVar(&duration, "duration", 0, "Updated duration in minutes")
	cmd.Flags().StringVar(&venueID, "venue-id", "", "Existing Meetup venue ID")
	cmd.Flags().StringVar(&location, "location", "", "Location name; cannot be combined with --venue-id")
	cmd.Flags().StringSliceVar(&topicIDs, "topic-id", nil, "Meetup topic ID (repeatable; replaces topics when supplied)")
	cmd.Flags().StringSliceVar(&hostIDs, "host-id", nil, "Eligible Meetup host ID (repeatable; replaces hosts when supplied)")
	cmd.Flags().StringVar(&photo, "photo", "", "JPEG, PNG, or GIF cover image")
	return cmd
}

func newNovelEventsPublishCmd(flags *rootFlags) *cobra.Command {
	var eventID, group string
	cmd := &cobra.Command{Use: "publish", Short: "Explicitly publish a Meetup draft after confirmation.", Annotations: map[string]string{"mcp:read-only": "false"}, RunE: func(cmd *cobra.Command, _ []string) error {
		if dryRunOK(flags) {
			return nil
		}
		if eventID == "" || group == "" {
			return usageErr(fmt.Errorf("--event-id and --group are required"))
		}
		if flags.agent || flags.noInput {
			if !cmd.Flags().Changed("yes") {
				return fmt.Errorf("events publish requires --yes in non-interactive or agent mode")
			}
		} else if !flags.yes {
			fmt.Fprintf(cmd.OutOrStdout(), "Publish Meetup event %s? [y/N]: ", eventID)
			answer, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
			answer = strings.ToLower(strings.TrimSpace(answer))
			if answer != "y" && answer != "yes" {
				return fmt.Errorf("publication cancelled")
			}
		}
		ev, changed, err := publishEventDraft(cmd.Context(), newGQLClient(), group, eventID)
		if err != nil {
			return err
		}
		if !changed {
			fmt.Fprintln(cmd.ErrOrStderr(), "event already ACTIVE; no publish mutation sent")
		}
		return flags.printJSON(cmd, ev)
	}}
	cmd.Flags().StringVar(&eventID, "event-id", "", "Meetup event ID (required)")
	cmd.Flags().StringVar(&group, "group", "", "Group URL slug (required)")
	return cmd
}

func newNovelEventsHostsCmd(flags *rootFlags) *cobra.Command {
	parent := &cobra.Command{Use: "hosts", Short: "Discover eligible Meetup event hosts.", RunE: parentNoSubcommandRunE(flags)}
	var group string
	list := &cobra.Command{Use: "list", Short: "List hosts eligible for a group.", Annotations: map[string]string{"mcp:read-only": "true"}, RunE: func(cmd *cobra.Command, _ []string) error {
		if dryRunOK(flags) {
			return nil
		}
		if group == "" {
			return usageErr(fmt.Errorf("--group is required"))
		}
		hosts, err := listEventHosts(cmd.Context(), newGQLClient(), group)
		if err != nil {
			return err
		}
		return flags.printJSON(cmd, hosts)
	}}
	list.Flags().StringVar(&group, "group", "", "Group URL slug (required)")
	parent.AddCommand(list)
	return parent
}
