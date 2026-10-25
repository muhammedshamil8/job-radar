package source

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Greenhouse: https://developers.greenhouse.io/job-board.html
func greenhouse(ctx context.Context, c *http.Client, co Company) ([]Job, error) {
	var resp struct {
		Jobs []struct {
			ID          int64  `json:"id"`
			Title       string `json:"title"`
			AbsoluteURL string `json:"absolute_url"`
			UpdatedAt   string `json:"updated_at"`
			Content     string `json:"content"`
			Location    struct {
				Name string `json:"name"`
			} `json:"location"`
		} `json:"jobs"`
	}
	url := fmt.Sprintf("https://boards-api.greenhouse.io/v1/boards/%s/jobs?content=true", co.Slug)
	if err := getJSON(ctx, c, url, &resp); err != nil {
		return nil, err
	}
	jobs := make([]Job, 0, len(resp.Jobs))
	for _, j := range resp.Jobs {
		posted, _ := time.Parse(time.RFC3339, j.UpdatedAt)
		jobs = append(jobs, Job{
			ID:          fmt.Sprintf("greenhouse:%s:%d", co.Slug, j.ID),
			Company:     co.Name,
			Title:       j.Title,
			Location:    j.Location.Name,
			URL:         j.AbsoluteURL,
			Description: plain(j.Content),
			Posted:      posted,
		})
	}
	return jobs, nil
}

// Lever: https://github.com/lever/postings-api
func lever(ctx context.Context, c *http.Client, co Company) ([]Job, error) {
	var resp []struct {
		ID               string `json:"id"`
		Text             string `json:"text"`
		HostedURL        string `json:"hostedUrl"`
		CreatedAt        int64  `json:"createdAt"`
		DescriptionPlain string `json:"descriptionPlain"`
		AdditionalPlain  string `json:"additionalPlain"`
		WorkplaceType    string `json:"workplaceType"`
		Lists            []struct {
			Text    string `json:"text"`
			Content string `json:"content"`
		} `json:"lists"`
		Categories struct {
			Location     string   `json:"location"`
			AllLocations []string `json:"allLocations"`
		} `json:"categories"`
	}
	url := fmt.Sprintf("https://api.lever.co/v0/postings/%s?mode=json", co.Slug)
	if err := getJSON(ctx, c, url, &resp); err != nil {
		return nil, err
	}
	jobs := make([]Job, 0, len(resp))
	for _, j := range resp {
		var desc strings.Builder
		desc.WriteString(j.DescriptionPlain)
		for _, l := range j.Lists {
			desc.WriteString(" " + l.Text + " " + plain(l.Content))
		}
		desc.WriteString(" " + j.AdditionalPlain)

		loc := j.Categories.Location
		if len(j.Categories.AllLocations) > 1 {
			loc = strings.Join(j.Categories.AllLocations, " / ")
		}
		if j.WorkplaceType == "remote" && !strings.Contains(strings.ToLower(loc), "remote") {
			loc += " (Remote)"
		}
		jobs = append(jobs, Job{
			ID:          "lever:" + co.Slug + ":" + j.ID,
			Company:     co.Name,
			Title:       j.Text,
			Location:    loc,
			URL:         j.HostedURL,
			Description: strings.TrimSpace(desc.String()),
			Posted:      time.UnixMilli(j.CreatedAt),
		})
	}
	return jobs, nil
}

// Ashby: https://developers.ashbyhq.com/docs/public-job-posting-api
func ashby(ctx context.Context, c *http.Client, co Company) ([]Job, error) {
	var resp struct {
		Jobs []struct {
			ID                 string `json:"id"`
			Title              string `json:"title"`
			Location           string `json:"location"`
			JobURL             string `json:"jobUrl"`
			PublishedAt        string `json:"publishedAt"`
			DescriptionPlain   string `json:"descriptionPlain"`
			IsRemote           bool   `json:"isRemote"`
			SecondaryLocations []struct {
				Location string `json:"location"`
			} `json:"secondaryLocations"`
		} `json:"jobs"`
	}
	url := fmt.Sprintf("https://api.ashbyhq.com/posting-api/job-board/%s", co.Slug)
	if err := getJSON(ctx, c, url, &resp); err != nil {
		return nil, err
	}
	jobs := make([]Job, 0, len(resp.Jobs))
	for _, j := range resp.Jobs {
		locs := []string{j.Location}
		for _, s := range j.SecondaryLocations {
			locs = append(locs, s.Location)
		}
		loc := strings.Join(locs, " / ")
		if j.IsRemote && !strings.Contains(strings.ToLower(loc), "remote") {
			loc += " (Remote)"
		}
		posted, _ := time.Parse(time.RFC3339, j.PublishedAt)
		jobs = append(jobs, Job{
			ID:          "ashby:" + co.Slug + ":" + j.ID,
			Company:     co.Name,
			Title:       j.Title,
			Location:    loc,
			URL:         j.JobURL,
			Description: j.DescriptionPlain,
			Posted:      posted,
		})
	}
	return jobs, nil
}

// RemoteOK: https://remoteok.com/api (first element is a legal notice).
// Slug is the tag to search, e.g. "golang".
func remoteOK(ctx context.Context, c *http.Client, co Company) ([]Job, error) {
	var resp []struct {
		ID          string `json:"id"`
		Position    string `json:"position"`
		Company     string `json:"company"`
		Location    string `json:"location"`
		URL         string `json:"url"`
		Description string `json:"description"`
		Date        string `json:"date"`
	}
	url := "https://remoteok.com/api?tag=" + co.Slug
	if err := getJSON(ctx, c, url, &resp); err != nil {
		return nil, err
	}
	var jobs []Job
	for _, j := range resp {
		if j.ID == "" || j.Position == "" {
			continue // legal notice
		}
		posted, _ := time.Parse(time.RFC3339, j.Date)
		jobs = append(jobs, Job{
			ID:          "remoteok:" + j.ID,
			Company:     j.Company + " (via RemoteOK)",
			Title:       j.Position,
			Location:    strings.TrimSpace(j.Location + " (Remote)"),
			URL:         j.URL,
			Description: plain(j.Description),
			Posted:      posted,
		})
	}
	return jobs, nil
}
