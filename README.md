# libgen-books

Mini app em Go (Amarra/Cais) para **buscar e baixar livros do Library Genesis** por CLI e por MCP (stdio). O catálogo local em SQLite guarda cada resultado encontrado, então busca offline e `download <título>` continuam funcionando depois.

- `libgen-books search` → busca ao vivo no espelho (`libgen.li`) e guarda os hits no catálogo.
- `libgen-books download` → baixa por id, md5 ou título (handshake anônimo `ads.php` → `get.php`).
- MCP `libgen_search_books` / `libgen_download_book` → as mesmas duas operações para um agente.
- `import` → carrega um snapshot SQLite do libgen para busca 100% offline.
- `dump` → lista e baixa os dumps brutos publicados pelo espelho.

## Por que não existe mais `libgen_books.sql`

O dump único `https://libgen.is/dbdump/libgen_books.sql` (e o espelho `libgen.im`) saiu do ar. Hoje o catálogo principal é publicado em partes de ~1 GiB:

```bash
libgen-books dump --list          # libgen_new-2026-10-25.part001.rar … part062.rar
libgen-books dump --part 1        # baixa uma parte para data/dumps/
```

Essas partes são RAR de um dump MySQL (precisam de `unrar` + MySQL). Para o catálogo local deste app, use um snapshot SQLite com tabela `books`:

```bash
libgen-books import ./libgen.sqlite3   # id, title, author, year, extension, filesize, md5
```

O import lê as colunas por nome, então snapshots com colunas extras também entram.

## Quick start

```bash
cd ferramentas/libgen-books
cp .env.example .env      # opcional
go build ./cmd/cli ./cmd/mcp

./cli search "clean code" --limit 5
./cli download 97889355 --out ~/Downloads/art-of-clean-code.epub
```

Sem passo de npm: o CLI e o MCP são Go puro. O servidor HTTP do scaffold continua disponível com `amarra-cais dev` (porta 8080).

## CLI

```bash
libgen-books search <título|autor|isbn> [--limit N] [--local]
libgen-books download <id|md5|título> [--out caminho] [--url URL]
libgen-books import [snapshot.sqlite3] [--from caminho]
libgen-books dump [--list] [--part N] [--out caminho] [--url URL]
libgen-books help
```

Exemplo real:

```
$ libgen-books search "clean code" --limit 3
ID         MD5                               TÍTULO                              AUTOR               ANO   EXT   TAMANHO
102864044  d3848357ae109884a140c7b3662e0e30  Clean Code / Clean coder two books  Robert c Martin     2022  pdf   10 MB
103904946  976993b85e921f0e0d0f57424e29416f  Clean Code Cookbook Recipes …       Countineri, Max…    2023  pdf   4 MB
97889355   3e219448bfb3f5f1923c4573faf75102  The Art of Clean Code …             Christian Mayer     2022  epub  2 MB
```

`search` usa o espelho ao vivo; se ele cair, cai para o catálogo local. `--local` força o offline.
`download` aceita o id, o md5 ou o título (resolvido no catálogo que o `search` acabou de preencher). `--url` pula o handshake e baixa um arquivo direto.

## MCP (stdio)

```bash
go build -o bin/libgen-mcp ./cmd/mcp
```

Config no cliente MCP (Claude Desktop, Cursor, Grok):

```json
{
  "mcpServers": {
    "libgen-books": {
      "command": "/Users/matheuspuppe/Desktop/Projetos/ferramentas/libgen-books/bin/libgen-mcp",
      "env": {
        "LIBGEN_DB": "/Users/matheuspuppe/Desktop/Projetos/ferramentas/libgen-books/data/libgen.db",
        "LIBGEN_DOWNLOAD_DIR": "/Users/matheuspuppe/Downloads/libgen"
      }
    }
  }
}
```

Ferramentas:

| Tool | Entrada | Saída |
| --- | --- | --- |
| `libgen_search_books` | `query`, `limit?` | `books[]` (id, md5, título, autor, ano, extensão, tamanho), `count`, `source` (`mirror` ou `local catalog`) |
| `libgen_download_book` | `ref` (id/md5/título), `url?`, `out?` | `path`, `bytes`, `url` de origem |

O servidor fala JSON-RPC em stdin/stdout; logs vão para stderr.

## Variáveis de ambiente

| Variável | Padrão | Para quê |
| --- | --- | --- |
| `LIBGEN_DB` | `./data/libgen.db` | catálogo SQLite local (cache + import) |
| `LIBGEN_BASE_URL` | `https://libgen.li` | espelho usado na busca/download |
| `LIBGEN_COOKIE` | vazio | cookie de sessão para record pages que exigem login (ex.: `libgen.ad/book/<id>`) |
| `LIBGEN_DOWNLOAD_DIR` | `downloads` | pasta quando `--out`/`out` é omitido |

## Stack e layout

Go 1.26, SQLite via `modernc.org/sqlite` (sem CGO), MCP via `github.com/modelcontextprotocol/go-sdk`, scaffold Amarra/Cais (views, Tailwind e servidor HTTP continuam no projeto).

```
cmd/cli/            → search, download, import, dump
cmd/mcp/            → servidor MCP stdio
internal/libgen/    → cliente do espelho: busca, record page, handshake de download, dumps
internal/catalog/   → SQLite (books, books_fts, cids), busca FTS5 e import de snapshot
internal/cache/     → grava hits ao vivo no catálogo
internal/config/    → variáveis LIBGEN_*
internal/mcpserver/ → tools MCP
cmd/server/         → app HTTP do scaffold Amarra
```

## Testes

```bash
go test ./...
```

Testes são headless: SQLite `:memory:`, HTML de fixtures reais do espelho em `internal/libgen/testdata/`, e servidores `httptest` para o handshake de download. Nenhum teste toca a rede.

## Uso responsável

O app indexa metadados públicos e baixa arquivos do espelho. Respeite direito autoral e a legislação do seu país; use para obras de domínio público, acesso aberto ou materiais que você tem direito de baixar.
