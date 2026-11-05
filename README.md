# Meetup CLI

**Query, diff, and script Meetup groups and events from the terminal — with local sync, cross-group digests, and conflict detection no web UI offers.**

meetup-pp-cli wraps Meetup's GraphQL API in a local-first CLI with a SQLite event store. Sync group events once, search and diff them offline, and pipe structured JSON into your scripts. The digest command folds multiple groups into one weekly timeline; the conflict detector shows competing events before you lose your RSVP audience.

Created by [@antoineayoub](https://github.com/antoineayoub) (Ayoub).

## Install

The recommended path installs both the `meetup-pp-cli` binary and the `pp-meetup` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install meetup
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install meetup --cli-only
```

For skill only — installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install meetup --skill-only
```

To constrain the skill install to one or more specific agents (repeatable — agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install meetup --agent claude-code
npx -y @mvanhorn/printing-press-library install meetup --agent claude-code --agent codex
```

### Without Node

The generated install path is category-agnostic until this CLI is published. If `npx` is not available before publish, install Node or use the category-specific Go fallback from the public-library entry after publish.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/meetup-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-meetup --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-meetup --force
```

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

```
Install the pp-meetup skill from https://github.com/mvanhorn/printing-press-library/tree/main/cli-skills/pp-meetup. The skill defines how its required CLI can be installed.
```

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle — Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/meetup-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `MEETUP_ACCESS_TOKEN` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


Install the MCP binary from this CLI's published public-library entry or pre-built release.

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "meetup": {
      "command": "meetup-pp-mcp",
      "env": {
        "MEETUP_ACCESS_TOKEN": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

Anonymous read access works without any token — group info, event search, and discovery all work out of the box. For write operations (events create, editEvent, deleteEvent), set MEETUP_ACCESS_TOKEN to an OAuth access token from meetup.com/meetup_api/auth/ with organizer scope on the target group.

## Quick Start

```bash
# Verify the CLI is set up and can reach the Meetup GraphQL API
meetup-pp-cli doctor


# Fetch a group by its URL slug — no token required
meetup-pp-cli groups get le-wagon-montreal --json


# List upcoming events for the group
meetup-pp-cli events --group le-wagon-montreal


# Sync group and event data to the local SQLite store
meetup-pp-cli sync --resources groups,events


# Cross-group weekly timeline
meetup-pp-cli digest --groups le-wagon-montreal,montreal-python --days 7


# See what changed since last sync — new, cancelled, rescheduled
meetup-pp-cli events diff --group le-wagon-montreal

```

## Unique Features

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

- **`events update`** — Update only explicitly supplied fields; repeated `--topic-id` and `--host-id` replace those lists. Image files are validated before any remote mutation.

  For Le Wagon Notion workflows, use the Notion `Topics` multi-select in the format `<topic_id> - <topic name>` (for example `1521079 - Automation`). Parse the numeric prefix before ` - ` and pass each extracted value as a repeated `--topic-id`. On the live Meetup `editEvent` mutation audited on 2026-08-28, `topics` must be sent as a flat list of IDs (for example `["1521079","488822"]`), not objects like `[{"id":"1521079"}]`.

  ```bash
  meetup-pp-cli events update --event-id 315809698 --group le-wagon-montreal \
    --title 'Turn Your Spreadsheet Into a Web App With AI' \
    --topic-id 10 --topic-id 42 --host-id 213975606 --photo ./cover.png --agent
  ```

- **`events publish`** — Explicitly publishes a DRAFT through Meetup's native mutation. It is idempotent for ACTIVE events and requires `--yes` for agent/non-interactive calls.

  ```bash
  meetup-pp-cli events publish --event-id 315809698 --group le-wagon-montreal --yes --agent
  ```

- **`events hosts list`** — Discover event hosts and eligibility before assigning host IDs.

  ```bash
  meetup-pp-cli events hosts list --group le-wagon-montreal --agent
  ```

- **`events create-from-notion`** — Create a Le Wagon Meetup event directly from one canonical Notion page. It maps title, dates, markdown description, `WWW Link`, online format/link, hosts, topics, and image; creates draft(s) by default; then writes returned URLs to `Meetup URL Montreal` / `Meetup URL Toronto`.

  ```bash
  meetup-pp-cli events create-from-notion --page-id <notion-page-id> --city auto --draft --agent
  ```

  For `--city auto`, `Online` creates Montréal + Toronto; onsite creates Montréal only. The command refuses an incomplete canonical record, `Hybrid`, missing images, or missing destination callback URL fields before it creates anything.

> `events update` never publishes an event. A Le Wagon description must start and end with `Register [Here](https://www.lewagon.com/events/your-event)`; keep the URL in `WWW Link` as the source of truth before invoking the CLI.

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

## Usage

Run `meetup-pp-cli --help` for the full command reference and flag list.

## Commands

### gql

Manage gql

- **`meetup-pp-cli gql create-event`** - Create a new event in a Meetup group. Requires MEETUP_ACCESS_TOKEN with
organizer scope for the target group. Use --draft to save as a draft
instead of publishing immediately.
- **`meetup-pp-cli gql get-event`** - Fetch full details for a single event by its ID and group URL name.
- **`meetup-pp-cli gql get-group`** - Fetch a Meetup group by its URL slug (e.g. "le-wagon-montreal").
- **`meetup-pp-cli gql list-group-events`** - List upcoming or past events for a specific Meetup group.
- **`meetup-pp-cli gql list-topics`** - List all available Meetup topic categories for filtering events and groups.
- **`meetup-pp-cli gql recommend-events`** - Returns personalized or popular event recommendations near a lat/lon pair.
- **`meetup-pp-cli gql search-events`** - Find Meetup events by topic, location, and date range.
- **`meetup-pp-cli gql search-groups`** - Find Meetup groups matching a topic keyword near a location.


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
meetup-pp-cli gql create-event --date-time 2026-01-15

# JSON for scripting and agents
meetup-pp-cli gql create-event --date-time 2026-01-15 --json

# Filter to specific fields
meetup-pp-cli gql create-event --date-time 2026-01-15 --json --select id,name,status

# Dry run — show the request without sending
meetup-pp-cli gql create-event --date-time 2026-01-15 --dry-run

# Agent mode — JSON + compact + no prompts in one flag
meetup-pp-cli gql create-event --date-time 2026-01-15 --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select id,name` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries when a no-op success is acceptable
- **Confirmable** - `--yes` for explicit confirmation of destructive actions
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Health Check

```bash
meetup-pp-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/meetup-pp-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `MEETUP_ACCESS_TOKEN` | per_call | No | Set to your API credential. |
| `MEETUP_BEARER_AUTH` | per_call | No | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `meetup-pp-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `meetup-pp-cli doctor` to check credentials
- Verify the environment variable is set: `echo $MEETUP_ACCESS_TOKEN`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific

- **GraphQL error 401 on writes** — Set MEETUP_ACCESS_TOKEN to your OAuth bearer token from meetup.com/meetup_api/auth/
- **events diff shows nothing after first sync** — Run sync again after an event has changed, or use --group to target a specific group
- **groups similar returns very few results** — The group may have few topic tags; check with `groups get <urlname> --json | jq .activeTopics`

---

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**meetup-scraper**](https://github.com/banjtheman/meetup-scraper) — Python
- [**scrape-meetup**](https://github.com/digitalcolony/scrape-meetup) — JavaScript
- [**meetup-scraper-by-topic**](https://github.com/javaldro/meetup-scraper-by-topic) — Python

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
