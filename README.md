# libgen-books

Go CLI and [MCP](https://modelcontextprotocol.io) (stdio) server to **search and download books from Library Genesis**. Every live hit is cached in a local SQLite catalog, so later offline search and `download <title>` keep working.

- `libgen-books search` — live search against the `libgen.li` mirror, then cache the hits.
- `libgen-books download` — fetch by id, md5, or title through the `LIBGEN_SOURCES` chain: the anonymous `ads.php` → `get.php` handshake across racing mirrors, then Anna's Archive over IPFS gateways.
- MCP `libgen_search_books` / `libgen_download_book` — the same two operations for an agent.
- `import` — load a SQLite snapshot of the catalog for fully offline search.
- `dump` — list and download the raw dumps published by the mirror.

Built with [Amarra/Cais](https://github.com/puppe1990/amarra-cais).

## Why `libgen_books.sql` is gone

The single-file dump at `https://libgen.is/dbdump/libgen_books.sql` (and the `libgen.im` mirror) is offline. The main catalog now ships as ~1 GiB RAR parts:

```bash
libgen-books dump --list          # libgen_new-2026-10-25.part001.rar … part062.rar
libgen-books dump --part 1        # download one part into data/dumps/
```

Those parts are a MySQL dump (you need `unrar` + MySQL). For this app's local catalog, import a SQLite snapshot with a `books` table:

```bash
libgen-books import ./libgen.sqlite3   # id, title, author, year, extension, filesize, md5
```

Import matches columns by name, so snapshots with extra columns still load.

## Quick start

```bash
git clone https://github.com/puppe1990/libgenbooks.git
cd libgenbooks
cp .env.example .env      # optional
go build -o libgen-books ./cmd/cli
go build -o libgen-mcp ./cmd/mcp

./libgen-books search "clean code" --limit 5
./libgen-books download 97889355 --out ~/Downloads/art-of-clean-code.epub
```

No npm step: the CLI and MCP are pure Go.

## Web

Sign in or sign up is required. Search, covers, downloads, and download history are per account.

`GET /?q=` searches the live `libgen.li` mirror (then the local SQLite catalog), newest publication year first. Each hit shows a cover thumbnail via `GET /cover?md5=` (proxied from `ads.php`, cached on disk). `GET /download?md5=` 302s the browser to same-host `get.php`. `GET /downloads` lists that user's Baixar clicks, 10 per page (`?page=N`, `?failed=1` for failures only); `POST /downloads/delete` removes one row from that history. `GET /configuracoes` changes the account password; a successful `POST` revokes open sessions and sends the reader back to `/login`.

On boot, `LIBGEN_HISTORY_OWNER_EMAIL` + `LIBGEN_HISTORY_OWNER_PASSWORD` create the cutover account when missing and assign leftover `user_id=0` history rows to it. The password is never logged.

```bash
amarra-cais dev   # http://localhost:8080
```

## CLI

```bash
libgen-books search <title|author|isbn> [--limit N] [--local]
libgen-books download <id|md5|title> [--out path] [--url URL] [--retry N] [--retry-interval 1m]
libgen-books retry-failed [--user N] [--from list.txt] [--out dir] [--retry N] [--no-alt] [--loop] [--pause 15m]
libgen-books import [snapshot.sqlite3] [--from path]
libgen-books dump [--list] [--part N] [--out path] [--url URL]
libgen-books help
```

Example:

```
$ libgen-books search "clean code" --limit 3
ID         MD5                               TITLE                               AUTHOR              YEAR  EXT   SIZE
102864044  d3848357ae109884a140c7b3662e0e30  Clean Code / Clean coder two books  Robert c Martin     2022  pdf   10 MB
103904946  976993b85e921f0e0d0f57424e29416f  Clean Code Cookbook Recipes …       Countineri, Max…    2023  pdf   4 MB
97889355   3e219448bfb3f5f1923c4573faf75102  The Art of Clean Code …             Christian Mayer     2022  epub  2 MB
```

`search` hits the live mirror rotation (newest year first) and falls back to the local catalog if every mirror is down. `--local` forces offline.
`download` accepts an id, md5, or title (resolved against the catalog that `search` just filled). `--url` skips the handshake and fetches a direct file. `--retry N --retry-interval 1m` pauses and retries when the CDN still reports the file as not synchronized.

`retry-failed` re-tries the books that failed in the download history (`--user N`, `0` = every user) or from an `md5|file` list (`--from`). When an md5 never syncs, it falls back to a synced copy of the same title (`--no-alt` disables). `--loop --pause 15m` keeps passes coming until everything is downloaded. Files land in `--out` (default `LIBGEN_DOWNLOAD_DIR`), successful retries flip the history row, and the web history labels failed rows "Try again".

## Download sources

`download` resolves an md5 through `LIBGEN_SOURCES` (default `libgen,annas`), in order:

1. **libgen** — every mirror races the anonymous `ads.php` → `get.php` handshake in parallel; each host gets a short timeout and the first signed link wins. Search walks the same mirror rotation, so one dead frontend cannot take either down.
2. **annas** — Anna's Archive's md5 page yields the IPFS CID, public gateways race for the first bytes, and the winner serves the file.

Every finished download is hashed and checked against the requested md5; a mismatch removes the file and fails the download. A mirror that answers `200` with an HTML page is rejected the same way instead of being written to disk as if it were a book. Anna's Archive sits behind a bot check that sometimes answers with a challenge page; the provider then reports it and the chain keeps going. In the web flow, a libgen link the CDN reports as unsynced is skipped so the chain can fall through to IPFS gateways. Set `LIBGEN_SOURCES=libgen` to disable the fallback, or `LIBGEN_SOURCES=annas` to go straight to IPFS gateways.

```
$ libgen-books download 3e219448bfb3f5f1923c4573faf75102
baixado: downloads/3e219448bfb3f5f1923c4573faf75102.epub (2 MB) via libgen
```

## MCP (stdio)

```bash
go build -o bin/libgen-mcp ./cmd/mcp
```

Client config (Claude Desktop, Cursor, Grok, OpenCode):

```json
{
  "mcpServers": {
    "libgen-books": {
      "command": "/absolute/path/to/libgenbooks/bin/libgen-mcp",
      "env": {
        "LIBGEN_DB": "/absolute/path/to/libgenbooks/data/libgen.db",
        "LIBGEN_DOWNLOAD_DIR": "/absolute/path/to/downloads"
      }
    }
  }
}
```

`catalog.Open` creates the parent directory of `LIBGEN_DB` on first run, so the MCP server can start from a working directory outside the app.

Tools:

| Tool                   | Input                                | Output                                                                                                     |
| ---------------------- | ------------------------------------ | ---------------------------------------------------------------------------------------------------------- |
| `libgen_search_books`  | `query`, `limit?`                    | `books[]` (id, md5, title, author, year, extension, size), `count`, `source` (`mirror` or `local catalog`) |
| `libgen_download_book` | `ref` (id/md5/title), `url?`, `out?` | `path`, `bytes`, source `url`, `source` (provider that worked)                                             |

The server speaks JSON-RPC on stdin/stdout; logs go to stderr.

## Environment

| Variable              | Default             | Purpose                                                                         |
| --------------------- | ------------------- | ------------------------------------------------------------------------------- |
| `LIBGEN_DB`           | `./data/libgen.db`  | local SQLite catalog (cache + import)                                           |
| `LIBGEN_BASE_URL`     | `https://libgen.li` | mirror used for search/download                                                 |
| `LIBGEN_COOKIE`       | empty               | session cookie for record pages that require login (e.g. `libgen.ad/book/<id>`) |
| `LIBGEN_DOWNLOAD_DIR` | `downloads`         | destination when `--out`/`out` is omitted                                       |
| `LIBGEN_SOURCES`      | `libgen,annas`      | download providers, in preference order                                         |
| `LIBGEN_ANNAS_URL`    | empty               | primary Anna's Archive domain (empty keeps the `.gl`/`.pk`/`.gd` rotation)      |

## Stack and layout

Go 1.26, SQLite via `modernc.org/sqlite` (no CGO), MCP via `github.com/modelcontextprotocol/go-sdk`, Amarra/Cais scaffold (views, Tailwind, and the HTTP server stay in the tree).

```
cmd/cli/            → search, download, import, dump
cmd/mcp/            → stdio MCP server
internal/libgen/    → mirror client: search, record page, download handshake, dumps
internal/sources/   → provider chain (libgen → annas over ipfs)
internal/annas/     → Anna's Archive md5 page → IPFS CID
internal/ipfs/      → public gateway racing
internal/catalog/   → SQLite (books, books_fts, cids), FTS5 search, snapshot import
internal/cache/     → persist live hits in the catalog
internal/config/    → LIBGEN_* variables
internal/mcpserver/ → MCP tools
cmd/server/         → Amarra HTTP app
```

## Tests

```bash
go test ./...
```

Tests are headless: SQLite `:memory:`, real mirror HTML fixtures in `internal/libgen/testdata/`, and `httptest` servers for the download handshake. Nothing on the suite hits the network.

## Responsible use

The app indexes public metadata and fetches files from the mirror. Respect copyright and the law where you live; use it for public-domain works, open-access material, or files you have the right to download.
