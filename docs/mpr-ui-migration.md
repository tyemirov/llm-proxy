# Shared UI Migration

I260 prepares llm-proxy for the mpr-ui I009 publication. B206 corrects the
application transition after shared session recovery.

## Candidate

The tests use shared revision
`768f25936497c5aabd426197d21c2100b6e5d9a1` from mpr-ui B069.
The production asset URLs retain literal `@latest`.
The test helper verifies each candidate file before browser use.

| File | SHA-256 |
| --- | --- |
| `mpr-ui-config.js` | `3f56fbd212a516d2bd8b0b95f73ae7ad82952c10d8d5f4e6f8b44d3233f01304` |
| `mpr-ui.js` | `3e725dbe911470ca934cb46456369479b6ac232eee5ccba2582bf8d939259ae8` |
| `mpr-ui.css` | `351bbf6c15054528a651571d8c8bd85536eea76c3e574f9335e6cd413878923f` |

## Application Contract

The API and tracked static config use `auth.providers`.
Google retains the configured client ID, login path, and nonce path.
Apple and password providers are disabled. The session path is `/auth/session`.
The API response retains `Cache-Control: no-store`.

The shared footer generator and all 52 generated pages use the sectioned `menu`.
The menu retains the project labels and destinations.
The menu button contains the attribution. The separate default prefix is hidden.
The compact mobile footer retains its height and width limits.

Protected management requests use `MPRUI.authenticatedFetch` with the header.
The backend authorizes requests before domain work. The client declares
`mutationReplay: "authorization-before-domain-work"` for that contract.
After recovery, the application sends `llm-proxy:management-ready` to clear
the transition and preserve access to the user menu.

## Local Evidence

The HTTP regression failed against both flat producers before the changes.
The changed producers passed `make test-shared-ui-config`.
The browser regression failed before the footer and transport changes.
A later regression reproduced the B206 transition failure after recovery.

The browser tests use the real Go CLI, TAuth service, SQLite databases,
application modules, and candidate shared assets.
The Google fixture controls the external credential response.
TAuth password login issues real local session cookies at that boundary.
These tests do not establish real Google acceptance.

The four migration scenarios cover frontend and direct TAuth origins at
390px and 1280px. Each scenario verifies login, restoration, read recovery,
mutation recovery, one persisted mutation, logout, and the project menu.
All four scenarios and the existing management scenario passed.
Final `make ci` passed all 12 gates in 345 seconds.
The run passed 114 application browser tests, six TAuth/MCP scenarios,
Python client checks, the Pages artifact check, and 100 percent Go coverage.
The complete log is `/tmp/llm-proxy-i260-ci-final3.log`.

## Public Evidence And Activation

The [public observations](mpr-ui-public-assets-2026-09-09.json) record seven
HTTP 200 responses from one network location on September 9, 2026.
The observations include the website, application, release marker, API config,
and three published shared assets. Published asset digests differ from the
candidate digests above.

The website responses declare a 600-second cache lifetime.
The shared CDN responses declare `max-age=604800, s-maxage=43200`.
HTTP success does not establish browser authentication or cache convergence.

1. Qualify all I009 consumers against one final shared candidate.
2. Record the final consumer commit and native CI result.
3. Use the central I009 maintenance and cache plan for coordinated publication.
4. Have the operator publish the shared assets, API config producer, and Pages artifact.
5. Verify the release marker and public bytes from each required network location.
6. Verify real Google login, restoration, protected requests, and logout in the production browser.

Shared publication, deployment, cache convergence, and production acceptance
remain separate gates.
