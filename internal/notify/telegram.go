// Package notify sends job alerts to Telegram.
package notify

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strings"
)

// Item is one alert line.
type Item struct {
	Title, Company, Location, URL string
	MinYears                      int
}

// Format renders one item as Telegram HTML.
func Format(it Item) string {
	years := "years not stated"
	if it.MinYears >= 0 {
		years = fmt.Sprintf("%d+ yrs", it.MinYears)
	}
	return fmt.Sprintf("<b>%s</b>\n%s · %s · %s\n%s",
		html.EscapeString(it.Title), html.EscapeString(it.Company),
		html.EscapeString(it.Location), years, it.URL)
}

// Telegram sends messages through a bot.
type Telegram struct {
	Token, ChatID string
	Client        *http.Client
}

// maxLen stays under Telegram's 4096-character message limit.
const maxLen = 3800

// Send posts items, packing several into each message.
func (t Telegram) Send(ctx context.Context, header string, items []Item) error {
	var msgs []string
	cur := header
	for _, it := range items {
		block := Format(it)
		if len(cur)+len(block)+2 > maxLen {
			msgs = append(msgs, cur)
			cur = ""
		}
		if cur != "" {
			cur += "\n\n"
		}
		cur += block
	}
	if cur != "" {
		msgs = append(msgs, cur)
	}
	for _, m := range msgs {
		if err := t.send(ctx, m); err != nil {
			return err
		}
	}
	return nil
}

func (t Telegram) send(ctx context.Context, text string) error {
	form := url.Values{
		"chat_id":                  {t.ChatID},
		"text":                     {text},
		"parse_mode":               {"HTML"},
		"disable_web_page_preview": {"true"},
	}
	endpoint := "https://api.telegram.org/bot" + t.Token + "/sendMessage"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := t.Client.Do(req)
	if err != nil {
		// The error text contains the URL, which contains the token.
		return fmt.Errorf("telegram: request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram: %s", resp.Status)
	}
	return nil
}
