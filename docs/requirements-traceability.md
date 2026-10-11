# Requirements traceability

These tests provide evidence for [REQUIREMENTS.md](../REQUIREMENTS.md). Test names are Go functions, grouped by
owning test file; this table is maintained documentation, not an additional coverage gate.

| Requirement | Evidence |
| --- | --- |
| CLI-01: tool and table parity | `internal/cli/tools_test.go`: `TestEveryToolIsACommand`, `TestCommandsCallTheirTools`; `internal/cli/account_test.go`: `TestSchemaDescribesEveryCommand` |
| CLI-02: stateless transport and service result | `internal/mcp/client_test.go`: `TestCallToolSendsOneStatelessRequest`, `TestCallToolReportsTheToolsRefusal`; `internal/cli/tools_test.go`: `TestAnInvitationIsPrintedAsItCame`, `TestRPCCommands` |
| CLI-03: JSON, diagnostics and outcomes | `internal/cli/tools_test.go`: `TestOutputIsOneLineOrIndented`, `TestStandardErrorIsOneDocumentPerLine`, `TestFailedExchangesSayWhetherToRepeat`, `TestARefusedPublicationKeepsTheDraft`; `internal/cli/account_test.go`: `TestAnAnswerThatCannotBePrintedIsReported` |
| CLI-04: parsing and confirmation | `internal/cli/tools_test.go`: `TestJSONArgumentsFromFilesAndStandardInput`, `TestLocalRefusalsSendNothing`, `TestCommandLinesThatWouldSendTheWrongThing`, `TestEveryDestructiveCommandNeedsConfirming` |
| CLI-05: usage skill and offline commands | `internal/cli/skill_test.go`: `TestTheSkillFollowsTheAgentSkillsSpecification`, `TestTheSkillNamesEveryCommand`, `TestEveryExampleInTheSkillRuns`, `TestSkillInstallNeverWritesThroughALink`, `TestSkillPackMakesAnUploadableZip`; `internal/cli/tools_test.go`: `TestLocalCommandsNeedNoService` |
| CLI-06: service-bound secrets and transport | `internal/cli/tools_test.go`: `TestKeySources`, `TestAnEnvironmentKeyOnlyGoesToItsService`, `TestAKeptKeyIsOnlySentToItsService`, `TestNoErrorEchoesAKey`, `TestServiceAddresses`, `TestRedirectsAreNotFollowed` |
| CLI-07: durable, concurrent credential storage | `internal/credentials/store_test.go`: `TestPutKeepsEveryServiceApartAndPrivate`, `TestCommandsKeepingKeysAtOnceLoseNone`, `TestFieldsOfANewerBuildSurvive`; `internal/cli/account_test.go`: `TestANewKeyIsKeptBeforeItIsPrinted`, `TestRegisterAgentWillNotLoseAKeptAccount`, `TestReplaceKeyNeverOverwritesAnotherAccount` |
| CLI-08: dependency, coverage and platform builds | `go.mod`, `Makefile`, `scripts/coverage.sh`, `scripts/ci-local.sh`, `.github/workflows/verify.yml`; `cmd/snaphop-maps/main_test.go`: `TestMainExitsWithTheStatus` |
| CLI-09: public CI and release controls | `.github/workflows/verify.yml`, pinned actions and `security/test_check.py`; release validation scripts and ADR 0008; maintainer authorization remains a human control |

All Go statements must remain covered. The current platform's compiled files determine local coverage;
[verification](verification.md) describes the additional native CI runs and security checks. Service contracts are
owned and tested in snaphop-maps, not duplicated here.
