# 0005 — Keep a new key before printing it

Accepted 2026-09-30. Builds on [0004](0004-send-a-key-only-to-its-service.md).

## Context

The service shows a new key once, and it is the account's only credential. `register-agent` and `replace-key`
printed the key and then kept it. Printing can fail where keeping cannot: an agent that pipes the answer into a
command that stops reading early makes the process die of SIGPIPE before it keeps anything, and standard output on
a full disk fails without a word. The account was then registered, one of the day's registrations used, and its key
gone. A key could also be issued that the CLI had no chance of keeping, such as with a credentials directory it
cannot write or a file system that cannot lock, and it was only printed.

A key kept by `replace-key` could also go unused. With `$SNAPHOP_MAPS_API_KEY` set, every command sent the
environment's key before the kept one, so after a replacement the old key went on being sent, and the new one never
was. When the old key expired, the advice was to register again with `--overwrite`, which replaced the new key: a
documented way to lose a kept key.

## Decision

**A new key is kept before it is printed, and a command that would issue one first proves it can keep it.**

- `register-agent` and `replace-key`, unless given `--no-save`, rewrite the credentials file as it is, under its
  lock, before sending anything. If that fails, nothing is sent: `CREDENTIALS_UNWRITABLE`, exit status 1.
- The answer's key is kept, then the answer is printed. SIGPIPE is ignored, so a reader that goes away makes the
  write fail instead of ending the process.
- An answer standard output cannot take is reported as `OUTPUT_FAILED` with a new exit status, 7. Its hint says
  whether a new key is kept, and for any other tool what repeating it would do. No existing status changes meaning.
- A key that cannot be kept is still printed, as before, with `CREDENTIALS_NOT_SAVED` and exit status 6, and the
  error now comes after the answer.

**A key that the kept key replaced is not sent.** When `replace-key` keeps a new key, the account also keeps the
SHA-256 of the key it replaced (`replacedKeySha256`). A later command whose `$SNAPHOP_MAPS_API_KEY` has that digest
sends the kept key instead, and warns `ENVIRONMENT_KEY_REPLACED`. `--api-key` is still sent as given.

A refused key from the flag or the environment, while another key is kept for the service, gets a hint to leave the
flag or variable out, not to register again. An expiry warning is only given for the kept key when it is the key
sent, and `credentials` says whether it is (`sendsKeptKey`).

## Left out

- **Sending the kept key before the environment's.** It would change what a set variable means, for every
  command. Only a key known to be replaced is passed over.
- **Printing the key to standard error when standard output fails.** No output but the answer that issued a key
  may hold it, and standard error is often logged.
- **Keeping the replaced key itself.** Its digest is enough to recognise it, and the file holds no key that no
  longer works.

## Consequences

- A command that issues a key may now leave an empty credentials file behind when the service then refuses: the
  proof that the key could be kept.
- Without `--no-save`, an agent can rely on a new key being kept whenever the exit status is not 6, including 7.
