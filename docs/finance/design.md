# Finance — design

Built 2026-11-21 (the user asked to build without a separate approval round). Finance is a personal money app for Singapore: **goals** you describe in your own words, **insights** on where money goes, and **statement import** that reads bank PDFs and logs every transaction. It must stay useful for years: goals come and go (an emergency fund now, a flat or a car later), and the history keeps accumulating.

## 1. Sections

| Section      | What it does                                                                                                                                |
| ------------ | ------------------------------------------------------------------------------------------------------------------------------------------- |
| Overview     | This month's spend vs your usual, category breakdown (click → merchants → entries), 12-month trend, insight cards, active goals at a glance |
| Goals        | Active goals with progress, status (on track / at risk / behind), forecast; check-ins; achieved goals kept as a timeline                    |
| Transactions | Every entry, searchable; fix a merchant or category once and similar entries follow                                                          |
| Import       | Drop statement PDFs; each is parsed, reconciled against the statement's own totals, and flagged if anything is off                        |
| Settings     | Claude switches + spend cap + key, merchant directory (refresh), accounts                                                                   |

## 2. Statement import

**Finding (2026-11-21):** the HSBC Singapore e-statements have **no text layer** — zero fonts, ~12,000 inline 1-bit image masks per file. Text-extraction libraries (`ledongthuc/pdf`, go-pdfium text API, pdfcpu) return nothing. Reading them needs OCR.

Pipeline (`api/internal/statement`, `api/internal/ocr`):

0. **Text layer first.** PDFs that carry their own text (Citibank Singapore) are read exactly with `pdftotext -bbox` — words with positions, no OCR, ~0.5 s. Rotated print codes in the margin are dropped. If those rows don't reconcile (or there is no text, as with HSBC), OCR takes over:
1. **Render** each page with `pdftoppm -r 300 -gray` (poppler).
2. **OCR** with Tesseract 5 (`--psm 6`, TSV output → words with bounding boxes). Local: statements never leave the machine. Tesseract 5.5 read both sample statements with every transaction amount correct; it misread one header figure (a 1 read as a 4) and missed one isolated line — hence step 5.
3. **Layout**: words → lines (by vertical overlap). The transaction column is found from the `DESCRIPTION` / `AMOUNT` header positions, so the account-summary panel on page 1 doesn't bleed into rows.
4. **Parse** (`DD Mon [DD Mon] description amount`; credits as `CR`, `(…)` or `-`; HSBC's label/value summary panel and Citibank's boxed summary row — previous − payments & credits + purchases & advances + interest + fees = current; `FOREIGN AMOUNT …` lines kept as the original currency; masked card-number lines and sub-totals skipped): post date, transaction date, description (+ wrapped continuation lines), amount, credit flag. Year inferred from the statement period (handles Dec → Jan). Summary fields: period, previous balance, payments & credits, purchases & debits, total due, card last 4.
5. **Reconcile**: Σdebits must equal _Purchases & Debits_, Σcredits _Payments & Credits_, and previous + debits − credits the total balance. On a mismatch the importer retries with other OCR settings (higher DPI, different page segmentation, a crop of the transaction column, digits-only re-read of amounts) and keeps the variant that reconciles. If none does, the statement is marked **Needs review** with the exact gap (e.g. "S$0.09 missing") and the page images, so the missing row can be added by hand.
6. **Dedupe**: a file is identified by its SHA-256; re-importing is a no-op.

Considered: Claude reading the PDF (accurate, but sends name, address and card number to an external API) — offered later as an opt-in fallback only. Pure-Go rendering via go-pdfium/WebAssembly — no OCR in pure Go, so Tesseract is needed anyway and poppler is the smaller dependency. The runtime image therefore is `debian:trixie-slim` + `poppler-utils` + `tesseract-ocr` instead of distroless; Dev mode needs the same two packages on the host (`tools/setup/wsl-toolchain.sh`).

## 3. Merchant resolution (Singapore)

Card descriptors are short, truncated and carry processor prefixes (`SMP*DAILY FUSION CUISI`, `fp*Food Panda`, `Grab* 1a2b3c4d…`, `WL *STEAM PURCHASE`, `KOPITIAM@JEM`, `SONG FA - VELOCITY@NOV`). Payment facilitators put their own prefix before the merchant name ([billing descriptors](https://en.wikipedia.org/wiki/Billing_descriptor), [PayFac prefixes](https://truptinatu.substack.com/p/statement-descriptors)). Resolution is a cascade, cheapest and most certain first (the pattern Plaid describes: [templates first, then fuzzy matching](https://plaid.com/blog/making-sense-of-messy-data/); [enrichment engine](https://plaid.com/blog/transaction-enrichment-engine)):

1. **Your rules** — a merchant/category you fixed before (exact normalised key). Like [Actual's payee rules](https://actualbudget.org/docs/budgeting/rules/), a correction becomes a rule that runs first next time.
2. **Clean**: strip the trailing location/country (`SINGAPORE SG`, `Vicenza IT` → foreign), processor prefixes (`SMP*`, `SMP_`, `fp*`, `WL *`, `TS/`, `SQ *`…), reference ids (hex hashes, long numbers), and split `brand@place` / `brand - branch`.
3. **SG brand list** (curated, `api/internal/merchant/brands.yaml`): Grab, foodpanda, Shopee, Lazada, FairPrice, CS Fresh, Kopitiam, Prudential, Singlife, Eight, SimplyGo `BUS/MRT`, Steam… each with a category.
4. **Directory match** against two open datasets, downloaded in bulk and matched **locally** (no spending data leaves the machine):
   - **OpenStreetMap** Singapore shops and food places (~16.6k named POIs, Overpass API, ODbL) — brand + branch + type → category.
   - **ACRA** registered entities ([data.gov.sg collection 2](https://data.gov.sg/collections/2/view), monthly, free licence) — legal name + **primary SSIC code** → category (56xxx food, 4711 supermarkets, 862xx dental…). Live entities only: status `Live` (sole proprietors, partnerships) and `Live Company` (companies), filtered server-side.
   - Scoring: descriptors are often cut at ~22 characters, so a descriptor that is a **prefix** of a directory name scores high (`DAILY FUSION CUISI` → `DAILY FUSION CUISINE`); plus token overlap with the last token allowed to be partial, character-trigram similarity, legal suffixes ignored (`(S) PTE. LTD.`), and a boost when the branch hint matches the address (`TPY` → Toa Payoh, `NOV` → Novena, `POIZ CENTR` → Poiz Centre). ≥ 0.8 auto, 0.55–0.8 auto but flagged to check, below → left as the cleaned descriptor.
5. **Claude with web search** (optional switch) for what's still unknown: only the descriptor string is sent, never amounts or dates; results are cached per descriptor.

## 4. Categories and kinds

Spending categories (stable ids, colour per category): dining, food delivery, groceries, public transport, taxi & ride-hailing, shopping, bills & telco, subscriptions, health, insurance, entertainment, leisure & fitness, travel, education, personal care, home, gifts, fees & interest, other. Each transaction also has a **kind**: purchase, refund (credit from a merchant → negative spend in its category), payment (paying the card — never spending), fee, income, transfer. `BUS/MRT` = public transport. `Grab*` descriptors don't say ride vs food: default taxi & ride-hailing, flagged to check.

Months are calendar months by transaction date (not statement periods). Large irregular items (e.g. a yearly insurance premium) are flagged as one-offs, and the trend can spread them over 12 months.

## 4b. Shared costs (splits)

When you pay for a group, the card shows the full amount but most of it isn't your spending. Approaches compared: Splitwise stores, per expense, each person's paid and owed share ([API](https://dev.splitwise.com/)); YNAB records only your share as spending and the rest as a reimbursement until it's paid back ([Splitwise and YNAB](https://support.ynab.com/en_us/splitwise-and-ynab-a-guide-H1GwOyuCq.md), [Reimbursements](https://support.ynab.com/en_us/reimbursements-in-ynab-H1W7ilhC5.md)); double-entry practice treats money fronted for friends as a receivable, not an expense ([plaintextaccounting.org](https://plaintextaccounting.org/Splitting-the-bill), [GnuCash list](https://lists.gnucash.org/pipermail/gnucash-user/2020-September/093225.html)).

Chosen model (`api/internal/split`, `txn_shares`, `people`):

- The **card amount never changes** (statements must still reconcile). Each other person gets a share row (amount in cents, weight, `settled_at`); **your share = amount − others' shares**, and only your share counts in insights, goals and averages.
- Split **equally**, **unequally** (weights / percentages) or by **exact amounts** (you pay the rest). Whole cents with the largest-remainder rule; leftover cents go to you, so shares always sum exactly.
- Others' shares are **owed to you** until you tick "paid back" (repayments arrive by PayNow/bank transfer, not on the card statement). Overview shows who owes what; a person's sheet lists every shared cost.
- **Your cost** (`my_cost`): type what something really cost you without naming anyone (e.g. part was repaid in cash) — no receivable is tracked. A split replaces it.
- "Leave out of insights" stays for costs that were entirely someone else's. Manual entries can have their date and amount edited; statement rows can't (they're the bank's record).

## 5. Goals — built for the long run

Goal shape informed by [Monarch Goals 3.0](https://help.monarch.com/hc/en-us/articles/44373110771860-Introducing-Goals-3-0) (target + date + planned monthly → on track / ahead / at risk; per-goal history; archiving keeps history) and fintech UX guidance to celebrate good decisions ([Eleken](https://eleken.co/blog-posts/fintech-ux-best-practices), [Arounda](https://arounda.agency/blog/personal-finance-apps-best-design-practices)).

| Kind     | Example                                    | Progress from                                            |
| -------- | ------------------------------------------ | -------------------------------------------------------- |
| `target` | Emergency fund S$20k by Dec 2027           | Check-ins of the current amount                          |
| `payoff` | Clear a S$5k loan                          | Check-ins of the remaining balance                       |
| `limit`  | Food delivery ≤ S$250 a month              | **Automatic** from transactions (categories / merchants / keywords), amount or count |
| `habit`  | Invest S$800 every month                   | Check-ins per period; streak                             |

Every goal has: title, _why_ (your own words), life area (safety net, home, retirement, lifestyle, debt, other), start/target date, milestones, status (active, paused, achieved, archived). Forecast: linear trend of check-ins → projected date; required per month = remaining ÷ months left. Achieved goals stay on a timeline, so in four years the app shows what you finished and you add the next ones.

**Write a paragraph → goals.** Works without any API key: the paragraph is read on this computer (`api/internal/goals/words.go`) — amounts and their role (`S$18k` target, `S$6k now`, `S$700 a month`), dates (`by July 2029`, `end of 2027`, `in 6 months`, `30/09/2029`), intent words (save, invest, pay off, keep under, at most N rides) and topic words (emergency, BTO, wedding, food delivery…). A sentence with only "have now" / "per month" amounts adds to the goal before it. If Claude is switched on and a key is set, Claude Opus 5.5 reads the paragraph instead (with category names and, if allowed, average monthly spend per category) and falls back to the local reader on any error. Nothing is saved until you review and edit the drafts.

Until a savings goal has check-ins, its planned monthly amount drives the forecast ("At S$2,600/month: reached Aug 2029 — S$2,715/month needed for Jul 2029").

## 6. Insights

Computed locally from transactions (`api/internal/insights`): category change vs the 3-month average (≥ 25 % and ≥ S$30), large one-offs, recurring charges / subscriptions (same merchant, ~monthly, amount within 10 %), frequency (e.g. delivery orders), foreign-currency **DCC fees** (avoidable: pay in local currency), and goal pace (spend-limit goals: projected end-of-month vs cap).

## 7. Claude API (optional)

Every feature works without a key: goal drafting has the local reader (§5), merchant lookup has the local directory (§3). Model `claude-opus-5-5`; each feature behind its own switch, off until a key is set (Settings, stored in the app's local database, or `ANTHROPIC_API_KEY`). Monthly **spend cap** (default US$5) with a usage meter; calls stop at the cap. Sent: goal paragraphs (+ optional category totals), unknown merchant descriptors. Never sent: statements, amounts, dates, name, card numbers.

## 8. UI

Copilot Money is widely rated the best-designed finance app ([Rob Berger](https://robberger.com/personal-finance-software/)); Monarch for goals/forecasting ([Monarch](https://www.monarch.com/blog/goals)). Principles: one number first (spent this month vs usual), progressive disclosure (category → merchants → entries in a side sheet), calm colour (one hue per category, red only for over-limit), amounts right-aligned in tabular numerals, CR/refunds in green, every chart clickable, works narrow (one column below ~760 px), light/dark with the hub theme.
