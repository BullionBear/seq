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

## Rotation

If a catalog token was ever committed or pushed, scrubbing the working tree is not enough — **rotate the token at the catalog/cpanel provider**.
