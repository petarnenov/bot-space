# Repository Guidelines

## Project Structure & Module Organization

This directory currently contains no application code, tests, assets, or package configuration. Establish the layout when adding the first implementation, and update this guide to match it.

Suggested directories are `src/` for source code, `tests/` for automated tests, `assets/` for static resources, and `docs/` for supporting documentation. Create only directories the project needs. Keep related functionality together and separate generated artifacts from maintained source files.

## Build, Test, and Development Commands

No build, test, or local development commands are configured yet. When introducing a toolchain, document its installation requirements and exact commands in `README.md` and this guide.

Provide reproducible commands for dependency installation, local development, testing, linting, and production builds where applicable. Do not document commands such as `npm test` or `make build` until their corresponding configuration exists.

## Coding Style & Naming Conventions

Follow the conventions of the language and framework selected for the project. Use consistent indentation and descriptive names within each module. Configure a formatter and linter early, and commit their configuration so contributors use the same rules. Avoid introducing multiple tools that enforce conflicting styles.

## Testing Guidelines

No testing framework or coverage threshold is established. Add automated tests alongside new behavior and regression tests for bug fixes. Use descriptive test names that state the behavior being verified. Keep tests deterministic and document how to run them once a test runner is configured.

## Commit & Pull Request Guidelines

This directory is not yet a Git repository, so no commit conventions can be inferred. When version control is initialized, use concise, imperative commit subjects, such as `Add configuration loader`.

Pull requests should explain the change, its purpose, and validation performed. Link relevant issues and include screenshots for visible interface changes. Identify any setup steps or configuration changes reviewers need.

## Security & Configuration

Keep credentials and secrets out of source control. If environment variables are introduced, provide an example configuration with placeholder values and ignore local secret files and generated outputs.
