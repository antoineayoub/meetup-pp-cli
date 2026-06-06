// Copyright 2026 Ayoub and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newGqlGetGroupCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "get-group <urlname>",
		Short:       "Fetch a Meetup group by its URL slug (e.g. 'le-wagon-montreal').",
		Example:     "  meetup-pp-cli gql get-group le-wagon-montreal",
		Annotations: map[string]string{"pp:endpoint": "gql.get-group", "mcp:read-only": "true"},
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			gc := newGQLClient()
			var data struct {
				GroupByUrlname *gqlGroup `json:"groupByUrlname"`
			}
			if err := gc.do(cmd.Context(), queryGetGroup, map[string]any{"urlname": args[0]}, &data); err != nil {
				return err
			}
			if data.GroupByUrlname == nil {
				return fmt.Errorf("group not found: %s", args[0])
			}
			return flags.printJSON(cmd, data.GroupByUrlname)
		},
	}
	return cmd
}
