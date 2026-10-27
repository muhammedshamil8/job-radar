// job-radar checks company job boards for new Go roles and sends them to Telegram.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"html"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/muhammedshamil8/job-radar/internal/filter"
	"github.com/muhammedshamil8/job-radar/internal/notify"
	"github.com/muhammedshamil8/job-radar/internal/source"
	"github.com/muhammedshamil8/job-radar/internal/store"
)

type config struct {
	Rules      filter.Rules     `json:"rules"`
	Companies  []source.Company `json:"companies"`
	PrepTopics []string         `json:"prep_topics"` // one per day in the daily summary
}

// ist is the timezone that decides when a new day starts for the daily summary.
var ist = time.FixedZone("IST", 5*3600+30*60)

type match struct {
	job source.Job
	res filter.Result
}

func main() {
	cfgPath := flag.String("config", "companies.json", "companies and rules")
	seenPath := flag.String("seen", "data/seen.json", "already-sent job IDs")
	digestPath := flag.String("digest", "data/digest.txt", "date of the last daily summary")
	detailsPath := flag.String("details", "data/park-details.json", "cached descriptions of IT park jobs")
	initOnly := flag.Bool("init", false, "mark every current match as seen without sending (first run)")
	all := flag.Bool("all", false, "print every current match, ignore and keep the seen file")
	workers := flag.Int("workers", 8, "boards fetched at once")
	flag.Parse()

	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	seen, err := store.Load(*seenPath)
	if err != nil {
		log.Fatalf("seen file: %v", err)
	}
	details, err := store.LoadDetails(*detailsPath)
	if err != nil {
		log.Fatalf("details file: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client := &http.Client{Timeout: 30 * time.Second}

	matches, failed := scan(ctx, client, cfg, *workers, &describer{client: client, cache: details})
	details.Prune(60 * 24 * time.Hour)
	if err := details.Save(*detailsPath); err != nil {
		log.Fatalf("save details: %v", err)
	}

	var fresh []match
	for _, m := range matches {
		if *all || seen[m.job.ID].IsZero() {
			fresh = append(fresh, m)
		}
	}
	sort.Slice(fresh, func(i, j int) bool { return fresh[i].job.Posted.After(fresh[j].job.Posted) })
	log.Printf("%d companies, %d failed, %d matches, %d new", len(cfg.Companies), failed, len(matches), len(fresh))

	if *all {
		printMatches(fresh)
		return
	}
	if !*initOnly && len(fresh) > 0 {
		if err := deliver(ctx, client, fresh); err != nil {
			// Keep the seen file unchanged so these are retried next run.
			log.Fatalf("deliver: %v", err)
		}
	}
	now := time.Now().UTC()
	for _, m := range fresh {
		seen[m.job.ID] = now
	}
	seen.Prune(60 * 24 * time.Hour)
	if err := seen.Save(*seenPath); err != nil {
		log.Fatalf("save seen: %v", err)
	}

	if !*initOnly {
		s := summary{companies: len(cfg.Companies), failed: failed, open: len(matches), newDay: seen.Since(now.Add(-24 * time.Hour))}
		if err := dailySummary(ctx, client, cfg.PrepTopics, s, *digestPath, now); err != nil {
			log.Printf("daily summary: %v", err)
		}
	}
}

type summary struct {
	companies, failed, open, newDay int
}

// dailySummary sends one message per IST day: what the scan saw, and a topic to prepare.
// It runs on the first scan of the day, so it doubles as proof the scanner is alive.
func dailySummary(ctx context.Context, client *http.Client, topics []string, s summary, path string, now time.Time) error {
	today := now.In(ist).Format("2006-01-02")
	if last, err := os.ReadFile(path); err == nil && strings.TrimSpace(string(last)) == today {
		return nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "☀️ <b>job-radar · %s</b>\n", now.In(ist).Format("Mon 2 Jan"))
	fmt.Fprintf(&b, "Scanned %d companies", s.companies)
	if s.failed > 0 {
		fmt.Fprintf(&b, " (%d failed)", s.failed)
	}
	fmt.Fprintf(&b, ".\nOpen matches: %d · new in last 24h: %d\n", s.open, s.newDay)
	if len(topics) > 0 {
		topic := topics[now.In(ist).YearDay()%len(topics)]
		fmt.Fprintf(&b, "\n📚 <b>Prep today:</b> %s", html.EscapeString(topic))
	}

	if err := send(ctx, client, b.String(), nil); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(today+"\n"), 0o644)
}

func loadConfig(path string) (config, error) {
	var cfg config
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	return cfg, json.Unmarshal(data, &cfg)
}

// scan fetches every board with a bounded worker pool and keeps the matches.
func scan(ctx context.Context, client *http.Client, cfg config, workers int, d *describer) (matches []match, failed int) {
	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		sem = make(chan struct{}, workers)
	)
	for _, co := range cfg.Companies {
		wg.Add(1)
		go func(co source.Company) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			jobs, err := source.Fetch(ctx, client, co)
			if err == nil {
				jobs = d.fill(ctx, cfg.Rules, jobs)
			}
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				log.Printf("skip %s (%s): %v", co.Name, co.ATS, err)
				failed++
				return
			}
			for _, j := range jobs {
				if res := filter.Check(cfg.Rules, j); res.OK {
					matches = append(matches, match{j, res})
				}
			}
		}(co)
	}
	wg.Wait()
	return matches, failed
}

// describer fills in descriptions for title-only jobs, opening each job page once
// and caching the text, so the normal Go and experience rules can run on them.
type describer struct {
	client  *http.Client
	mu      sync.Mutex
	cache   store.Details
	fetched int
}

// maxDescribe caps new detail-page requests per run, to stay polite to the portals.
const maxDescribe = 150

func (d *describer) fill(ctx context.Context, r filter.Rules, jobs []source.Job) []source.Job {
	out := jobs[:0]
	for _, j := range jobs {
		if !j.TitleOnly {
			out = append(out, j)
			continue
		}
		if !filter.ParkCandidate(r, j.Title) {
			continue
		}
		d.mu.Lock()
		cached, ok := d.cache[j.ID]
		budget := d.fetched < maxDescribe
		if !ok && budget {
			d.fetched++
		}
		d.mu.Unlock()

		if !ok {
			if !budget {
				continue // next run
			}
			text, err := source.Describe(ctx, d.client, j)
			if err != nil {
				log.Printf("describe %s: %v", j.ID, err)
				continue
			}
			cached = store.Detail{Text: text, Fetched: time.Now().UTC()}
			d.mu.Lock()
			d.cache[j.ID] = cached
			d.mu.Unlock()
		}
		j.Description = cached.Text
		j.TitleOnly = false
		out = append(out, j)
	}
	return out
}

func items(ms []match) []notify.Item {
	out := make([]notify.Item, len(ms))
	for i, m := range ms {
		title := m.job.Title
		if strings.HasPrefix(m.job.ID, "technopark:") || strings.HasPrefix(m.job.ID, "infopark:") {
			title = "🌴 " + title // Kerala IT park
		}
		out[i] = notify.Item{Title: title, Company: m.job.Company, Location: m.job.Location, URL: m.job.URL, MinYears: m.res.MinYears}
	}
	return out
}

// deliver sends new matches to Telegram when configured, otherwise prints them.
func deliver(ctx context.Context, client *http.Client, ms []match) error {
	return send(ctx, client, fmt.Sprintf("🔎 %d new Go job(s)", len(ms)), ms)
}

func send(ctx context.Context, client *http.Client, header string, ms []match) error {
	token, chat := os.Getenv("TELEGRAM_BOT_TOKEN"), os.Getenv("TELEGRAM_CHAT_ID")
	if token == "" || chat == "" {
		fmt.Println(header)
		printMatches(ms)
		return nil
	}
	tg := notify.Telegram{Token: token, ChatID: chat, Client: client}
	return tg.Send(ctx, header, items(ms))
}

func printMatches(ms []match) {
	for _, it := range items(ms) {
		years := "?"
		if it.MinYears >= 0 {
			years = fmt.Sprintf("%d+", it.MinYears)
		}
		fmt.Printf("%s | %s | %s | %s yrs\n  %s\n", it.Title, it.Company, it.Location, years, it.URL)
	}
}
