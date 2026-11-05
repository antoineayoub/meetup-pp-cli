---
name: pp-meetup
description: "Query, diff, and script Meetup groups and events from the terminal — with local sync, cross-group digests Trigger phrases: `upcoming meetup events`, `what's happening on meetup this week`, `find tech meetups near Montreal`, `check if le wagon has new events`, `compare meetup groups by topic`, `use meetup`, `run meetup-pp-cli`."
author: "Ayoub"
license: "Apache-2.0"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - meetup-pp-cli
---

# Meetup — Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `meetup-pp-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer:
   ```bash
   npx -y @mvanhorn/printing-press-library install meetup --cli-only
   ```
2. Verify: `meetup-pp-cli --version`
3. Ensure `$GOPATH/bin` (or `$HOME/go/bin`) is on `$PATH`.

If the `npx` install fails before this CLI has a public-library category, install Node or use the category-specific Go fallback after publish.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed with skill commands until verification succeeds.

meetup-pp-cli wraps Meetup's GraphQL API in a local-first CLI with a SQLite event store. Sync group events once, search and diff them offline, and pipe structured JSON into your scripts. The digest command folds multiple groups into one weekly timeline; the conflict detector shows competing events before you lose your RSVP audience.

## When to Use This CLI

Use meetup-pp-cli when you need to track, script, create, or diff events from one or more Meetup groups. Ideal for bootcamp coordinators who monitor recurring sessions, community scouts who aggregate across multiple topic areas, and organizers who want to detect competing events before scheduling or create events without touching the web UI.

## Anti-triggers

Do not use this CLI for:
- Managing RSVP waitlists or sending event announcements (requires the Meetup web UI or a confirmed OAuth token with the right scopes)
- Discovering events on platforms other than Meetup (Eventbrite, Luma — separate CLIs exist for those)
- Editing group settings or membership roles (use the Meetup organizer dashboard)

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Local state that compounds

- **`events diff`** — See exactly what changed in a group's event list since your last sync — new events, cancellations, and reschedules.

  _Use when you need to detect quiet schedule changes without re-reading an entire group page; exit code 1 if changes exist makes it cron-safe._

  ```bash
  meetup-pp-cli events diff --group le-wagon-montreal --json
  ```
- **`digest`** — Get a single sorted timeline of upcoming events across all your tracked groups for the next N days.

  _Use when you want a unified weekly event plan across multiple groups without switching tabs or running multiple searches._

  ```bash
  meetup-pp-cli digest --groups le-wagon-montreal,montreal-python --days 7 --json
  ```
- **`events series`** — Detect recurring event series within a group's history — cadence, last occurrence, and whether the next expected event is scheduled.

  _Use to identify a group's recurring programs (bootcamp sessions, weekly standups) and flag when an expected occurrence is missing from the schedule._

  ```bash
  meetup-pp-cli events series --group le-wagon-montreal --json
  ```

### Agent-native plumbing

- **`events conflicts`** — For each upcoming event in your group, see competing events in the same city on the same date that share your topics, ranked by RSVP count.

  _Use before scheduling an event to identify competing events that could split your audience; sort by RSVP count to prioritize the biggest threats._

  ```bash
  meetup-pp-cli events conflicts --group le-wagon-montreal --radius 25 --json
  ```
- **`groups similar`** — Find Meetup groups most topically similar to a group you already follow, ranked by Jaccard similarity over shared topic tags.

  _Use when expanding your community map or finding backup audiences; outputs groups you would never find via manual search._

  ```bash
  meetup-pp-cli groups similar --group le-wagon-montreal --limit 10 --json
  ```

### Organizer actions

- **`events create`** — Create a new event in a Meetup group from the terminal — title, date/time, duration, description, venue, and draft-or-publish in one command.

  _Use when creating events programmatically or in batch. Pairs with events diff to detect whether the published event appeared correctly. The --draft flag publishes as a draft instead of going live immediately._

  ```bash
  meetup-pp-cli events create --group le-wagon-montreal --title 'Demo Day' --date 2026-06-15T18:00 --duration 120 --description 'Monthly demo night'
  ```

- **`events update`** — Use for partial edits only. Every omitted field stays unchanged. Pass one or more `--topic-id` / `--host-id` values only when those lists should be replaced. `--photo` accepts JPEG/PNG/GIF and is checked locally before mutation.

  For Le Wagon Notion workflows, the `Topics` property should use the human-readable format `<topic_id> - <topic name>` (for example `1521079 - Automation`). Parse the numeric prefix before ` - ` and pass each extracted value as a repeated `--topic-id`. On the live Meetup `editEvent` mutation audited on 2026-08-28, the accepted payload shape is a flat list of IDs for `topics` (for example `["1521079","488822"]`), not objects like `[{"id":"1521079"}]`.
- **`events publish`** — The only explicit publication path. In `--agent` or `--no-input` mode, include `--yes`; an ACTIVE event is a successful no-op after verification.
- **`events hosts list --group <slug>`** — Discover valid `id`, `name`, and `eligible` values before assigning hosts.

For Le Wagon events, enforce the description CTA before create/update: first and final lines must be exactly `Register [Here](<WWW Link>)`. Do not substitute a raw URL or RSVP disclaimer. Never create a Le Wagon event when its Notion `Image` file is absent or not JPEG/PNG/GIF.

## Command Reference

**gql** — Manage gql

- `meetup-pp-cli gql create-event` — Create a new event in a Meetup group. Requires MEETUP_ACCESS_TOKEN with organizer scope for the target group.
- `meetup-pp-cli gql get-event` — Fetch full details for a single event by its ID and group URL name.
- `meetup-pp-cli gql get-group` — Fetch a Meetup group by its URL slug (e.g. 'le-wagon-montreal').
- `meetup-pp-cli gql list-group-events` — List upcoming or past events for a specific Meetup group.
- `meetup-pp-cli gql list-topics` — List all available Meetup topic categories for filtering events and groups.
- `meetup-pp-cli gql recommend-events` — Returns personalized or popular event recommendations near a lat/lon pair.
- `meetup-pp-cli gql search-events` — Find Meetup events by topic, location, and date range.
- `meetup-pp-cli gql search-groups` — Find Meetup groups matching a topic keyword near a location.


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
meetup-pp-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match — fall back to `--help` or use a narrower query.

## Recipes


### Monday morning change check

```bash
meetup-pp-cli events diff --group le-wagon-montreal --json | jq '.changes[] | select(.type == "RESCHEDULED")'
```

After syncing, pipe the diff output into jq to isolate rescheduled events for a quick status report.

### Find competing events this Saturday

```bash
meetup-pp-cli events conflicts --group le-wagon-montreal --radius 20 --json --select title,dateTime,rsvpCount,eventUrl
```

Use --select to return only the fields relevant to scheduling decisions.

### Discover similar communities

```bash
meetup-pp-cli groups similar --group le-wagon-montreal --limit 5 --agent
```

Structured agent output for downstream processing or to seed a `groups watch` watchlist.

### Weekly event digest in one shot

```bash
meetup-pp-cli digest --groups le-wagon-montreal,montreal-python,pyladiesmtl --days 7 --json
```

Merge upcoming events from three groups into one sorted list for weekly planning.

### Detect missing recurring sessions

```bash
meetup-pp-cli events series --group le-wagon-montreal --json --select name,cadenceDays,nextExpected,isScheduled
```

Use --select with dotted paths to pull just the scheduling fields from the series analysis.

### Create a draft event from the terminal

```bash
meetup-pp-cli events create --group le-wagon-montreal --title 'Demo Day' --date 2026-06-15T18:00 --duration 120 --description 'Monthly demo night' --draft --json
```

Use --draft to publish as a draft for review before going live. The --json flag returns the event ID and URL for piping into downstream scripts.

### Create from a canonical Notion event

```bash
meetup-pp-cli events create-from-notion \
  --page-id <notion-page-id> --city auto --draft --agent
```

This is the Le Wagon end-to-end path. It reads the canonical page's `Event name`, `Dates`, markdown description, `WWW Link`, `Format`, `Host ID`, `Topics`, and `Image`; creates the right city draft(s); attaches the image; and writes the returned URLs into `Meetup URL Montreal` and/or `Meetup URL Toronto`. `Online` maps to both cities in `--city auto`; onsite maps to Montréal only. It refuses to create when a destination URL callback property is missing, the image is absent, or the format is `Hybrid`.

## Auth Setup

Anonymous read access works without any token — group info, event search, and discovery all work out of the box. For write operations (events create, editEvent, deleteEvent), set MEETUP_ACCESS_TOKEN to an OAuth access token from meetup.com/meetup_api/auth/ with organizer scope on the target group.

Run `meetup-pp-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable** — JSON on stdout, errors on stderr
- **Filterable** — `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  meetup-pp-cli gql create-event --date-time 2026-01-15 --agent --select id,name,status
  ```
- **Previewable** — `--dry-run` shows the request without sending
- **Offline-friendly** — sync/search commands can use the local SQLite store when available
- **Non-interactive** — never prompts, every input is a flag
- **Explicit retries** — use `--idempotent` only when an already-existing create should count as success

### Response envelope

Commands that read from the local store or the API wrap output in a provenance envelope:

```json
{
  "meta": {"source": "live" | "local", "synced_at": "...", "reason": "..."},
  "results": <data>
}
```

Parse `.results` for data and `.meta.source` to know whether it's live or local. A human-readable `N results (live)` summary is printed to stderr only when stdout is a terminal AND no machine-format flag (`--json`, `--csv`, `--compact`, `--quiet`, `--plain`, `--select`) is set — piped/agent consumers and explicit-format runs get pure JSON on stdout.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
meetup-pp-cli feedback "the --since flag is inclusive but docs say exclusive"
meetup-pp-cli feedback --stdin < notes.txt
meetup-pp-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/meetup-pp-cli/feedback.jsonl`. They are never POSTed unless `MEETUP_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `MEETUP_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

Write what *surprised* you, not a bug report. Short, specific, one line: that is the part that compounds.

## Output Delivery

Every command accepts `--deliver <sink>`. The output goes to the named sink in addition to (or instead of) stdout, so agents can route command results without hand-piping. Three sinks are supported:

| Sink | Effect |
|------|--------|
| `stdout` | Default; write to stdout only |
| `file:<path>` | Atomically write output to `<path>` (tmp + rename) |
| `webhook:<url>` | POST the output body to the URL (`application/json` or `application/x-ndjson` when `--compact`) |

Unknown schemes are refused with a structured error naming the supported set. Webhook failures return non-zero and log the URL + HTTP status on stderr.

## Named Profiles

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled agent calls the same command every run with the same configuration - HeyGen's "Beacon" pattern.

```
meetup-pp-cli profile save briefing --json
meetup-pp-cli --profile briefing gql create-event --date-time 2026-01-15
meetup-pp-cli profile list --json
meetup-pp-cli profile show briefing
meetup-pp-cli profile delete briefing --yes
```

Explicit flags always win over profile values; profile values win over defaults. `agent-context` lists all available profiles under `available_profiles` so introspecting agents discover them at runtime.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 2 | Usage error (wrong arguments) |
| 3 | Resource not found |
| 4 | Authentication required |
| 5 | API error (upstream issue) |
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `meetup-pp-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

Install the MCP binary from this CLI's published public-library entry or pre-built release, then register it:

```bash
claude mcp add meetup-pp-mcp -- meetup-pp-mcp
```

Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which meetup-pp-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   meetup-pp-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `meetup-pp-cli <command> --help`.
