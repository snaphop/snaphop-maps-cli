# Embedding a map returned by the CLI

`create-map`, `get-map` and successful publication return the service's structured result, including the published
page link and embed codes when available. Give a person the returned `published.page` link, or copy the returned
embed code into their site. Use the service-provided addresses and runtime version instead of constructing them.
A draft or refused publication does not provide a new live embed; check `publicationError` and the exit status.

The CLI does not render, rewrite or execute embed HTML. Marker and map text stays plain text. Publishing an update
or rolling back changes the live map through the service, while rollback leaves the draft as it is. Withdrawal
requires `--yes` and takes the map down permanently.

The viewer, widget API, content security policy, runtime compatibility and delivery behavior belong to snaphop-maps.
Its `docs/embedding.md` is the detailed widget contract in the service checkout. Diagnose CLI argument and response
handling here; diagnose rendering or delivery there. A CLI upgrade alone does not update a published viewer runtime.
