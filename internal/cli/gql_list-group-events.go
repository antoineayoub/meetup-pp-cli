// Copyright 2026 Ayoub and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"github.com/spf13/cobra"
)

func newGqlListGroupEventsCmd(flags *rootFlags) *cobra.Command {
	var flagStatus string
	var flagFirst int

	cmd := &cobra.Command{
		Use:         "list-group-events <urlname>",
		Short:       "List upcoming, past, draft, or cancelled events for a specific Meetup group.",
		Example:     "  meetup-pp-cli gql list-group-events le-wagon-montreal --status ACTIVE",
		Annotations: map[string]string{"pp:endpoint": "gql.list-group-events", "mcp:read-only": "true"},
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			gc := newGQLClient()
			events, err := fetchAllGroupEvents(cmd.Context(), gc, args[0], flagStatus, flagFirst)
			if err != nil {
				return err
			}
			return flags.printJSON(cmd, events)
		},
	}
	cmd.Flags().StringVar(&flagStatus, "status", "ACTIVE", "Event status filter (ACTIVE, PAST, DRAFT, CANCELLED)")
	cmd.Flags().IntVar(&flagFirst, "first", 20, "Max events to return")
	return cmd
}
