# Sample configs

Scenario YAML under this directory is safe to commit. **Never** put venue API keys, secrets, or other credentials in tracked files.

## Logger and msglog

Both use the same `file:` rotation block (`core/logger/rotate.FileConfig`):

```yaml
logger:
  level: debug
  stdout: true
  file:
    dir: ./logs
    name: seq              # -> seq_<YYYY-MM-DD>.log (+ .N on size roll)
    max_bytes: 10485760
    daily: true
    max_backups: 5
    max_age_days: 0
    sync: rotate           # none | rotate | periodic | each

msgbus:
  msglog:
    enabled: true
    file:
      dir: ./logs
      name: msg             # -> msg_<YYYY-MM-DD>.jsonl (events + commands)
      max_bytes: 104857600
      daily: true
      max_backups: 10
      max_age_days: 7
      sync: rotate
```

Omit `logger.file.dir` (or leave it empty) to disable text-file logging. Msglog never writes to stdout.

## Instruments

Market data metadata (symbols, exchanges, products, tokens, precisions) loads from a local JSON file referenced by `catalog.instruments`. A relative path is resolved against the config file's directory, so sample configs use:

```yaml
catalog:
  instruments: ./instruments.json
```

See [`instruments.json`](instruments.json) for the expected shape. Add an entry per tradable symbol; `exchange.id` must match the internal exchange IDs (Binance=1, Bybit=2) and `product.id` the internal product IDs (SPOT=1, PERPETUAL=2). Binance USD-M futures instruments use product PERPETUAL (e.g. `BINANCE_PERPETUAL_ETHUSDT`) and the [`adapter/binancefutures`](../adapter/binancefutures/BINANCE_FUTURES.md) clients.

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
