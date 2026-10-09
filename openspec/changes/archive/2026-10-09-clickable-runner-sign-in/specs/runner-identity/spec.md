## ADDED Requirements

### Requirement: Clickable browser sign-in output

When enrollment prints a verified browser sign-in URL, the runner SHALL show
the complete URL on its own line. Interactive output SHALL expose the exact
URL as a terminal hyperlink usable with Cmd+click in supporting macOS terminals.
Redirected output and dumb terminals SHALL receive plain text without hyperlink
control sequences. Architect and executor SHALL use the same behavior.

#### Scenario: Architect sign-in in an interactive terminal
- **GIVEN** an architect requiring enrollment in a supporting macOS terminal
- **WHEN** `make architect` prints the verified browser sign-in URL
- **THEN** Cmd+click on the link opens that exact URL, including its full query
  string, even when the visible URL wraps across terminal lines.

#### Scenario: Redirected sign-in output
- **GIVEN** enrollment output redirected to a file or pipe
- **WHEN** the runner prints the verified sign-in URL
- **THEN** the complete URL appears on its own line without terminal hyperlink
  control sequences.

#### Scenario: Dumb terminal
- **GIVEN** interactive enrollment output with `TERM=dumb`
- **WHEN** the runner prints the verified sign-in URL
- **THEN** the complete URL appears as plain text on its own line.

#### Scenario: Executor sign-in
- **GIVEN** an executor requiring enrollment in a supporting terminal
- **WHEN** `make executor` prints the verified sign-in URL
- **THEN** the executor provides the same hyperlink and plain-text behavior as
  the architect.
