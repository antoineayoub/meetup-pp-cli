// Copyright 2026 Ayoub and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newGqlGetEventCmd(flags *rootFlags) *cobra.Command {
	var flagUrlname string

	cmd := &cobra.Command{
		Use:         "get-event <eventId>",
		Short:       "Fetch details for a single event by ID and group URL name.",
		Example:     "  meetup-pp-cli gql get-event 12345678 --urlname le-wagon-montreal",
		Annotations: map[string]string{"pp:endpoint": "gql.get-event", "mcp:read-only": "true"},
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if flagUrlname == "" {
				return fmt.Errorf("--urlname is required")
			}
			gc := newGQLClient()
			// Try ACTIVE events first, then PAST
			for _, status := range []string{"ACTIVE", "PAST"} {
				events, err := fetchAllGroupEvents(cmd.Context(), gc, flagUrlname, status, 200)
				if err != nil {
					return err
				}
				for _, e := range events {
					if e.ID == args[0] {
						return flags.printJSON(cmd, e)
					}
				}
			}
			return fmt.Errorf("event not found: %s", args[0])
		},
	}
	cmd.Flags().StringVar(&flagUrlname, "urlname", "", "Group URL slug that owns the event (required)")
	_ = cmd.MarkFlagRequired("urlname")
	return cmd
}
