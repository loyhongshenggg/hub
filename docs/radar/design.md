# Radar — design

Approved with the user on 2026-11-21. Radar is a personal **learning dashboard** with a separate **news** section. It drives Phase 2 of the hub.

## 1. Two sections

|         | Learning (main, ~70%)                                                                       | News (side, ~30%)                                           |
| ------- | ------------------------------------------------------------------------------------------- | ----------------------------------------------------------- |
| Purpose | Learn senior-engineer skills in depth                                                       | Stay informed                                               |
| Unit    | Track → chapters → steps                                                                    | Story                                                       |
| Shape   | Structured courses: learning outcomes, step-by-step lessons, practice, checkpoint, capstone | Hot topics on your tracks' topics + latest relevant stories |
| Never   | A feed of latest posts                                                                      | Off-topic news                                              |

## 2. Learning model

Designed backwards ([Understanding by Design](https://www.wikipedia.org/wiki/Understanding_by_Design)): outcomes → how you'll know → activities.

- **Track**: one skill (e.g. _AI red teaming: LLM apps & agents_), 5–10 chapters, ends with a **capstone**.
- **Chapter**:
  1. **Learning outcomes**: 2–4 "By the end you can …" lines, senior-level (trade-offs, design, review).
  2. **Steps** (ordered): one resource each, with a _focus note_ ("read sections 2–3; notice the exfil channel") and minutes. A worked example comes first, then guided practice; later chapters fade guidance ([worked-example effect, expertise reversal](https://en.wikipedia.org/wiki/Worked-example_effect)).
  3. **Practice / walkthrough**: an external lab (pwn.college, PortSwigger, Rehberger's labs) or an in-app checklist to run locally.
  4. **Checkpoint**: tick each outcome you can now do, plus a rating (_Too easy / Just right / Too hard_ + _Worth it?_). Any outcome not met → a corrective step before moving on ([mastery learning](https://ascd.org/el/articles/lessons-of-mastery-learning)).
- **First tracks built in full**: Agentic engineering (Lauren Tan), Exploitation fundamentals (pwn.college), AI red teaming (Rehberger + OWASP LLM Top 10 + PortSwigger LLM labs). Outlines: Adversarial ML & poisoning (Carlini), Web security (PortSwigger), Malware analysis & detection (Hammond, ATT&CK, Sigma), Threat modeling & secure design.
- **Mentors**: Lauren Tan (no feed; curated links), Johann Rehberger, Nicholas Carlini, pwn.college (Yan Shoshitaishvili), John Hammond. **Gap fillers**: PortSwigger Web Security Academy, OWASP, MITRE ATLAS/ATT&CK, Sigma. Every curated URL is checked before it ships.
- **Skill map**: 23 topics in 5 areas (foundations, offense, AI security, defense, AI-era SWE) with prerequisite edges; a first-run self-assessment seeds the starting level per topic.

## 3. Adaptive engine

- **Within a chapter** steps keep their order. _Too easy_ offers to skip to practice; _too hard_ inserts a corrective step (a foundations step, or a prerequisite chapter's key step).
- **Next chapter** (on chapter completion) — candidates are chapters whose prerequisite topics are met. Score:
  - **Interest**: Thompson sample from a per-track/topic Beta(α, β); _worth it_ → α+1, _not_ → β+1, skip → β+0.5 ([bandit personalisation](https://ar5iv.arxiv.org/html/1604.03506)).
  - **Fit**: chapter difficulty vs your topic skill, aiming slightly above (zone of proximal development). Skill and difficulty update Elo-style from ratings ([Elo in learning systems](https://pmc.ncbi.nlm.nih.gov/articles/PMC12784335)).
  - **Learning progress**: topics where your recent outcomes improve score higher ([ZPDES](https://arxiv.org/abs/1310.3174)).
  - **Momentum**: bonus for staying in the current track; another track wins only when clearly better.
  - **Priority**: your area priorities.
- **Fresh mentor content** that matches a chapter's topic shows as an optional "Fresh from <mentor>" step there; otherwise it goes to News.
- **Weekly goal**: minutes per week (default 5 h), from step minutes.
- Later (not v1): spaced review of finished chapters ([FSRS](https://domenic.me/fsrs/)).

## 4. In-portal viewer (no redirects)

Each resource opens inside Radar in the best mode for it; mode is chosen automatically:

| Mode             | For                                                             | How                                                                                                                                                                                                                                  |
| ---------------- | --------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Reader (default) | Articles, blog posts, papers (arXiv HTML), news stories         | Backend fetches once, extracts with `codeberg.org/readeck/go-readability/v2`, sanitises with `bluemonday`, rewrites links/images, caches in SQLite (local, personal use, source + link always shown). Works on frame-blocking sites. |
| Video            | YouTube                                                         | `youtube-nocookie.com/embed/<id>` (frameable, verified).                                                                                                                                                                             |
| Live page        | Interactive pages on frameable sites (pwn.college, MITRE ATLAS) | iframe; framing headers checked per domain and remembered.                                                                                                                                                                           |
| Open outside     | Login-only labs on frame-blocking sites (PortSwigger labs)      | New tab; the walkthrough stays in Radar.                                                                                                                                                                                             |

Checked 2026-11-21: frameable — embracethered.com, nicholas.carlini.com, pwn.college, genai.owasp.org, atlas.mitre.org, arxiv.org/html, simonwillison.net, youtube-nocookie embed, conf2024.react.dev, krebsonsecurity.com. Blocked — portswigger.net, owasp.org/Top10, arxiv.org/abs, youtube.com/watch, github.com, Substack (latent.space).

## 5. News

- Stories from the news feeds (`api/content/sources.yaml`), tagged to skill-map topics; untagged stories hidden from the dashboard panel (still on the full News page).
- **Hot topics**: a topic is hot when its story count over the last 72 h is well above its 4-week rate (simplified [burst detection](https://www.cs.cornell.edu/home/kleinber/kdd02.html)); related stories grouped into a chip ("Prompt injection in coding agents · 6 ↑").
- Panel: top 5 hot topics + 10 latest relevant stories. Full page keeps search, read, save.

## 6. Claude API (Claude Opus 5.5) — each behind its own switch

| Feature                                                                     | Claude on                                                                     | Claude off                                         |
| --------------------------------------------------------------------------- | ----------------------------------------------------------------------------- | -------------------------------------------------- |
| Tagging (topics, difficulty, type, minutes, prerequisites, 2-line summary)  | Claude reads title + feed summary + page start                                | Keyword tagger + per-source defaults; hand-fixable |
| Chapter drafting (outcomes, step order + focus notes, practice walkthrough) | Claude drafts from verified resources; goes live only after the user approves | Hand-written chapters                              |
| Finding new resources                                                       | Claude with web search proposes resources for thin topics                     | Algorithmic discovery (below)                      |

Not used: self-check quizzes. Key in an untracked env file (`ANTHROPIC_API_KEY`) until Phase 3 secrets; without a key the switches are disabled. **Monthly spend cap** (default $5) with a usage meter. Only public content is sent; history and ratings stay local.

**Algorithmic discovery** (weekly, for topics with < 5 unfinished steps): candidates from (1) outbound links of trusted items — co-citation counts as authority ([snowballing](https://swov.nl/en/publicatie/guidelines-snowballing-systematic-literature-studies-and-replication-software), [HITS](https://nlp.stanford.edu/IR-book/html/htmledition/hubs-and-authorities-1.html)); (2) Semantic Scholar references + citations of trusted papers (keyless, verified); (3) HN Algolia stories ≥ 100 points (verified); (4) recurring blogs/channels → suggested feeds. Rank: BM25 relevance × authority × freshness × type fit, then MMR for diversity ([Carbonell & Goldstein](https://www.cs.cmu.edu/~jgc/publication/MMR_DiversityBased_Reranking_SIGIR_1998.pdf)); canonical-URL dedupe. ≤ 5 per topic into the **Suggestions inbox** with a reason; nothing joins until approved.

## 7. UI principles

From [LMS UX guidance](https://www.lazarev.agency/articles/lms-ux) (focus, no overload), dashboard practice ([Toptal](https://www.toptal.com/designers/ui/dashboard-design-best-practices)), motion systems ([Atlassian](https://atlassian.design/foundations/motion), [Material](https://m1.material.io/motion/duration-easing.html)), reading research ([Baymard](https://baymard.com/blog/line-length-readability)), [NN/g on skeletons](https://nngroup.com/articles/skeleton-screens/), [View Transitions](https://developer.chrome.com/blog/view-transitions-in-2025), [web.dev motion a11y](https://web.dev/learn/accessibility/motion):

1. **One primary action per screen.** The dashboard leads with _Continue_ (next step of the current chapter). Everything else is secondary or one click deeper (progressive disclosure). No leaderboards, no badge spam.
2. **Focus mode for lessons.** Lesson view hides the dashboard chrome: step list, resource, focus note, _Done_.
3. **Motion with purpose.** Tokens: 120 ms (press/hover), 240 ms (enter/expand), 400 ms (view change); standard ease-out on enter. View changes use the View Transitions API (a track card grows into the course header). All motion off under `prefers-reduced-motion`.
4. **Instant feel.** Optimistic updates (_Done_ ticks immediately, next step slides in); prefetch the next step's reader content; skeletons only for waits of 1–10 s (reader fetches), nothing for fast loads.
5. **Reading comfort.** Reader text at ~66 characters per line (max 75), 1.7 line height, larger body size, light/dark following the hub theme.
6. **Responsive.** Container queries: one column below ~760 px (the hub tab can be narrow), learning + news columns above; touch-sized targets.
7. **Keyboard.** `j`/`k` previous/next step, `d` done, `o` open outside, `Esc` back.
8. **Calm progress.** Weekly goal ring, chapter progress bar, a small celebration on chapter completion.

## 8. Build plan (Phase 2 step 3)

| Step | What                                                                                                                                                                                  |
| ---- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 3a   | Content model + loader (tracks/chapters/steps/outcomes/practice, skill map, mentors), curated v1 for the 3 first tracks (verified URLs), keyword tagger, mentor feeds → fresh content |
| 3b   | Engine (progress, ratings, chapter selection, correctives) + API; simulated-learner tests                                                                                             |
| 3c   | Reader service (fetch → readability → sanitise → cache; mode detection; YouTube embeds)                                                                                               |
| 3d   | UI: dashboard (Learning + News), track/chapter/lesson views, reader, self-assessment, settings; motion + responsive                                                                   |
| 3e   | News tagging + hot topics; Claude features with switches + spend cap; algorithmic discovery + Suggestions inbox                                                                       |
| 3f   | Hub integration: badge (new fresh steps), notify (mentor publishes); ADR with sources                                                                                                 |
