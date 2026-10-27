// Package filter decides whether a posting is a Go role at the right level
// and in the right place.
package filter

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/muhammedshamil8/job-radar/internal/source"
)

// Rules come from the "rules" block of companies.json.
type Rules struct {
	// Locations: a job passes if its location contains any of these (case-insensitive).
	Locations []string `json:"locations"`
	// MaxMinYears: drop jobs whose smallest stated experience is >= this. 0 disables.
	MaxMinYears int `json:"max_min_years"`
	// SeniorYears: drop jobs that mention any "N years" figure >= this, which catches
	// "5+ years, including 2+ years in X". 0 disables.
	SeniorYears int `json:"senior_years"`
	// ExcludeTitles: drop jobs whose title contains any of these (case-insensitive).
	ExcludeTitles []string `json:"exclude_titles"`
}

// Result explains why a job matched.
type Result struct {
	OK       bool
	MinYears int // smallest "N years" in the text, -1 if none found
	Reason   string
}

var (
	golangRE = regexp.MustCompile(`(?i)\bgo[ -]?lang\b`)
	// "Go" as a whole word, case-sensitive: "go" in lower case is almost always the verb.
	goWordRE = regexp.MustCompile(`\bGo\b`)
	gtmRE    = regexp.MustCompile(`(?i)go[- ]to[- ]market|\bGTM\b`)

	seniorRE = regexp.MustCompile(`(?i)\b(senior|sr\.?|staff|principal|lead|manager|director|head|architect|vp|distinguished|intern(ship)?|iii|l[5-9]|sde[- ]?3|engineer\s+3)\b`)

	// A title that names another stack ("Backend (Python)", "Frontend (React)") is not a Go role
	// even if the description mentions Go somewhere.
	otherStackRE = regexp.MustCompile(`(?i)\b(python|java|php|ruby|rails|node(\.js)?|react|angular|vue|\.net|c#|kotlin|swift|scala|flutter|frontend|front-end|android|ios|salesforce)\b`)

	// IT park portals list titles first; only developer titles are worth opening.
	devTitleRE = regexp.MustCompile(`(?i)\b(developer|engineer|programmer|sde|backend|back[- ]end|full[- ]?stack|golang)\b`)
	// Park titles built on a stack outside Go/Node/React/Vue are skipped before opening.
	parkOtherRE = regexp.MustCompile(`(?i)(\bjava\b|\.net|\bangular|\bc#|salesforce|power platform|\bphp\b|laravel|\bruby\b|django|wordpress|odoo|\bsap\b|servicenow|coldfusion|flutter|android|\bios\b|qa\b|test)`)
	parkMineRE  = regexp.MustCompile(`(?i)\b(golang|go|node(\.js|js)?|react(js)?|mern|typescript|vue)\b`)

	yearsRE = regexp.MustCompile(`(?i)(\d{1,2})\s*\+?\s*(?:(?:-|–|to)\s*\d{1,2}\s*\+?\s*)?(?:years|yrs)`)
)

// Words before "Go" that mark it as the language ("experience in Go").
var goBefore = map[string]bool{
	"in": true, "with": true, "using": true, "like": true, "as": true, "and": true, "or": true,
	"written": true, "e.g.": true, "eg": true, "including": true,
}

// Phrases after "Go" that mark it as the verb or an idiom.
var goAfterVerb = []string{
	" to ", "-to", " live", "-live", " beyond", " above", " ahead", " further",
	" deep", " back", " through", " out", " get", " for it", " big", " from ", " the extra",
}

// MentionsGo reports whether the title or description is about Go the language.
func MentionsGo(title, desc string) bool {
	if gtmRE.MatchString(title) {
		return false
	}
	if golangRE.MatchString(title) || goWordRE.MatchString(title) {
		return true
	}
	if golangRE.MatchString(desc) {
		return true
	}
	for _, loc := range goWordRE.FindAllStringIndex(desc, -1) {
		if goIsLanguage(desc, loc[0], loc[1]) {
			return true
		}
	}
	return false
}

func goIsLanguage(s string, start, end int) bool {
	after := strings.ToLower(s[end:min(len(s), end+12)])
	for _, p := range goAfterVerb {
		if strings.HasPrefix(after, p) {
			return false
		}
	}
	// Listed next to punctuation: "Go, Rust", "Go/Python", "(Go)".
	if len(after) > 0 && strings.ContainsRune(",/)", rune(after[0])) {
		return true
	}
	if start > 0 && strings.ContainsRune("(/,", rune(s[start-1])) {
		return true
	}
	// Previous word: "in Go", "with Go".
	before := strings.TrimRightFunc(s[:start], unicode.IsSpace)
	if i := strings.LastIndexFunc(before, unicode.IsSpace); i >= 0 {
		before = before[i+1:]
	}
	return goBefore[strings.ToLower(before)]
}

// yearFigures returns the lower bound of every "N years" / "N-M yrs" figure in s.
// Figures above 15 are ignored ("founded 20 years ago").
func yearFigures(s string) []int {
	var out []int
	for _, m := range yearsRE.FindAllStringSubmatch(s, -1) {
		if n, err := strconv.Atoi(m[1]); err == nil && n <= 15 {
			out = append(out, n)
		}
	}
	return out
}

// MinYears returns the smallest year figure in s, or -1.
func MinYears(s string) int {
	best := -1
	for _, n := range yearFigures(s) {
		if best == -1 || n < best {
			best = n
		}
	}
	return best
}

// MaxYears returns the largest year figure in s, or -1.
func MaxYears(s string) int {
	best := -1
	for _, n := range yearFigures(s) {
		best = max(best, n)
	}
	return best
}

// LocationOK reports whether loc (or a remote job open to India) fits the rules.
func LocationOK(r Rules, loc, desc string) bool {
	l := strings.ToLower(loc)
	for _, want := range r.Locations {
		if strings.Contains(l, strings.ToLower(want)) {
			return true
		}
	}
	// "Remote" with no country: keep it if the description mentions India.
	return strings.Contains(l, "remote") && strings.Contains(strings.ToLower(desc), "india")
}

func titleExcluded(r Rules, title string) bool {
	t := strings.ToLower(title)
	for _, w := range r.ExcludeTitles {
		if strings.Contains(t, strings.ToLower(w)) {
			return true
		}
	}
	return false
}

// ParkCandidate reports whether a title-only park job is worth opening to read its
// description: a developer title, not senior, not excluded, not built on another stack.
func ParkCandidate(r Rules, title string) bool {
	return devTitleRE.MatchString(title) &&
		!seniorRE.MatchString(title) &&
		!titleExcluded(r, title) &&
		!(parkOtherRE.MatchString(title) && !parkMineRE.MatchString(title))
}

// Check applies all rules to j.
func Check(r Rules, j source.Job) Result {
	res := Result{MinYears: MinYears(j.Title + " " + j.Description)}
	switch {
	case j.TitleOnly:
		res.Reason = "no description"
	case !MentionsGo(j.Title, j.Description):
		res.Reason = "no Go"
	case otherStackRE.MatchString(j.Title) && !golangRE.MatchString(j.Title) && !goWordRE.MatchString(j.Title):
		res.Reason = "title names another stack"
	case seniorRE.MatchString(j.Title):
		res.Reason = "senior/intern title"
	case titleExcluded(r, j.Title):
		res.Reason = "excluded title"
	case !LocationOK(r, j.Location, j.Description):
		res.Reason = "location"
	case r.MaxMinYears > 0 && res.MinYears >= r.MaxMinYears:
		res.Reason = "needs " + strconv.Itoa(res.MinYears) + "+ years"
	case r.SeniorYears > 0 && MaxYears(j.Title+" "+j.Description) >= r.SeniorYears:
		res.Reason = "mentions " + strconv.Itoa(MaxYears(j.Title+" "+j.Description)) + "+ years"
	default:
		res.OK = true
	}
	return res
}
