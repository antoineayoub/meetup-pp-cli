// Copyright 2026 Ayoub and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"meetup-pp-cli/internal/store"
)

// pp:data-source local

type eventSnapshotRow struct {
	ID       string
	Title    string
	DateTime string
	Status   string
}

type eventDiff struct {
	Type     string `json:"type"` // NEW, CANCELLED, RESCHEDULED, STATUS_CHANGED
	ID       string `json:"id"`
	Title    string `json:"title"`
	DateTime string `json:"dateTime,omitempty"`
	OldValue string `json:"oldValue,omitempty"`
	NewValue string `json:"newValue,omitempty"`
	EventURL string `json:"eventUrl,omitempty"`
}

func newNovelEventsDiffCmd(flags *rootFlags) *cobra.Command {
	var flagGroup string
	var flagDBPath string

	cmd := &cobra.Command{
		Use:   "diff",
		Short: "See exactly what changed in a group's event list since your last sync.",
		Long: `Fetches current events from the Meetup API and compares them to the last
snapshot stored locally. Reports NEW, CANCELLED, and RESCHEDULED events.

Run this command regularly (e.g. via cron) to detect quiet schedule changes
without re-reading the full group page. Exit code 1 when changes are detected.

Do NOT use for multi-group summaries; use 'digest' instead.`,
		Example: `  meetup-pp-cli events diff --group le-wagon-montreal --json
  meetup-pp-cli events diff --group le-wagon-montreal && echo "no changes"`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if flagGroup == "" {
				return fmt.Errorf("--group is required")
			}
			if dryRunOK(flags) {
				return nil
			}

			dbPath := flagDBPath
			if dbPath == "" {
				home, _ := os.UserHomeDir()
				dbPath = filepath.Join(home, ".local", "share", "meetup-pp-cli", "meetup.db")
			}
			if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
				return fmt.Errorf("creating DB dir: %w", err)
			}

			db, err := store.Open(dbPath)
			if err != nil {
				return fmt.Errorf("opening store: %w", err)
			}
			defer db.Close()

			if !hintIfUnsynced(cmd, db, "") {
				hintIfStale(cmd, db, "", flags.maxAge)
			}

			// Load snapshot
			prior, err := loadEventSnapshot(db, flagGroup)
			if err != nil {
				return fmt.Errorf("loading snapshot: %w", err)
			}

			// Fetch live
			gc := newGQLClient()
			live, err := fetchAllGroupEvents(cmd.Context(), gc, flagGroup, "ACTIVE", 200)
			if err != nil {
				return err
			}

			// Compute diff
			diffs := computeEventDiff(prior, live)

			// Save new snapshot
			if err := saveEventSnapshot(db, flagGroup, live); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not save snapshot: %v\n", err)
			}

			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(diffs)
			}

			if len(diffs) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No changes detected.")
				return nil
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Changes in %s (%d):\n\n", flagGroup, len(diffs))
			for _, d := range diffs {
				switch d.Type {
				case "NEW":
					fmt.Fprintf(cmd.OutOrStdout(), "  + NEW       %s  %s\n    %s\n", d.DateTime, d.Title, d.EventURL)
				case "CANCELLED":
					fmt.Fprintf(cmd.OutOrStdout(), "  - CANCELLED %s  %s\n", d.OldValue, d.Title)
				case "RESCHEDULED":
					fmt.Fprintf(cmd.OutOrStdout(), "  ~ RESCHEDULED %s\n    %s → %s\n", d.Title, d.OldValue, d.NewValue)
				case "STATUS_CHANGED":
					fmt.Fprintf(cmd.OutOrStdout(), "  ~ STATUS %s: %s → %s\n", d.Title, d.OldValue, d.NewValue)
				}
			}

			// Non-zero exit when changes detected (cron-safe)
			return &cliError{code: 1, err: fmt.Errorf("%d change(s) detected in %s", len(diffs), flagGroup)}
		},
	}
	cmd.Flags().StringVar(&flagGroup, "group", "", "Group URL slug (required)")
	cmd.Flags().StringVar(&flagDBPath, "db", "", "SQLite database path (default: ~/.local/share/meetup-pp-cli/meetup.db)")
	_ = cmd.MarkFlagRequired("group")
	return cmd
}

func computeEventDiff(prior map[string]eventSnapshotRow, live []gqlEvent) []eventDiff {
	var diffs []eventDiff
	liveByID := make(map[string]gqlEvent, len(live))
	for _, e := range live {
		liveByID[e.ID] = e
	}

	// New events not in prior
	for _, e := range live {
		if _, exists := prior[e.ID]; !exists {
			diffs = append(diffs, eventDiff{
				Type:     "NEW",
				ID:       e.ID,
				Title:    e.Title,
				DateTime: e.DateTime,
				EventURL: e.EventURL,
			})
		} else {
			p := prior[e.ID]
			// Rescheduled
			if p.DateTime != e.DateTime {
				diffs = append(diffs, eventDiff{
					Type:     "RESCHEDULED",
					ID:       e.ID,
					Title:    e.Title,
					OldValue: p.DateTime,
					NewValue: e.DateTime,
				})
			} else if p.Status != e.Status {
				diffs = append(diffs, eventDiff{
					Type:     "STATUS_CHANGED",
					ID:       e.ID,
					Title:    e.Title,
					OldValue: p.Status,
					NewValue: e.Status,
				})
			}
		}
	}

	// Cancelled (in prior but not in live)
	for id, p := range prior {
		if _, exists := liveByID[id]; !exists {
			diffs = append(diffs, eventDiff{
				Type:     "CANCELLED",
				ID:       id,
				Title:    p.Title,
				OldValue: p.DateTime,
			})
		}
	}
	return diffs
}

func loadEventSnapshot(db *store.Store, group string) (map[string]eventSnapshotRow, error) {
	rows, err := db.DB().Query(
		`SELECT event_id, title, date_time, status FROM event_snapshots WHERE group_urlname = ?`,
		group,
	)
	if err != nil {
		// Table may not exist yet on first run
		return map[string]eventSnapshotRow{}, nil
	}
	defer rows.Close()

	m := make(map[string]eventSnapshotRow)
	for rows.Next() {
		var r eventSnapshotRow
		if err := rows.Scan(&r.ID, &r.Title, &r.DateTime, &r.Status); err != nil {
			continue
		}
		m[r.ID] = r
	}
	return m, rows.Err()
}

func saveEventSnapshot(db *store.Store, group string, events []gqlEvent) error {
	sqlDB := db.DB()
	// Ensure table exists
	_, err := sqlDB.Exec(`CREATE TABLE IF NOT EXISTS event_snapshots (
		group_urlname TEXT NOT NULL,
		event_id TEXT NOT NULL,
		title TEXT,
		date_time TEXT,
		status TEXT,
		snapped_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (group_urlname, event_id)
	)`)
	if err != nil {
		return err
	}

	tx, err := sqlDB.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Clear old snapshot for this group
	if _, err := tx.Exec(`DELETE FROM event_snapshots WHERE group_urlname = ?`, group); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, e := range events {
		_, err := tx.Exec(
			`INSERT INTO event_snapshots (group_urlname, event_id, title, date_time, status, snapped_at) VALUES (?,?,?,?,?,?)`,
			group, e.ID, e.Title, e.DateTime, e.Status, now,
		)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}


