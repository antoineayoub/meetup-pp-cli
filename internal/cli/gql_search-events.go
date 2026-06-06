// Copyright 2026 Ayoub and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"github.com/spf13/cobra"
)

func newGqlSearchEventsCmd(flags *rootFlags) *cobra.Command {
	var flagQuery string
	var flagLat float64
	var flagLon float64
	var flagRadius int
	var flagFirst int

	cmd := &cobra.Command{
		Use:         "search-events",
		Short:       "Find Meetup events by topic, location, and date range.",
		Example:     "  meetup-pp-cli gql search-events --query golang --lat 45.5 --lon -73.6",
		Annotations: map[string]string{"pp:endpoint": "gql.search-events", "mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			gc := newGQLClient()
			vars := map[string]any{
				"first": flagFirst,
				"query": flagQuery,
				"lat":   flagLat,
				"lon":   flagLon,
			}
			if flagRadius > 0 {
				vars["radius"] = float64(flagRadius)
			}

			var data struct {
				EventSearch gqlEventPage `json:"eventSearch"`
			}
			if err := gc.do(cmd.Context(), querySearchEvents, vars, &data); err != nil {
				return err
			}
			events := make([]gqlEvent, 0, len(data.EventSearch.Edges))
			for _, e := range data.EventSearch.Edges {
				events = append(events, e.Node)
			}
			return flags.printJSON(cmd, events)
		},
	}
	cmd.Flags().StringVar(&flagQuery, "query", "", "Keyword search query")
	cmd.Flags().Float64Var(&flagLat, "lat", 0, "Latitude for location search")
	cmd.Flags().Float64Var(&flagLon, "lon", 0, "Longitude for location search")
	cmd.Flags().IntVar(&flagRadius, "radius", 25, "Search radius in miles")
	cmd.Flags().IntVar(&flagFirst, "first", 20, "Max events to return")
	return cmd
}
