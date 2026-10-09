## 1. Make entrypoints

- [x] 1.1 Add a root Makefile with compose, verification, OpenSpec, and Railway
      wrapper targets, plus a discoverable `help` target.
- [x] 1.2 Add local test-database lifecycle targets and package-scoped test
      targets for focused validation.

## 2. Documentation alignment

- [x] 2.1 Update README command examples to use make targets as primary
      invocation path where equivalents exist.
- [x] 2.2 Preserve command semantics and environment-variable expectations.

## 3. Validation

- [x] 3.1 Validate the active follow-up change through the new make wrappers
      (`make openspec-apply` / `make openspec-validate-change`).
- [x] 3.2 Execute representative runner tests via make wrappers and clean up
      temporary test database container lifecycle.

## Requirement mapping

| Requirement | Task IDs |
| --- | --- |
| Tooling/docs normalization through Make entrypoints | 1.1, 1.2, 2.1, 2.2, 3.1, 3.2 |
