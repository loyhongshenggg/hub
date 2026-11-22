# Finance

Goals, spending insights and credit-card statement import for Singapore. Design, decisions and sources: [docs/finance/design.md](../../docs/finance/design.md).

```bash
pnpm nx run finance:validate          # build, start, health-check, tear down
pnpm nx run finance:docker-build      # rebuild the image after changes, then Restart in the hub
pnpm nx test finance-api              # Go tests (synthetic data only)
```

Dev mode (app menu → _Start in dev mode_) runs the API and Vite on the host. Statement import then needs `pdftoppm` and `tesseract` on the host: `sudo apt install poppler-utils tesseract-ocr` (also in `tools/setup/wsl-toolchain.sh`). The container image already has them.

## Layout

| Path                         | What                                                                                                          |
| ---------------------------- | ------------------------------------------------------------------------------------------------------------- |
| `api/internal/ocr`           | PDF → page PNGs (`pdftoppm`) → words with boxes (Tesseract TSV)                                               |
| `api/internal/statement`     | Lines → rows + account summary; reconcile against the statement's totals; extra OCR passes fill gaps         |
| `api/internal/merchant`      | Descriptor cleaning, SG brands (`brands.yaml`), OSM + ACRA directory, fuzzy matching for truncated names     |
| `api/internal/insights`      | Monthly overview, category/merchant breakdowns, 12-month trend, recurring charges, insight cards             |
| `api/internal/goals`         | Goal kinds (target, payoff, limit, habit): progress, forecast, status, streaks, milestones                   |
| `api/internal/claude`        | Optional: paragraph → goal drafts (structured output), unknown merchant lookup (web search); spend cap      |
| `api/internal/split`         | Shared costs: equal / shares / exact splits in whole cents (largest remainder); your share = amount − others' |
| `api/internal/importer`      | Background jobs: statement reads, directory refresh, Claude lookups                                           |
| `api/internal/store`         | SQLite (`$DATA_DIR/finance.db`); uploaded PDFs in `$DATA_DIR/statements/`                                     |
| `web/src`                    | React UI: Overview, Goals, Transactions, Import, Settings                                                     |

## Testing with real statements

Real statements hold personal data and are never committed. To check the parser against your own PDFs:

```bash
FINANCE_TEST_PDFS=/path/to/folder go test ./internal/statement -run TestRealStatements -v   # in apps/finance/api
```

## Claude (optional)

Nothing needs it: goal paragraphs are read locally (`api/internal/goals/words.go`), merchants are matched against the local directory. With a key (Settings, stored in the app's database, or `ANTHROPIC_API_KEY` via `apps/finance/.env`, git-ignored) and a feature switched on, Claude reads looser goal paragraphs and identifies merchants the directory can't, with a monthly cap (default US$5). Only goal paragraphs (plus optional category averages) and unknown merchant descriptors are sent.

## Adding a bank

Supported: HSBC Singapore (image-only PDFs → OCR) and Citibank Singapore (text PDFs → exact text) credit cards; OCBC and Trust bank accounts (text PDFs, added 2026-11-22). `statement.Parse` handles any statement laid out as `DD Mon [DD Mon] description amount` under a `DESCRIPTION … AMOUNT` header, with totals as labelled values (_Previous balance_ / _Payments & credits_ / _Purchases & debits_) or a boxed row (`boxSummary`). For another layout, add its bank name to `banks` and its labels in `parseSummary`, then test with `FINANCE_TEST_PDFS`.

**Bank accounts.** A page with a short header row holding *Date*, *Withdrawal/Debit*, *Deposit/Credit* and *Balance* is a ledger (`statement/ledger.go`): columns come from the header, a row starts with a date and runs to the next, *BALANCE B/F* / *C/F* (or *Brought/Carried Forward*) are the opening and closing balances, and every printed balance is checked against the one before, so no printed totals are needed. Trust prints card-style rows with `+` for money in, an FCY column and a one-row balance table (opening + in − out + interest = closing). `classify` (`statement/bankrow.go`) types each row from what the bank prints: your own name (also masked, `LOX HOXX SHEXX`) → transfer; *SALA* GIRO, interest, *GOV* GIRO → income; AXS → card payment (flagged to check); PayNow-UEN / QR / NETS → purchase matched on the payee; PayNow-Mobile, account numbers, masked names → transfer to a person, with the message kept as a note; other money in → transfer (flagged), never a refund that would lower spending. DBS/POSB print the same ledger layout, but no sample was available to test them (2026-11-22).

Plain tables (date · name · amount) also read: numeric dates (`31/10/2026`, `2026-10-31`, `31-Oct-26`, `Oct 31, 2026`), `S$` that OCR reads as `$$`/`5$`, and repeated header rows. A statement records the PDF's page count, so every page can be viewed.
