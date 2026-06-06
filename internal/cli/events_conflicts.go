// Copyright 2026 Ayoub and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

// pp:data-source live

type conflictResult struct {
	YourEvent    string          `json:"yourEvent"`
	YourDateTime string          `json:"yourDateTime"`
	Competitors  []competitorRow `json:"competitors"`
}

type competitorRow struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	DateTime string `json:"dateTime"`
	Group    string `json:"group"`
	Going    int    `json:"going"`
	EventURL string `json:"eventUrl"`
	Topics   string `json:"topics,omitempty"`
}

func newNovelEventsConflictsCmd(flags *rootFlags) *cobra.Command {
	var flagGroup string
	var flagRadius int
	var flagLat float64
	var flagLon float64

	cmd := &cobra.Command{
		Use:   "conflicts",
		Short: "For each upcoming event in your group, see competing events happening nearby on the same date.",
		Long: `Fetches upcoming events for the target group and, for each, searches for
other events within the configured radius on the same date. Ranked by RSVP
count so you can prioritize the biggest scheduling conflicts.

Use alongside 'digest' to plan event scheduling and avoid date clashes.`,
		Example: `  meetup-pp-cli events conflicts --group le-wagon-montreal --radius 25
  meetup-pp-cli events conflicts --group le-wagon-montreal --lat 45.5 --lon -73.6 --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if flagGroup == "" {
				return fmt.Errorf("--group is required")
			}
			if dryRunOK(flags) {
				return nil
			}

			gc := newGQLClient()

			if !cmd.Flags().Changed("lat") {
				var data struct {
					GroupByUrlname *gqlGroup `json:"groupByUrlname"`
				}
				if err := gc.do(cmd.Context(), queryGetGroup, map[string]any{"urlname": flagGroup}, &data); err != nil {
					return err
				}
				if data.GroupByUrlname == nil {
					return fmt.Errorf("group not found: %s", flagGroup)
				}
				flagLat = data.GroupByUrlname.Lat
				flagLon = data.GroupByUrlname.Lon
			}

			upcomingEvents, err := fetchAllGroupEvents(cmd.Context(), gc, flagGroup, "ACTIVE", 50)
			if err != nil {
				return err
			}

			const conflictSearchQuery = `
query ConflictSearch($lat: Float!, $lon: Float!, $radius: Float, $start: ZonedDateTime, $end: ZonedDateTime) {
  eventSearch(filter: { lat: $lat, lon: $lon, radius: $radius, startDateRange: $start, endDateRange: $end }, first: 50) {
    edges { node {
      id title dateTime status eventUrl
      rsvps { yesCount }
      group { id name urlname }
    }}
  }
}`

			var results []conflictResult
			for _, ev := range upcomingEvents {
				if len(ev.DateTime) < 10 {
					continue
				}
				date := ev.DateTime[:10]

				vars := map[string]any{
					"lat":    flagLat,
					"lon":    flagLon,
					"radius": float64(flagRadius),
					"start":  date + "T00:00:00",
					"end":    date + "T23:59:59",
				}
				var searchData struct {
					EventSearch gqlEventPage `json:"eventSearch"`
				}
				if err := gc.do(cmd.Context(), conflictSearchQuery, vars, &searchData); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: conflict search for %q failed: %v\n", ev.Title, err)
					continue
				}

				var competitors []competitorRow
				for _, se := range searchData.EventSearch.Edges {
					c := se.Node
					if c.ID == ev.ID {
						continue
					}
					groupName := ""
					if c.Group != nil {
						groupName = c.Group.Name
					}
					competitors = append(competitors, competitorRow{
						ID:       c.ID,
						Title:    c.Title,
						DateTime: c.DateTime,
						Group:    groupName,
						Going:    c.Going(),
						EventURL: c.EventURL,
					})
				}
				if len(competitors) == 0 {
					continue
				}
				sort.Slice(competitors, func(i, j int) bool {
					return competitors[i].Going > competitors[j].Going
				})
				results = append(results, conflictResult{
					YourEvent:    ev.Title,
					YourDateTime: ev.DateTime,
					Competitors:  competitors,
				})
			}

			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(results)
			}
			if len(results) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No competing events found for your upcoming events.")
				return nil
			}
			for _, r := range results {
				fmt.Fprintf(cmd.OutOrStdout(), "\nYour event: %s (%s)\n", r.YourEvent, r.YourDateTime[:10])
				fmt.Fprintf(cmd.OutOrStdout(), "Competing events (%d):\n", len(r.Competitors))
				for _, c := range r.Competitors {
					t := "     "
					if len(c.DateTime) >= 16 {
						t = c.DateTime[11:16]
					}
					fmt.Fprintf(cmd.OutOrStdout(), "  [%3d going] %s  %s\n    %s\n",
						c.Going, t, c.Title, c.EventURL)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagGroup, "group", "", "Group URL slug (required)")
	cmd.Flags().IntVar(&flagRadius, "radius", 25, "Search radius in miles")
	cmd.Flags().Float64Var(&flagLat, "lat", 0, "Latitude (default: fetched from group)")
	cmd.Flags().Float64Var(&flagLon, "lon", 0, "Longitude (default: fetched from group)")
	_ = cmd.MarkFlagRequired("group")
	return cmd
}
