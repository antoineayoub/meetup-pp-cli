// Copyright 2026 Ayoub and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newGqlRecommendEventsCmd(flags *rootFlags) *cobra.Command {
	var flagLat float64
	var flagLon float64
	var flagFirst int
	var flagTopic string
	var flagAfter string

	cmd := &cobra.Command{
		Use:         "recommend-events",
		Short:       "Get recommended events near a location, optionally filtered by topic and time-of-day.",
		Example:     "  meetup-pp-cli gql recommend-events --lat 45.5 --lon -73.6 --topic python --after 18:00",
		Annotations: map[string]string{"pp:endpoint": "gql.recommend-events", "mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if !cmd.Flags().Changed("lat") {
				return fmt.Errorf("--lat and --lon are required")
			}
			gc := newGQLClient()
			vars := map[string]any{
				"lat":   flagLat,
				"lon":   flagLon,
				"first": flagFirst,
			}
			var data struct {
				RecommendedEvents gqlEventPage `json:"recommendedEvents"`
			}
			if err := gc.do(cmd.Context(), queryRecommendedEvents, vars, &data); err != nil {
				return err
			}
			events := make([]gqlEvent, 0, len(data.RecommendedEvents.Edges))
			for _, e := range data.RecommendedEvents.Edges {
				ev := e.Node
				// Local post-filter: topic substring
				if flagTopic != "" {
					match := false
					topicLower := strings.ToLower(flagTopic)
					for _, edge := range ev.Topics.Edges {
						t := edge.Node
						if strings.Contains(strings.ToLower(t.Name), topicLower) {
							match = true
							break
						}
					}
					if !match {
						continue
					}
				}
				// Local post-filter: time-of-day (HH:MM threshold)
				if flagAfter != "" && len(ev.DateTime) >= 16 {
					evTime := ev.DateTime[11:16] // "HH:MM" slice from ISO 8601
					if evTime < flagAfter {
						continue
					}
				}
				events = append(events, ev)
			}
			return flags.printJSON(cmd, events)
		},
	}
	cmd.Flags().Float64Var(&flagLat, "lat", 0, "Latitude (required)")
	cmd.Flags().Float64Var(&flagLon, "lon", 0, "Longitude (required)")
	cmd.Flags().IntVar(&flagFirst, "first", 20, "Max events to fetch from API before local filtering")
	cmd.Flags().StringVar(&flagTopic, "topic", "", "Filter by topic keyword (local post-filter)")
	cmd.Flags().StringVar(&flagAfter, "after", "", "Only show events starting at or after HH:MM (local post-filter)")
	return cmd
}
