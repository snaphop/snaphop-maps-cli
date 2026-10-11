# Code review

Report verified findings first, ordered by severity: what is wrong, where, and what it breaks. State uncertainty
when evidence is missing. Whole-queue reviews follow [`review-prs`](.agents/skills/review-prs/SKILL.md).

## Review concerns, in order

1. **Keys and transport:** enforce [SECURITY.md](SECURITY.md). Keys go only to their intended service, over HTTPS
   except loopback, without redirects. Errors and warnings reveal no key. Caller-supplied `apiKey` stays an argument.
2. **Credential integrity:** private, atomic writes; bounded locking; no lost account or concurrent replacement;
   preflight saving and keep-before-print ordering; no link traversal during credential or skill writes.
3. **MCP parity:** one service operation, unchanged structured results and the same tool arguments. No service
   validation, fitted-view or publication logic copied here. Change the command table and refresh the local
   tool-list fixture when the service changes; check named commands, help and `schema` together.
4. **Agent recovery:** stable exit statuses and error codes, correct `outcomeKnown`, useful retry guidance and
   confirmation switches. Publication refusal, key-saving failure and output failure have distinct outcomes.
5. **Parsing and hostile input:** flags, positional arguments, JSON files and stdin send exactly the intended
   arguments or fail before transport. Enforce size limits and preserve plain-text map content.
6. **Structure and dependencies:** only standard-library runtime dependencies; an added dependency needs an ADR.
   Keep responsibilities in CLI, MCP transport, credentials and atomic-file packages.
7. **Tests and documentation:** cover success and refusal paths through parallel tests against the fake service.
   Mutate package state only in `cmd/snaphop-maps/main_test.go`. The statement floor is 100.0% and never drops.
   Keep requirements, traceability, design, configuration, operations, decisions, changelog and usage skill current.
8. **Public CI and releases:** GitHub-hosted runners, read-only tokens, no secrets or `pull_request_target`, pinned
   actions. No releasing workflow here; preserve the scripts used by snaphop-build-deploy. Dependabot changes
   must respect these policies and the version in `go.mod`.
9. **Diff hygiene:** no credentials, production data, generated binaries or scan reports, unrelated changes,
   test bypasses or weakened security exceptions. Keep license text intact.

Run focused checks when useful, then [`scripts/ci-local.sh`](scripts/ci-local.sh) before PR handoff. Check the
separate security workflow as [verification](docs/verification.md) describes. A Linux run cannot prove the native
macOS and Windows locking paths. Merging does not publish a binary or update an umbrella repository pointer.
