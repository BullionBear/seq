# Sample configs

Scenario YAML under this directory is safe to commit. **Never** put venue API keys, secrets, or other credentials in tracked files.

## Instruments

Market data metadata (symbols, exchanges, products, tokens, precisions) loads from a local JSON file referenced by `catalog.instruments`. A relative path is resolved against the config file's directory, so sample configs use:

```yaml
catalog:
  instruments: ./instruments.json
```

See [`instruments.json`](instruments.json) for the expected shape. Add an entry per tradable symbol; `exchange.id` must match the internal exchange IDs (Binance=1, Bybit=2) and `product.id` the internal product IDs (SPOT=1).

## Accounts and credentials

Accounts, wallets, and API keys are defined under `catalog.accounts`:

```yaml
catalog:
  instruments: ./instruments.json
  accounts:
    - name: bybit-hephe
      exchange: Bybit            # Binance | Bybit
      api_keys:
        - name: bybit-hephe-hmac
          type: HMAC             # HMAC | RSA | ED25519
          key: ${BYBIT_HEPHE_API_KEY}
          secret: ${BYBIT_HEPHE_API_SECRET}
      wallets:
        - name: bybit-hephe-unified
          type: unified          # spot | umargin | cmargin | leverage | unified
```

`execrouter` entries reference these by name (`account`, `wallet`, `api`).

### Option A — environment variables (preferred)

`LoadConfig` expands `${VAR}` / `$VAR` placeholders in each API key's `key`, `secret`, and `passphrase` fields:

```bash
export BYBIT_HEPHE_API_KEY='...' BYBIT_HEPHE_API_SECRET='...'
./bin/seq -c config/xarb.yml
```

### Option B — gitignored local override

```bash
cp config/xarb.yml config/xarb.local.yml
# Edit the api_keys key/secret values in the .local.yml copy
./bin/seq -c config/xarb.local.yml
```

`config/*.local.yml` is gitignored. Keep private copies out of git.

## Rotation

If a venue API key was ever committed or pushed, scrubbing the working tree is not enough — **rotate the key at the venue**, then update the environment variables / `*.local.yml` only.
