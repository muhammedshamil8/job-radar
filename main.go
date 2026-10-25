// job-radar checks company job boards for new Go roles and sends them to Telegram.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/muhammedshamil8/job-radar/internal/filter"
	"github.com/muhammedshamil8/job-radar/internal/notify"
	"github.com/muhammedshamil8/job-radar/internal/source"
	"github.com/muhammedshamil8/job-radar/internal/store"
)

type config struct {
	Rules     filter.Rules     `json:"rules"`
	Companies []source.Company `json:"companies"`
}

type match struct {
	job source.Job
	res filter.Result
}

func main() {
	cfgPath := flag.String("config", "companies.json", "companies and rules")
	seenPath := flag.String("seen", "data/seen.json", "already-sent job IDs")
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client := &http.Client{Timeout: 30 * time.Second}

	matches, failed := scan(ctx, client, cfg, *workers)

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
func scan(ctx context.Context, client *http.Client, cfg config, workers int) (matches []match, failed int) {
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

func items(ms []match) []notify.Item {
	out := make([]notify.Item, len(ms))
	for i, m := range ms {
		out[i] = notify.Item{Title: m.job.Title, Company: m.job.Company, Location: m.job.Location, URL: m.job.URL, MinYears: m.res.MinYears}
	}
	return out
}

// deliver sends to Telegram when configured, otherwise prints.
func deliver(ctx context.Context, client *http.Client, ms []match) error {
	token, chat := os.Getenv("TELEGRAM_BOT_TOKEN"), os.Getenv("TELEGRAM_CHAT_ID")
	if token == "" || chat == "" {
		printMatches(ms)
		return nil
	}
	tg := notify.Telegram{Token: token, ChatID: chat, Client: client}
	header := fmt.Sprintf("🔎 %d new Go job(s)\n\n", len(ms))
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
