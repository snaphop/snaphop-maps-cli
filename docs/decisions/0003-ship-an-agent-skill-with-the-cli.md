# 0003 — Ship an Agent Skill with the CLI

Accepted 2026-09-30. Builds on [0001](0001-call-the-mcp-server-for-parity.md).

## Context

An assistant that has never seen `snaphop-maps` has to discover it before using it. It needs to know when a map
would help, that positions are longitude first, that `withdraw-map` cannot be undone, that the key is a secret, and
what each exit status means. `help` and `schema` answer these questions once the assistant thinks to ask them.

Agent Skills (<https://agentskills.io>) is an open standard for this: a `SKILL.md` with a name and description,
which a client loads only when a task matches. Claude, ChatGPT and OpenAI Codex, Gemini CLI, Grok Build, Cursor,
GitHub Copilot and others read the same file. They differ only in the directory they load it from, or, for
claude.ai, ChatGPT and the model APIs, in taking it as a zip upload.

## Decision

**One skill, `skills/snaphop-maps/SKILL.md`, shipped in the repository and in the binary.**

- **One file for every assistant.** No per-vendor variants to drift apart. The skill is written for any assistant
  that can run a shell.
- **In the binary.** Package `skills` embeds the directory. `snaphop-maps skill` prints it, so an assistant can
  learn the CLI in one step and always reads the skill that matches the version it runs.
- **Installed where each client looks.** `skill install --client` writes it under the home directory, or under
  `--project`, at `.claude/skills` (Claude Code), `.agents/skills` (Codex, and the shared directory Gemini CLI and
  Grok Build also read), `.gemini/skills` (Gemini CLI), `.grok/skills` (Grok Build) or `.cursor/skills` (Cursor).
  It replaces its own earlier copy and touches nothing else.
- **Packed for upload.** `skill pack` writes a zip holding the skill's one directory. The same skill always packs
  to the same bytes. Each release attaches it, with its checksum (ADR 0007).
- **Tested like code.** Tests hold the skill to the specification's frontmatter rules, require it to show every
  command, and run every example in its code blocks through the CLI.

## Left out

- **Per-vendor formats** such as a custom GPT's instructions or a Gemini CLI extension: each would be a second copy
  of the same guidance. The standard reaches them all.
- **Guessing the client.** `skill install` needs `--client`: an assistant knows which one it is, and writing to
  every directory would install the skill where nobody asked for it.

## Consequences

- A change an assistant relies on also changes `SKILL.md`, in the same pull request. The example tests fail if a
  command or flag in it stops working.
- The skill is only useful to an assistant that can run the binary. The MCP server remains the way in for one
  without a shell.
