// Copyright 2026 Ayoub and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"github.com/spf13/cobra"
)

func newGqlSearchGroupsCmd(flags *rootFlags) *cobra.Command {
	var flagTopic string
	var flagLat float64
	var flagLon float64
	var flagRadius int
	var flagFirst int

	cmd := &cobra.Command{
		Use:         "search-groups",
		Short:       "Find Meetup groups matching a topic keyword near a location.",
		Example:     "  meetup-pp-cli gql search-groups --topic golang --lat 45.5 --lon -73.6",
		Annotations: map[string]string{"pp:endpoint": "gql.search-groups", "mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			gc := newGQLClient()
			vars := map[string]any{
				"first": flagFirst,
				"topic": flagTopic,
				"lat":   flagLat,
				"lon":   flagLon,
			}
			if flagRadius > 0 {
				vars["radius"] = float64(flagRadius)
			}
			var data struct {
				GroupSearch gqlGroupPage `json:"groupSearch"`
			}
			if err := gc.do(cmd.Context(), querySearchGroups, vars, &data); err != nil {
				return err
			}
			groups := make([]gqlGroup, 0, len(data.GroupSearch.Edges))
			for _, g := range data.GroupSearch.Edges {
				groups = append(groups, g.Node)
			}
			return flags.printJSON(cmd, groups)
		},
	}
	cmd.Flags().StringVar(&flagTopic, "topic", "", "Topic keyword to filter by")
	cmd.Flags().Float64Var(&flagLat, "lat", 0, "Latitude for location search")
	cmd.Flags().Float64Var(&flagLon, "lon", 0, "Longitude for location search")
	cmd.Flags().IntVar(&flagRadius, "radius", 25, "Search radius in miles")
	cmd.Flags().IntVar(&flagFirst, "first", 20, "Max groups to return")
	return cmd
}
