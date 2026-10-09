## 1. Claude adapter implementation and evidence

- [ ] 1.1 Add/finish Claude Code start and exact-resume adapter wiring in the runner provider surface with explicit supported-version/auth checks.
- [ ] 1.2 Verify bounded Claude stream parsing, interruption handling and normalized error/session output with executable tests.
- [ ] 1.3 Run real Claude execution + exact-resume continuation through the managed runner and record exact commands/evidence.

## 2. Cross-provider continuity with Claude

- [ ] 2.1 Verify two-turn prior-context continuity and normalized failures for Codex→Claude, Claude→Copilot and Copilot→Claude across independent runner processes.
- [ ] 2.2 Classify deterministic CI evidence versus real-model evidence for all Claude-including provider paths and publish the matrix in docs.

## 3. Documentation and tracker alignment

- [ ] 3.1 Update runner/client/docs language to state three-provider support only after Claude evidence is complete.
- [ ] 3.2 Re-run OpenSpec strict validation and ensure requirement mappings match the implemented artifacts.

## Requirement mapping

| Requirement | Task IDs |
| --- | --- |
| Three provider adapters | 1.1, 1.2, 1.3, 2.1, 2.2 |
| Repository examples and managed runner boundary | 3.1 |
