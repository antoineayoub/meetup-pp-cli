// Copyright 2026 Ayoub and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"github.com/spf13/cobra"
)

const queryListTopics = `
query ListTopics {
  topicCategories {
    edges { node { id name urlkey } }
  }
}`

func newGqlListTopicsCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "list-topics",
		Short:       "List all available Meetup topic categories.",
		Example:     "  meetup-pp-cli gql list-topics --json",
		Annotations: map[string]string{"pp:endpoint": "gql.list-topics", "mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			gc := newGQLClient()
			var data struct {
				TopicCategories struct {
					Edges []struct{ Node gqlTopic } `json:"edges"`
				} `json:"topicCategories"`
			}
			if err := gc.do(cmd.Context(), queryListTopics, nil, &data); err != nil {
				return err
			}
			topics := make([]gqlTopic, 0, len(data.TopicCategories.Edges))
			for _, e := range data.TopicCategories.Edges {
				topics = append(topics, e.Node)
			}
			return flags.printJSON(cmd, topics)
		},
	}
	return cmd
}
