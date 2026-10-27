# job-radar

Checks company job boards for new **Go (Golang)** roles at junior to mid level in India, and sends each new match to Telegram. Runs on GitHub Actions three times a day.

```
🔎 2 new Go job(s)

Intermediate Backend Engineer, India
GitLab · Bangalore, India · years not stated
https://job-boards.greenhouse.io/gitlab/jobs/...
```

## How it works

1. Reads `companies.json`: a list of companies and the public job-board API they use (Greenhouse, Lever, Ashby), RemoteOK's `golang` tag, and the Technopark (Trivandrum) and Infopark (Kochi) job portals, which cover hundreds of small and mid-size Kerala companies.
2. Fetches all boards concurrently (bounded worker pool, per-request timeouts, retries with exponential backoff on 429/5xx).
3. Keeps a job only if it:
   - is about Go the language. `golang` anywhere, or `Go` used as a language ("in Go", "Go, Rust", "Python/Go"), not the verb ("go to market", "go live");
   - is not senior, lead, staff, intern or level 3, not a support/sales title, and the title does not name a different stack ("Backend (Python)");
   - is in India, an Indian city, APAC, or remote and open to India;
   - does not ask for 3+ years (the smallest "N years" figure) and does not mention 5+ years anywhere.
   - IT park portals have titles but no descriptions, and Kerala has few Go roles, so their jobs (marked 🌴) only need a backend / full-stack / Node / Go title that is not senior and not built on Java, .NET, Angular, PHP or Django.
4. Sends only jobs not in `data/seen.json`, then records them. The workflow commits that file back so the next run remembers.
5. Once a day (first run after midnight IST) it sends a short summary: companies scanned, open matches, new in the last 24 hours, and one interview topic to prepare from `prep_topics` in `companies.json`. It doubles as proof the scanner is running.

Only official public job-board APIs are used. No scraping, no logins.

## Run locally

```sh
go test ./...
go run . -all     # print every current match, change nothing
go run .          # print new matches and record them (sends to Telegram if the env vars are set)
go run . -init    # record current matches without sending (quiet start)
```

## Set up alerts

1. In Telegram, message **@BotFather**, send `/newbot`, and copy the token.
2. Send any message to your new bot, then open `https://api.telegram.org/bot<TOKEN>/getUpdates` and copy `message.chat.id`.
3. In the GitHub repo: Settings → Secrets and variables → Actions → add `TELEGRAM_BOT_TOKEN` and `TELEGRAM_CHAT_ID`.
4. Actions → scan → Run workflow, to test.

## Add a company

Find its board slug, check it returns jobs, and add a line to `companies.json`:

| ATS | Test URL |
|---|---|
| greenhouse | `https://boards-api.greenhouse.io/v1/boards/<slug>/jobs` |
| lever | `https://api.lever.co/v0/postings/<slug>?mode=json` |
| ashby | `https://api.ashbyhq.com/posting-api/job-board/<slug>` |

Filters (locations, max years, excluded title words) are in the `rules` block of the same file.
