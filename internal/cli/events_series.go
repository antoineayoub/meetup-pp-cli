// Copyright 2026 Ayoub and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// pp:data-source live

type seriesResult struct {
	Name        string `json:"name"`
	Occurrences int    `json:"occurrences"`
	FirstSeen   string `json:"firstSeen"`
	LastSeen    string `json:"lastSeen"`
	// Inferred cadence in days (0 if insufficient data)
	CadenceDays int    `json:"cadenceDays,omitempty"`
	NextExpected string `json:"nextExpected,omitempty"`
	IsScheduled  bool  `json:"isScheduled"`
	Events       []struct {
		ID       string `json:"id"`
		DateTime string `json:"dateTime"`
		Title    string `json:"title"`
	} `json:"events"`
}

func newNovelEventsSeriesCmd(flags *rootFlags) *cobra.Command {
	var flagGroup string
	var flagMinOccurrences int

	cmd := &cobra.Command{
		Use:   "series",
		Short: "Detect recurring event series within a group's history using title-prefix clustering.",
		Long: `Fetches the group's full event history and clusters events by shared title
prefix. Series with 2+ occurrences are emitted with cadence and next-expected
date analysis.

Detects recurring event series using title-prefix clustering (exact substring
match). Reports cadence and flags missing expected occurrences.`,
		Example: `  meetup-pp-cli events series --group le-wagon-montreal
  meetup-pp-cli events series --group le-wagon-montreal --min-occurrences 3 --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if flagGroup == "" {
				return fmt.Errorf("--group is required")
			}
			if dryRunOK(flags) {
				return nil
			}

			gc := newGQLClient()

			// Fetch full history (PAST + ACTIVE)
			past, err := fetchAllGroupEvents(cmd.Context(), gc, flagGroup, "PAST", 500)
			if err != nil {
				return fmt.Errorf("fetching past events: %w", err)
			}
			active, err := fetchAllGroupEvents(cmd.Context(), gc, flagGroup, "ACTIVE", 100)
			if err != nil {
				return fmt.Errorf("fetching active events: %w", err)
			}

			all := append(past, active...)
			sort.Slice(all, func(i, j int) bool {
				return all[i].DateTime < all[j].DateTime
			})

			// Cluster by title prefix (longest common prefix of ≥6 chars)
			type cluster struct {
				prefix string
				events []gqlEvent
			}
			clusters := make(map[string]*cluster)

			for _, ev := range all {
				matched := false
				for prefix, cl := range clusters {
					if strings.HasPrefix(strings.ToLower(ev.Title), strings.ToLower(prefix)) ||
						strings.HasPrefix(strings.ToLower(prefix), strings.ToLower(ev.Title[:min(len(ev.Title), len(prefix))])) {
						cl.events = append(cl.events, ev)
						matched = true
						break
					}
				}
				if !matched {
					// Find prefix to add (use first 6+ chars of title)
					prefix := ev.Title
					if len(prefix) > 30 {
						// Trim at word boundary
						if idx := strings.LastIndex(prefix[:30], " "); idx > 6 {
							prefix = prefix[:idx]
						} else {
							prefix = prefix[:20]
						}
					}
					clusters[prefix] = &cluster{prefix: prefix, events: []gqlEvent{ev}}
				}
			}

			var results []seriesResult
			now := time.Now().UTC()

			for prefix, cl := range clusters {
				if len(cl.events) < flagMinOccurrences {
					continue
				}

				evList := make([]struct {
					ID       string `json:"id"`
					DateTime string `json:"dateTime"`
					Title    string `json:"title"`
				}, 0, len(cl.events))
				for _, e := range cl.events {
					evList = append(evList, struct {
						ID       string `json:"id"`
						DateTime string `json:"dateTime"`
						Title    string `json:"title"`
					}{ID: e.ID, DateTime: e.DateTime, Title: e.Title})
				}

				first := cl.events[0].DateTime
				last := cl.events[len(cl.events)-1].DateTime

				cadenceDays := 0
				nextExpected := ""
				isScheduled := false

				if len(cl.events) >= 2 {
					// Compute average gap
					var totalDays float64
					count := 0
					for i := 1; i < len(cl.events); i++ {
						t1, err1 := time.Parse(time.RFC3339, cl.events[i-1].DateTime)
						t2, err2 := time.Parse(time.RFC3339, cl.events[i].DateTime)
						if err1 != nil || err2 != nil {
							continue
						}
						totalDays += t2.Sub(t1).Hours() / 24
						count++
					}
					if count > 0 {
						cadenceDays = int(totalDays / float64(count))
					}

					// Project next expected
					lastTime, err := time.Parse(time.RFC3339, last)
					if err == nil && cadenceDays > 0 {
						nextTime := lastTime.AddDate(0, 0, cadenceDays)
						nextExpected = nextTime.Format("2006-01-02")
						// Check if any active event is near next expected
						for _, ae := range active {
							if ae.DateTime[:10] == nextTime.Format("2006-01-02") ||
								strings.HasPrefix(strings.ToLower(ae.Title), strings.ToLower(prefix[:min(len(prefix), 10)])) {
								isScheduled = true
								break
							}
						}
						// Flag if overdue
						if nextTime.Before(now) && !isScheduled {
							nextExpected = nextExpected + " (overdue)"
						}
					}
				}

				results = append(results, seriesResult{
					Name:         prefix,
					Occurrences:  len(cl.events),
					FirstSeen:    first[:min(len(first), 10)],
					LastSeen:     last[:min(len(last), 10)],
					CadenceDays:  cadenceDays,
					NextExpected: nextExpected,
					IsScheduled:  isScheduled,
					Events:       evList,
				})
			}

			sort.Slice(results, func(i, j int) bool {
				return results[i].Occurrences > results[j].Occurrences
			})

			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(results)
			}

			if len(results) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No recurring series found (min-occurrences=%d).\n", flagMinOccurrences)
				return nil
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Recurring event series in %s:\n\n", flagGroup)
			for _, r := range results {
				scheduled := ""
				if r.NextExpected != "" {
					scheduled = fmt.Sprintf("  next: %s", r.NextExpected)
					if r.IsScheduled {
						scheduled += " ✓"
					}
				}
				fmt.Fprintf(cmd.OutOrStdout(), "  %s\n  %dx  first: %s  last: %s  cadence: ~%dd%s\n\n",
					r.Name, r.Occurrences, r.FirstSeen, r.LastSeen, r.CadenceDays, scheduled)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagGroup, "group", "", "Group URL slug (required)")
	cmd.Flags().IntVar(&flagMinOccurrences, "min-occurrences", 2, "Minimum number of occurrences to qualify as a series")
	_ = cmd.MarkFlagRequired("group")
	return cmd
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
