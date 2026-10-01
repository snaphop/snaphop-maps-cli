---
name: snaphop-maps
description: Create, publish, change and withdraw interactive web maps with markers through the SnapHop Maps CLI (snaphop-maps). Use when the user wants a map of places, addresses, stops or points of interest that they can open, share as a link or embed in a web page, or wants to change or take down a map made earlier with SnapHop Maps.
license: MIT
compatibility: Needs a shell that can run the snaphop-maps binary and reach https://maps.snaphop.ai over HTTPS.
metadata:
  homepage: https://github.com/snaphop/snaphop-maps-cli
---

# SnapHop Maps

SnapHop Maps publishes an interactive web map with plain-text markers at a stable link, the **page**, and as HTML to
embed in any web page. The `snaphop-maps` command drives it. Every command prints one JSON document on stdout. A
failure prints `{"error": {"code", "message", "hint", ...}}` on stderr and exits with a nonzero status. Read `hint`: it is
the next step.

## Before the first map

1. Check the CLI is there: `snaphop-maps version`. If it is not, install it with
   `go install github.com/snaphop/snaphop-maps-cli/cmd/snaphop-maps@latest`, or download the binary for this platform
   from https://github.com/snaphop/snaphop-maps-cli/releases.
2. Check for an account: `snaphop-maps credentials`. If `keySource` is `none`, open one once:

   ```sh
   snaphop-maps register-agent --name "Your agent's name"
   ```

   The API key it returns is the account's only credential, and the service shows it only once. The CLI keeps it
   in a credentials file that only the user can read, before it prints it, and uses it for every later command.
   Never repeat the key to the user, write it into a file or put it in a command line unless they ask, or unless
   the exit status is 6 (below). Do not register again when an account is already kept: `ACCOUNT_ALREADY_KEPT`
   means use the one you have.

## Make a map

You supply the coordinates. Every position is **`[longitude, latitude]`, longitude first**. Lisbon is
`[-9.14, 38.71]`, not `[38.71, -9.14]`. Use coordinates you are sure of or that a tool gave you. Never guess them:
a marker in the sea looks as wrong to the user as a missing one.

```sh
snaphop-maps create-map --name "Coffee in Lisbon" --style positron \
  --markers '[{"position": [-9.1427, 38.7107], "title": "A Brasileira", "description": "Open since 1905"},
              {"position": [-9.1365, 38.7139], "title": "Fabrica Coffee Roasters", "color": "#b5452b"}]'
```

- The answer's `id` names the map in every later command. `published.page` is the link to give the user.
  `published.embedScript` and `published.embedFrame` are HTML they can paste into a web page. A new map's page may
  take up to a minute to appear.
- `name` and each marker `title` are 1 to 120 characters. A `description` is at most 1000 characters and may contain
  line breaks. `color` is `#rrggbb`. All of it is plain text: never HTML or Markdown.
- A map holds at most 500 markers. `style` is one of `liberty` (the default), `bright`, `positron`, `dark` or
  `fiord`. `snaphop-maps get-installation` lists the styles this installation offers.
- Leave `--view` out and the map opens on all of its markers. Give one only when the user wants a particular
  opening view: `--view '{"center": [-9.14, 38.71], "zoom": 13}'`. `--controls '{"cooperativeGestures": true}'`
  makes a page scroll past an embedded map until the reader uses two fingers or a modifier key.
- A long marker list is easier to pass from a file, `--markers @markers.json`, or from standard input,
  `--markers -`. Either may hold up to 8 MiB (`INPUT_TOO_LARGE` beyond that).
- `--publish=false` keeps the map a draft, and `snaphop-maps publish-map MAP_ID` publishes it later. A switch takes
  its value after `=`: `--publish false` is refused, since `false` would be read as the map's id.

## Change, publish and withdraw

```sh
snaphop-maps list-maps
snaphop-maps get-map MAP_ID
snaphop-maps update-map MAP_ID --name "Coffee in Lisbon, 2026"
snaphop-maps update-map MAP_ID --markers '[{"position": [-9.1427, 38.7107], "title": "A Brasileira"}]'
snaphop-maps publish-map MAP_ID
snaphop-maps withdraw-map MAP_ID --yes
```

- `list-maps` lists each map's `id`, `name`, `activeRelease`, `unpublishedChanges` and marker count.
- `get-map` returns the whole map, including its `draft`: the definition you can change.
- `update-map` replaces only the fields you give. `--markers` replaces the **whole** list, so to add one marker,
  read the map with `get-map` and send every marker plus the new one. New markers without a new `--view` get a view
  that shows them all. It publishes unless given `--publish=false`, and the link and every embed show the change
  within about a minute.
- `withdraw-map` takes the map down for good: the link and every embed stop working, and it cannot be undone. Ask
  the user first, and pass `--yes` only once they have agreed.

## Roll back to an earlier release

Each publication makes a new numbered release. If a change went wrong, make an earlier release live again:

```sh
snaphop-maps list-releases MAP_ID
snaphop-maps rollback-map MAP_ID --release 1
```

- `list-releases` lists every release, newest first, with its `number`, state and when it was published and last
  made live.
- `rollback-map` makes that release live. The page link and every embed show it within about a minute. The draft
  is left as it is, so the next `publish-map` publishes the draft, not the rolled-back release.
- The rollback is refused with `ACTIVE_RELEASE_CHANGED` if another release went live since you looked.
  `--expected-active N` names the release you expect to be live, and defaults to the one live now.

## Keep the account alive

- The account is deleted, with every map on it, after `limits.inactivityDays` days without a request (the
  `register-agent` answer holds `limits`). Any command counts, including `snaphop-maps list-maps`. Tell the user.
  If they need a map for longer, keep the `draft` from `get-map`: `create-map --args` takes it back as it is.
- The key expires at `expiresAt`. A warning `API_KEY_EXPIRING` on stderr means run `snaphop-maps replace-key`, which
  keeps the new key in place of the old one. The old key works until the new one is first used, so if the answer is
  lost, run `replace-key` again. A warning `API_KEY_EXPIRED` means it is too late: if the service refuses the key,
  the account cannot be recovered, and only a new one, `register-agent --overwrite`, will do.
- `$SNAPHOP_MAPS_API_KEY` is only sent to `$SNAPHOP_MAPS_URL`, or to https://maps.snaphop.ai when that is not set;
  `--url` alone never sends it elsewhere. Once `replace-key` has replaced the key it holds, the kept key is sent
  instead, with the warning `ENVIRONMENT_KEY_REPLACED`: tell the user to unset the variable.

## Exit statuses and errors

| Status | Meaning | What to do |
| ------ | ------- | ---------- |
| 0 | Done. | Read stdout. |
| 1 | The CLI could not do its own part, such as reading the credentials file. Nothing was sent. | Follow `hint`. |
| 2 | The command line is wrong. Nothing was sent. | Fix it as `error.message` says. |
| 3 | The service refused and changed nothing. | Act on `error.code` and `hint`. |
| 4 | The exchange failed. | If `error.outcomeKnown` is not true, the request may have been carried out. Follow `hint` before repeating. |
| 5 | The map was saved, but publishing it was refused. | stdout holds the map, with `publicationError`. |
| 6 | A new key was issued but not kept. | The key on stdout is its only copy. This once, give it to the user to keep somewhere safe, such as a password manager, and fix what `error.message` names. |
| 7 | The command did its part, but stdout could not take the answer. | `hint` says what became of it, such as whether a new key is kept. Do not repeat a change before checking it. |

Refusals you will meet:

- `MAP_INVALID`: `error.fields` names each refused field by its path, such as `markers[3].position`. Swapped
  coordinates are the usual cause.
- `MAP_NOT_FOUND`: the id is wrong or the map was withdrawn. Run `list-maps`.
- `RELEASE_UNAVAILABLE`: that release cannot be made live. Run `list-releases`. A map never published has none.
- `DRAFT_CHANGED`: run `get-map` again and redo the change.
- `TOO_MANY_REQUESTS`: wait and try again later.
- `MAP_LIMIT_REACHED`: the workspace is full. Offer to withdraw a map the user no longer needs.
- `REFUSED`: the service refused without a code of its own. `error.detail` holds what it said.
- `API_KEY_INVALID`: the key is unknown, expired or revoked. Check `snaphop-maps credentials`. If `hint` says another key is
  kept, use that one. Only if there is no newer key, register again with `--overwrite`, which starts a new account
  and replaces the kept key.

Never repeat `create-map` after a failed exchange without first running `list-maps` and looking for the map by name.
Otherwise the user may end up with two maps.

## More

- `snaphop-maps get-account` shows the account the key belongs to: its workspace, role and what the role permits.
  `snaphop-maps list-activity` lists the workspace's 50 most recent events, such as publications, rollbacks and
  withdrawals, and who did them.
- `snaphop-maps help COMMAND` explains one command, and `snaphop-maps schema` describes every command as JSON.
- `snaphop-maps guide` returns the service's own instructions and limits.
- `snaphop-maps tools` lists the service's tools, and `snaphop-maps call TOOL --args JSON` calls any of them.
- `snaphop-maps skill` prints this skill. `snaphop-maps skill install --client NAME` installs it for Claude Code,
  Codex, Gemini CLI, Grok Build, Cursor or `agents`, and `snaphop-maps skill pack` builds a zip to upload.
- `snaphop-maps ping` checks the service answers.
- Without a shell, the same service is an MCP server at https://maps.snaphop.ai/mcp.
