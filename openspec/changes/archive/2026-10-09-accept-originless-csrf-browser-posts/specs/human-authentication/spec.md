## MODIFIED Requirements

### Requirement: Logout and browser mutation protection
Logout SHALL require POST, an authenticated session, and a session-bound CSRF token; it SHALL revoke the session and expire the cookie. All browser mutations SHALL use this protection. Explicit `Origin` or `Referer` evidence SHALL match the public origin. When both are absent, the valid CSRF token SHALL provide mutation proof, whether Fetch Metadata is same-origin or unavailable. GET SHALL NOT perform logout or role/membership changes.

#### Scenario: Logout revokes a cookie
- **GIVEN** a valid session and CSRF token
- **WHEN** same-origin POST logout succeeds
- **THEN** replaying its old cookie cannot access protected pages.

#### Scenario: Forged mutation
- **GIVEN** missing or incorrect CSRF data
- **WHEN** logout or another browser mutation is attempted
- **THEN** it is rejected with HTTP 403 and state remains unchanged.

#### Scenario: Referrer-suppressed same-origin form
- **GIVEN** a valid session-bound CSRF token and a browser request with no `Origin` or `Referer`
- **WHEN** Fetch Metadata is same-origin or unavailable
- **THEN** the mutation is evaluated normally instead of being rejected for missing browser metadata.

#### Scenario: Foreign origin remains authoritative
- **GIVEN** a mutation carrying an explicit foreign `Origin` or `Referer`
- **WHEN** Fetch Metadata is absent, contradictory, or claims same-origin
- **THEN** the mutation is rejected without changing state.
