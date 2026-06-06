// Copyright 2026 Ayoub and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
)

// pp:data-source live

type digestEntry struct {
	DateTime string `json:"dateTime"`
	Title    string `json:"title"`
	Group    string `json:"group"`
	Going    int    `json:"going"`
	EventURL string `json:"eventUrl"`
	IsOnline bool   `json:"isOnline"`
}

func newNovelDigestCmd(flags *rootFlags) *cobra.Command {
	var flagGroups string
	var flagDays int

	cmd := &cobra.Command{
		Use:   "digest",
		Short: "Get a single sorted timeline of upcoming events across all your tracked groups for the next N days.",
		Long: `Fetches upcoming events from each specified group in parallel and merges
them into a single deduped, chronologically sorted list.

Multi-group timeline for the coming N days. Each group requires a separate API
call; aggregation and deduplication happen locally.
For per-group change detection use 'events diff' instead.`,
		Example: `  meetup-pp-cli digest --groups le-wagon-montreal,montreal-python --days 7
  meetup-pp-cli digest --groups le-wagon-montreal,montreal-python --days 14 --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if flagGroups == "" {
				return fmt.Errorf("--groups is required (comma-separated group URL slugs)")
			}
			if dryRunOK(flags) {
				return nil
			}

			slugs := strings.Split(flagGroups, ",")
			for i, s := range slugs {
				slugs[i] = strings.TrimSpace(s)
			}

			cutoff := time.Now().UTC().Add(time.Duration(flagDays) * 24 * time.Hour)
			now := time.Now().UTC()

			gc := newGQLClient()

			type result struct {
				slug   string
				events []gqlEvent
				err    error
			}
			ch := make(chan result, len(slugs))
			var wg sync.WaitGroup
			for _, slug := range slugs {
				wg.Add(1)
				go func(s string) {
					defer wg.Done()
					evs, err := fetchAllGroupEvents(cmd.Context(), gc, s, "ACTIVE", 100)
					ch <- result{slug: s, events: evs, err: err}
				}(slug)
			}
			wg.Wait()
			close(ch)

			seen := make(map[string]bool)
			var entries []digestEntry
			var fetchErrs []string
			for r := range ch {
				if r.err != nil {
					fetchErrs = append(fetchErrs, fmt.Sprintf("%s: %v", r.slug, r.err))
					continue
				}
				for _, e := range r.events {
					if seen[e.ID] {
						continue
					}
					// Filter to window
					dt, err := time.Parse(time.RFC3339, e.DateTime)
					if err != nil {
						// Try without timezone
						dt, err = time.Parse("2006-01-02T15:04", e.DateTime[:16])
					}
					if err == nil && (dt.Before(now) || dt.After(cutoff)) {
						continue
					}
					seen[e.ID] = true
					entries = append(entries, digestEntry{
						DateTime: e.DateTime,
						Title:    e.Title,
						Group:    r.slug,
						Going:    e.Going(),
						EventURL: e.EventURL,
						IsOnline: e.IsOnline(),
					})
				}
			}

			for _, fe := range fetchErrs {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", fe)
			}

			sort.Slice(entries, func(i, j int) bool {
				return entries[i].DateTime < entries[j].DateTime
			})

			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(entries)
			}

			if len(entries) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No events in the next %d days across %s.\n", flagDays, flagGroups)
				return nil
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Upcoming events — next %d days (%d groups, %d events)\n\n",
				flagDays, len(slugs), len(entries))
			var lastDate string
			for _, e := range entries {
				date := ""
				if len(e.DateTime) >= 10 {
					date = e.DateTime[:10]
				}
				if date != lastDate {
					fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", date)
					lastDate = date
				}
				onlineFlag := ""
				if e.IsOnline {
					onlineFlag = " [online]"
				}
				timeStr := ""
				if len(e.DateTime) >= 16 {
					timeStr = e.DateTime[11:16]
				}
				fmt.Fprintf(cmd.OutOrStdout(), "    %s  %-45s [%s]%s\n",
					timeStr, truncate(e.Title, 45), e.Group, onlineFlag)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagGroups, "groups", "", "Comma-separated group URL slugs (required)")
	cmd.Flags().IntVar(&flagDays, "days", 7, "Number of days ahead to include")
	_ = cmd.MarkFlagRequired("groups")
	return cmd
}
