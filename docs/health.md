# Health endpoint

The API serves unauthenticated `GET /healthz`. It checks the local management
database with a read-only query and a one-second deadline.
Success returns `200` and `{"status":"ok"}`. A database failure returns
`503` and `{"status":"unavailable"}`. Both responses use
`Cache-Control: no-store`.

The probe does not verify provider credentials, dispatch providers, or
record usage or audit events. Successful probes produce no routine request
events. Failed probes retain error evidence.

Local startup and production readiness use `/healthz`. Capability and
configuration endpoints remain available for their application functions.
Authentication and frontend integration checks remain in their test suites.
Docker probes keep failure output. They use a one-second startup interval,
a 30-second steady interval, and a 30-second startup period.

The site renderer copies `site/healthz` into the publication artifact.
The local frontend already applies the no-store header to all resources.
GitHub Pages controls production response headers. I239 keeps the production
cache requirement open until a hosting decision is made.
