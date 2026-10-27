package source

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Kerala IT park portals list jobs for hundreds of small and mid-size companies
// that do not use Greenhouse, Lever or Ashby. They expose titles but no
// descriptions, so their jobs are marked TitleOnly and filtered on the title.

// maxParkPages caps requests per run; both portals list newest first or a few hundred jobs.
const maxParkPages = 25

// Technopark (Trivandrum): JSON API behind technopark.in/job-search.
func technopark(ctx context.Context, c *http.Client, co Company) ([]Job, error) {
	var jobs []Job
	for page := 1; page <= maxParkPages; page++ {
		var resp struct {
			LastPage int `json:"last_page"`
			Data     []struct {
				ID         int64  `json:"id"`
				Title      string `json:"job_title"`
				PostedDate string `json:"posted_date"`
				Company    struct {
					Name string `json:"company"`
				} `json:"company"`
			} `json:"data"`
		}
		url := fmt.Sprintf("https://technopark.in/api/paginated-jobs?page=%d&search=&type=", page)
		if err := getJSON(ctx, c, url, &resp); err != nil {
			return nil, err
		}
		for _, j := range resp.Data {
			posted, _ := time.Parse("2006-01-02", j.PostedDate)
			jobs = append(jobs, Job{
				ID:        fmt.Sprintf("technopark:%d", j.ID),
				Company:   strings.TrimSpace(j.Company.Name) + " (Technopark)",
				Title:     strings.TrimSpace(j.Title),
				Location:  "Technopark, Trivandrum, Kerala",
				URL:       fmt.Sprintf("https://technopark.in/job-details/%d", j.ID),
				Posted:    posted,
				TitleOnly: true,
			})
		}
		if page >= resp.LastPage {
			break
		}
	}
	return jobs, nil
}

var (
	infoparkRowRE  = regexp.MustCompile(`(?s)<tr>\s*<td[^>]*>([^<]*)</td>\s*<td[^>]*>([^<]*)</td>\s*<td[^>]*>([^<]*)</td>.*?href="(https://infopark\.in/company-jobs/details/[^"]+)"`)
	infoparkPageRE = regexp.MustCompile(`companies-job/0\?page=(\d+)`)
)

// Infopark (Kochi, Cherthala, Thrissur): the job table is returned as HTML inside JSON.
func infopark(ctx context.Context, c *http.Client, co Company) ([]Job, error) {
	var jobs []Job
	last := 1
	for page := 1; page <= last && page <= maxParkPages; page++ {
		url := fmt.Sprintf("https://infopark.in/companies-job/0?page=%d", page)
		var resp struct {
			AllJobs    string `json:"all_jobs"`
			Pagination string `json:"pagination"`
		}
		if err := getInfopark(ctx, c, url, &resp); err != nil {
			return nil, err
		}
		for _, m := range infoparkPageRE.FindAllStringSubmatch(resp.Pagination, -1) {
			if n, _ := strconv.Atoi(m[1]); n > last {
				last = n
			}
		}
		for _, m := range infoparkRowRE.FindAllStringSubmatch(resp.AllJobs, -1) {
			posted, _ := time.Parse("02-01-2006", strings.TrimSpace(m[1]))
			link := m[4]
			jobs = append(jobs, Job{
				ID:        "infopark:" + link[strings.LastIndex(link, "/details/")+len("/details/"):],
				Company:   plain(m[3]) + " (Infopark)",
				Title:     plain(m[2]),
				Location:  "Infopark, Kochi, Kerala",
				URL:       link,
				Posted:    posted,
				TitleOnly: true,
			})
		}
	}
	return jobs, nil
}

// getInfopark is getJSON with the AJAX header the portal needs to return JSON.
func getInfopark(ctx context.Context, c *http.Client, url string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "job-radar/1.0 (+https://github.com/muhammedshamil8/job-radar)")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	return json.Unmarshal(body, v)
}
