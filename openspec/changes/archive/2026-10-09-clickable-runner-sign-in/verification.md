# Verification

Verified on 2026-10-09 with Go 1.27.2 on macOS arm64.

## Passed checks

- `go test ./cmd/runner ./internal/runneridentity`: passed. Enrollment tests cover
  both architect and executor with `--no-open`, exact verified URL output, and
  rejection of an untrusted login URL before printing it.
- `go test -count=1 -run TestTerminalHyperlinksForInteractiveOutput -v ./cmd/runner`
  in a controlling terminal: passed without skipping. A real terminal enables
  hyperlinks; `TERM=dumb` disables them.
- Formatter tests cover a long URL with an encoded query, exact hyperlink target
  and visible text, closing the hyperlink before the newline, and write errors.
  Redirected file, pipe, and buffer output do not enable hyperlinks.
- `go test -race ./cmd/runner ./internal/runneridentity`: passed.
- `go vet ./cmd/runner`: passed.
- `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -o <temporary-path> ./cmd/runner`:
  passed.
- `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o <temporary-path> ./cmd/runner`:
  passed.
- `git diff --check`: passed.
- `openspec validate clickable-runner-sign-in --strict`: passed.
- README instructions match the shared `--no-open` Make targets.

## Manual interaction

Actual Cmd+click in the user's terminal has not been observed. The implementation
emits OSC 8 links with the full verified URL; automated and controlling-terminal
tests verify output and terminal selection, not the emulator's mouse handling.
No production enrollment or browser sign-in was performed.
