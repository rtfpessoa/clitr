# clitr - Trade Republic CLI

> **Work in Progress**: This is an unofficial side project and is not affiliated with, endorsed by, or associated with Trade Republic Bank GmbH. Use at your own risk.

A Go CLI tool to export transaction history from [Trade Republic](https://traderepublic.com/) to CSV format.

<p align="center">
  <img src="assets/clitr-banner.png" alt="clitr banner">
</p>

## Features

- **Three-Step Workflow**: Fetch raw data, export to CSV, and self-heal structs when the API changes
- **Web Login Authentication**: Uses Trade Republic's web login flow with 2FA verification
- **Session Persistence**: Optionally save session cookies to avoid re-authenticating every time
- **Incremental Fetching**: Fetch only new transactions since last run
- **CSV Export**: Export transactions to a semicolon-delimited CSV file
- **Transaction Types**: Supports Buy, Sell, Deposit, Removal, Interest, Dividend, Tax Refund, and Saveback transactions
- **Struct Self-Healing**: Detects unknown API fields and auto-patches Go struct definitions
- **Web UI**: Browser-based interface with login, real-time progress, and sortable table view
- **Configurable**: Custom data directory and output paths

## Installation

**Requirements**: Go 1.25 or later

```bash
go install github.com/rtfpessoa/clitr@latest
```

Or build from source:

```bash
git clone https://github.com/rtfpessoa/clitr
cd clitr
go build -o clitr .
```

## Quick Start

```bash
# Step 1: Fetch transactions from Trade Republic
clitr fetch --phone "+4912345678"
# You'll be prompted for your PIN and 2FA code

# Step 2: Export to CSV
clitr export

# Or use the web UI instead
clitr serve
# Open http://localhost:8080 in your browser
```

## Usage

### Fetch Command

Downloads transaction data from Trade Republic and saves it as JSON files.

```bash
# Basic fetch - prompts for PIN and 2FA
clitr fetch --phone "+4912345678"

# Save session for future use (avoids re-authenticating)
clitr fetch --phone "+4912345678" --save-credentials

# Fetch only new transactions (incremental update)
clitr fetch --phone "+4912345678" --incremental

# Reset everything (credentials + data) and fetch fresh
clitr fetch --phone "+4912345678" --reset

# Reset only credentials (clear saved session from keyring)
clitr fetch --phone "+4912345678" --reset-credentials

# Reset only data (delete transaction files)
clitr fetch --phone "+4912345678" --reset-data
```

**Flags:**
| Flag | Short | Description |
|------|-------|-------------|
| `--phone` | | Phone number in international format (e.g., `+4912345678`) |
| `--save-credentials` | `-s` | Save session cookies for future use |
| `--incremental` | `-i` | Fetch only new transactions |
| `--reset` | `-r` | Reset all: clear credentials and delete transaction data |
| `--reset-credentials` | | Clear saved credentials from system keyring |
| `--reset-data` | | Delete transaction data from disk |

### Serve Command

Starts a local web server with a browser-based UI for fetching and exporting transactions.

```bash
# Start web server on default port (8080)
clitr serve

# Use a custom port and host
clitr serve --port 3000 --host 127.0.0.1

# Trust X-Forwarded-For header (when behind a reverse proxy)
clitr serve --trusted-proxy
```

The web UI provides:
- **Login form** with phone number and PIN input
- **2FA verification** with countdown timer
- **Real-time progress** via Server-Sent Events (SSE)
- **CSV result page** with toggle between raw CSV and sortable table view
- **Copy to clipboard** for easy export

**Flags:**
| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--port` | `-P` | Port to listen on | `8080` |
| `--host` | | Host to bind to | `0.0.0.0` |
| `--trusted-proxy` | | Trust X-Forwarded-For header | `false` |

### Export Command

Converts fetched JSON data to CSV format.

```bash
# Export to default file (transactions.csv)
clitr export

# Export to a specific file
clitr export --output my-transactions.csv

# Export without chronological sorting
clitr export --sort=false
```

**Flags:**
| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--output` | `-o` | Output CSV file path | `transactions.csv` |
| `--sort` | `-s` | Sort transactions by date | `true` |

### Patch Command

Scans saved JSON event files for fields not yet defined in the Go struct types. Useful for keeping the codebase in sync when Trade Republic's API evolves.

```bash
# Detect unknown fields (dry run)
clitr patch

# Automatically patch Go source files with new fields
clitr patch --apply
```

**Flags:**
| Flag | Description |
|------|-------------|
| `--apply` | Apply patches directly to Go source files |

### Global Flags

```bash
# Enable verbose/debug logging
clitr -v fetch --phone "+4912345678"

# Use a custom data directory
clitr --data-dir /path/to/data fetch --phone "+4912345678"
```

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--verbose` | `-v` | Enable debug logging | `false` |
| `--data-dir` | `-d` | Directory to store data | `~/.local/share/clitr` |

## How It Works

1. **Fetch**: Authenticates with Trade Republic using phone + PIN + 2FA, then downloads all transactions via their WebSocket API. Raw data is saved as individual JSON files in `~/.local/share/clitr/events/`.

2. **Export**: Reads the JSON files, parses transaction details (type, ISIN, shares, fees, etc.), and exports to CSV.

3. **Patch**: Scans the saved JSON files against the Go struct definitions, detects any new fields from API changes, infers their Go types, and optionally patches the source code automatically.

4. **Serve**: Launches a local web server that provides a browser-based UI for the full fetch-and-export workflow, with real-time progress updates via SSE.

This multi-step approach means you can:
- Re-export with different options without re-fetching
- Keep raw data for debugging or future format changes
- Use incremental fetching to update only new transactions
- Self-heal the codebase when the API evolves

## CSV Format

The exported CSV uses semicolon (`;`) as delimiter and contains:

| Column | Description | Example |
|--------|-------------|---------|
| Date | Transaction date (RFC3339) | `2024-01-15T14:30:00+01:00` |
| Type | Transaction type | `BUY`, `SELL`, `DEPOSIT`, `REMOVAL`, `INTEREST`, `DIVIDEND`, `TAX_REFUND` |
| Value | Amount in EUR | `150.50` |
| Note | Title and description | `Apple Inc. \| Buy Order` |
| ISIN | Securities identifier | `US0378331005` |
| Shares | Number of shares | `10.50` |
| Fees | Transaction fees (negated) | `1.00` |
| Taxes | Taxes paid (negated) | `5.25` |

**Notes:**
- Fees and taxes are shown as positive values (they reduce your transaction value)
- Saveback transactions generate two rows: a BUY and a DEPOSIT
- Canceled transactions are automatically excluded

## Security

- **2FA Required**: Every login requires verification via Trade Republic app or SMS
- **In-Memory by Default**: Credentials are kept in memory only during execution
- **Secure Cookie Storage**: When using `--save-credentials`, session cookies are stored in your system's secure keychain:
  - **macOS**: Keychain Services (hardware-backed, Touch ID support)
  - **Linux**: Secret Service API (GNOME Keyring, KWallet)
  - **Windows**: Windows Credential Manager
- **No Password Storage**: Your PIN is never stored on disk
- **Automatic Migration**: Existing plaintext cookie files are automatically migrated to the system keyring
- **Web UI Hardening** (when using `serve`):
  - Content Security Policy (CSP) with per-request nonce
  - CSRF token protection on all POST requests
  - Rate limiting (5 POST requests per IP per minute)
  - Request body size limits (4 KB)
  - Security headers: `X-Frame-Options: DENY`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`

## Data Storage

Default location: `~/.local/share/clitr/`

```
~/.local/share/clitr/
└── events/
    ├── metadata.json           # Fetch state and cursors
    └── <date>_<id>.json        # Individual transaction files
```

Session cookies are stored in your system keyring, not on the filesystem.

## Troubleshooting

### "Login failed" error
- Verify phone number is in international format (e.g., `+4912345678`)
- Ensure PIN is correct (4 digits)
- Check that you're receiving 2FA codes in your Trade Republic app

### "Session expired" error
- Your saved session has expired, re-authenticate with `--save-credentials`
- Or run without `--save-credentials` to authenticate fresh each time

### Missing transactions
- Some very old transactions may not be available via the API
- Use `--reset` to fetch all available transactions from scratch
- Download official PDFs from Trade Republic for complete historical records

## Project Structure

```
clitr/
├── main.go                    # CLI entry point
├── fetch.go                   # Fetch command
├── export.go                  # Export command
├── patch.go                   # Patch command (struct self-healing)
├── serve.go                   # Serve command (web server)
├── internal/
│   ├── client/                # Trade Republic API client
│   │   ├── auth.go            # Authentication logic
│   │   ├── client.go          # WebSocket client and subscriptions
│   │   └── interfaces.go      # Interfaces for testing
│   ├── export/                # Output formatters
│   │   ├── csv.go             # CSV exporter
│   │   └── parse.go           # Raw event parsing
│   ├── fetch/                 # Fetch pipeline
│   │   └── fetch.go           # Pagination and incremental fetching
│   ├── json/                  # JSON utilities
│   │   └── utils.go           # Strict JSON marshal/unmarshal
│   ├── log/                   # Logging
│   │   └── logger.go          # Zap logger wrapper
│   ├── patch/                 # Struct self-healing
│   │   ├── apply.go           # Source file patching
│   │   ├── detect.go          # Unknown field detection
│   │   ├── infer.go           # Go type inference from JSON
│   │   └── types.go           # Patch types
│   ├── types/                 # Data models
│   │   ├── event.go           # Event types and parsing
│   │   └── raw.go             # Raw API response types
│   ├── utils/                 # Shared utilities
│   │   └── file.go            # Path resolution
│   └── web/                   # Web server
│       ├── server.go          # HTTP server, routes, middleware chain
│       ├── handlers.go        # Request handlers (login, 2FA, progress, result)
│       ├── middleware.go       # Security headers, body size limits
│       ├── session.go         # In-memory session store with TTL
│       ├── csrf.go            # CSRF token generation/validation
│       ├── ratelimit.go       # Per-IP sliding window rate limiter
│       └── templates/         # Embedded HTML templates
│           ├── base.html      # Base layout with responsive CSS
│           ├── login.html     # Phone + PIN login form
│           ├── twofa.html     # 2FA verification
│           ├── progress.html  # Real-time progress (SSE)
│           ├── result.html    # CSV display with sortable table
│           └── error.html     # Error page
└── go.mod
```

## See Also

Similar projects for Trade Republic:
- [pytr](https://pypi.org/project/pytr/) - Python library with CLI for Trade Republic
- [TradeRepublicApi](https://github.com/Zarathustra2/TradeRepublicApi) - Unofficial Python API

## Disclaimer

This is an **unofficial** tool and is **not affiliated** with, endorsed by, or associated with Trade Republic Bank GmbH.

- **Use at your own risk**
- Trade Republic's private API may change without notice, breaking this tool
- No warranty or support is provided
- This tool accesses Trade Republic's unofficial API which may violate their Terms of Service

## License

MIT License

## Contributing

Contributions are welcome! Please open an issue or pull request.
