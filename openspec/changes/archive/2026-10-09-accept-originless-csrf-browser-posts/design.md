# Design

## Context

See proposal.md. Browser mutation middleware currently requires a recognized origin signal before it parses and validates the CSRF token. Production evidence shows that valid form submissions can arrive without any recognized signal.

## Goals / Non-Goals

**Goals:**

- Support authenticated HTML form POSTs from browsers that omit origin metadata.
- Preserve rejection of explicit cross-origin evidence.
- Preserve session-bound CSRF validation for every mutation.

**Non-Goals:**

- Relax authentication, project authorization, or CSRF validation.
- Trust forwarding headers as browser evidence.
- Change non-browser APIs.

## Decisions

Treat origin validation as a rejection check for explicit evidence. A supplied `Origin` or `Referer` must match the configured public origin. If neither is supplied, continue to form parsing and CSRF validation instead of requiring Fetch Metadata.

The synchronizer token remains bound to the authenticated server-side session, while the session cookie remains `Secure`, `HttpOnly`, and `SameSite=Lax`. A cross-site page cannot read the token, and an explicit foreign origin is still rejected.

## Risks / Trade-offs

- [Clients outside browsers can omit origin metadata] → They still require both a valid browser session cookie and its unpredictable CSRF token.
- [A same-site hostile page can send the session cookie] → It cannot read the session-bound CSRF token under the same-origin policy.

## Migration Plan

Deploy the middleware-only change after focused HTTP and full repository verification. Roll back to the previous commit if valid submissions or foreign-origin rejection regress.
