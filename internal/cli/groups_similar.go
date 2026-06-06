// Copyright 2026 Ayoub and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// pp:data-source live

type similarGroup struct {
	Urlname     string   `json:"urlname"`
	Name        string   `json:"name"`
	City        string   `json:"city"`
	Country     string   `json:"country"`
	MemberCount int      `json:"memberCount"`
	Jaccard     float64  `json:"jaccardSimilarity"`
	Topics      []string `json:"sharedTopics"`
	Link        string   `json:"link"`
}

func newNovelGroupsSimilarCmd(flags *rootFlags) *cobra.Command {
	var flagGroup string
	var flagLimit int

	cmd := &cobra.Command{
		Use:   "similar",
		Short: "Find Meetup groups most topically similar to a group, ranked by Jaccard similarity.",
		Long: `Fetches the target group's topic tags, searches for other groups sharing
those topics, deduplicates results, and ranks by Jaccard similarity score.

Given a group you follow, surfaces groups most topically similar using Jaccard
similarity over topic tag sets. Useful for expanding a community map.
Do NOT use for new-group alerts; use 'groups watch' instead.`,
		Example: `  meetup-pp-cli groups similar --group le-wagon-montreal --limit 10
  meetup-pp-cli groups similar --group le-wagon-montreal --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if flagGroup == "" {
				return fmt.Errorf("--group is required")
			}
			if dryRunOK(flags) {
				return nil
			}

			gc := newGQLClient()

			// Fetch the reference group
			var refData struct {
				GroupByUrlname *gqlGroup `json:"groupByUrlname"`
			}
			if err := gc.do(cmd.Context(), queryGetGroup, map[string]any{"urlname": flagGroup}, &refData); err != nil {
				return err
			}
			if refData.GroupByUrlname == nil {
				return fmt.Errorf("group not found: %s", flagGroup)
			}
			ref := refData.GroupByUrlname

			if len(ref.Topics) == 0 {
				return fmt.Errorf("group %s has no topic tags — cannot compute similarity", flagGroup)
			}

			refTopics := make(map[string]bool)
			for _, t := range ref.Topics {
				refTopics[strings.ToLower(t.Name)] = true
			}

			// Search per topic, collect candidate groups
			candidateMap := make(map[string]*gqlGroup)
			for _, t := range ref.Topics {
				vars := map[string]any{
					"topic":  t.Name,
					"lat":    ref.Lat,
					"lon":    ref.Lon,
					"radius": float64(100), // broad radius for discovery
					"first":  50,
				}
				var data struct {
					GroupSearch gqlGroupPage `json:"groupSearch"`
				}
				if err := gc.do(cmd.Context(), querySearchGroups, vars, &data); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: search for topic %q failed: %v\n", t.Name, err)
					continue
				}
				for _, edge := range data.GroupSearch.Edges {
					g := edge.Node
					if g.Urlname == flagGroup {
						continue
					}
					if _, exists := candidateMap[g.Urlname]; !exists {
						copied := g
						candidateMap[g.Urlname] = &copied
					}
				}
			}

			// Score by Jaccard
			var ranked []similarGroup
			for _, g := range candidateMap {
				candTopics := make(map[string]bool)
				for _, t := range g.Topics {
					candTopics[strings.ToLower(t.Name)] = true
				}
				intersection := 0
				var shared []string
				for k := range refTopics {
					if candTopics[k] {
						intersection++
						shared = append(shared, k)
					}
				}
				union := len(refTopics) + len(candTopics) - intersection
				jaccard := 0.0
				if union > 0 {
					jaccard = float64(intersection) / float64(union)
				}
				memberCount := 0
				if g.Stats != nil && g.Stats.MemberCounts != nil {
					memberCount = g.Stats.MemberCounts.All
				}
				ranked = append(ranked, similarGroup{
					Urlname:     g.Urlname,
					Name:        g.Name,
					City:        g.City,
					Country:     g.Country,
					MemberCount: memberCount,
					Jaccard:     jaccard,
					Topics:      shared,
					Link:        g.Link,
				})
			}

			sort.Slice(ranked, func(i, j int) bool {
				if ranked[i].Jaccard != ranked[j].Jaccard {
					return ranked[i].Jaccard > ranked[j].Jaccard
				}
				return ranked[i].MemberCount > ranked[j].MemberCount
			})

			if flagLimit > 0 && len(ranked) > flagLimit {
				ranked = ranked[:flagLimit]
			}

			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(ranked)
			}

			if len(ranked) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No similar groups found.")
				return nil
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Groups similar to %s (by topic Jaccard similarity):\n\n", flagGroup)
			for i, g := range ranked {
				fmt.Fprintf(cmd.OutOrStdout(), "  %2d. [%.2f] %s (%s, %s)\n      Shared: %s\n      %s\n",
					i+1, g.Jaccard, g.Name, g.City, g.Country,
					strings.Join(g.Topics, ", "), g.Link)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagGroup, "group", "", "Reference group URL slug (required)")
	cmd.Flags().IntVar(&flagLimit, "limit", 10, "Max results to return")
	_ = cmd.MarkFlagRequired("group")
	return cmd
}
