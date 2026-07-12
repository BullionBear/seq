# Sample configs

Scenario YAML under this directory is safe to commit. **Never** put live catalog tokens, venue API keys, or other credentials in tracked files.

## Supplying credentials

### Option A — environment variable (preferred for `catalog.api_token`)

Sample configs use `${CATALOG_API_TOKEN}`. `LoadConfig` expands `${VAR}` / `$VAR` in `catalog.api_token` and, if still empty, fills it from `CATALOG_API_TOKEN`:

```bash
export CATALOG_API_TOKEN='your-token-here'
./bin/seq -c config/obtest.yml
```

### Option B — gitignored local override

```bash
cp config/obtest.yml config/obtest.local.yml
# Edit catalog.api_token (and any other secrets) in the .local.yml copy
./bin/seq -c config/obtest.local.yml
```

`config/*.local.yml` is gitignored. Keep private copies out of git.

## Mint / rotate a catalog API token (cpanel)

Provider endpoints (production):

- API: `https://capi.lynxlinkage.com` (`GET /health` → `{"status":"healthy"}`)
- UI: `https://cpanel.lynxlinkage.com` (Authelia-gated)

Operator mint path (do **not** paste token values into tickets or commits):

1. Sign in at `https://cpanel.lynxlinkage.com` via Authelia, then complete the cpanel UI session so the browser holds a `ui_` JWT (Authelia-gated `GET /api/v2/auth/session` exchanges the session for that JWT).
2. From the authenticated UI (or with `Authorization: Bearer ui_…`), call `POST /api/v2/auth/api-token`. Response is a long-lived `api_…` bearer token (one per user; POST replaces any existing API token).
3. Supply it only via env or a gitignored local override:

```bash
export CATALOG_API_TOKEN='api_…'   # never commit
# or edit catalog.api_token in config/*.local.yml
```

4. Confirm historical/revoked tokens stay gone unless intentionally re-minted (`access.access_token LIKE 'api_%'` for the operator user).

## Rotation

If a catalog token was ever committed or pushed, scrubbing the working tree is not enough — **rotate the token at the catalog/cpanel provider** using the mint path above, then update `CATALOG_API_TOKEN` / `*.local.yml` only.
