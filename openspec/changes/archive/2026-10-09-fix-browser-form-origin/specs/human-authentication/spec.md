## MODIFIED Requirements

### Requirement: Logout and browser mutation protection
Logout SHALL require POST, an authenticated session, a session-bound CSRF token, and valid same-origin context; it SHALL revoke the session and expire the cookie. All browser mutations SHALL use this protection. Explicit `Origin` or `Referer` evidence SHALL match the public origin. Only when both are absent, `Sec-Fetch-Site: same-origin` MAY establish browser context; CSRF remains mandatory. GET SHALL NOT perform logout or role/membership changes.

#### Scenario: Logout revokes a cookie
- **GIVEN** a valid session and CSRF token
- **WHEN** same-origin POST logout succeeds
- **THEN** replaying its old cookie cannot access protected pages.

#### Scenario: Forged mutation
- **GIVEN** missing/incorrect CSRF data or a foreign browser origin
- **WHEN** logout or another browser mutation is attempted
- **THEN** it is rejected with HTTP 403 and state remains unchanged.

#### Scenario: Referrer-suppressed same-origin form
- **GIVEN** a valid session-bound CSRF token and a browser request with no `Origin` or `Referer`
- **WHEN** Fetch Metadata identifies the POST as same-origin
- **THEN** the mutation is evaluated normally instead of being rejected for missing referrer data.

#### Scenario: Foreign origin remains authoritative
- **GIVEN** a mutation carrying an explicit foreign `Origin` or `Referer`
- **WHEN** Fetch Metadata is absent, contradictory, or claims same-origin
- **THEN** the mutation is rejected without changing state.
