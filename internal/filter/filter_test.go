package filter

import (
	"testing"

	"github.com/muhammedshamil8/job-radar/internal/source"
)

func TestMentionsGo(t *testing.T) {
	tests := []struct {
		title, desc string
		want        bool
	}{
		{"Backend Engineer (Golang)", "", true},
		{"Go Developer", "", true},
		{"Software Engineer", "Experience building services in Go and PostgreSQL.", true},
		{"Software Engineer", "Languages: Go, Rust or Java.", true},
		{"Software Engineer", "We use Python/Go for our backend.", true},
		{"Software Engineer", "Strong golang skills.", true},
		{"Go-To-Market Manager", "", false},
		{"Software Engineer", "Go to market fast. Go beyond your role.", false},
		{"Software Engineer", "Go live in weeks. We go deep on quality.", false},
		{"Software Engineer", "Ready to Go? Apply now.", false},
		{"Software Engineer", "Java and Spring only.", false},
	}
	for _, tt := range tests {
		if got := MentionsGo(tt.title, tt.desc); got != tt.want {
			t.Errorf("MentionsGo(%q, %q) = %v, want %v", tt.title, tt.desc, got, tt.want)
		}
	}
}

func TestMinYears(t *testing.T) {
	tests := []struct {
		s    string
		want int
	}{
		{"2+ years of experience in Go", 2},
		{"0-4 years experience", 0},
		{"1 – 3 yrs", 1},
		{"5+ years required, 3+ years in Go", 3},
		{"Founded 20 years ago", -1},
		{"No experience figure here", -1},
	}
	for _, tt := range tests {
		if got := MinYears(tt.s); got != tt.want {
			t.Errorf("MinYears(%q) = %d, want %d", tt.s, got, tt.want)
		}
	}
}

func TestCheck(t *testing.T) {
	r := Rules{Locations: []string{"india", "bengaluru", "bangalore", "kerala"}, MaxMinYears: 3, SeniorYears: 5, ExcludeTitles: []string{"support"}}
	tests := []struct {
		name   string
		job    source.Job
		wantOK bool
	}{
		{"fits", source.Job{Title: "Backend Engineer", Location: "Bengaluru", Description: "2+ years building APIs in Go."}, true},
		{"senior", source.Job{Title: "Senior Backend Engineer", Location: "Bengaluru", Description: "Go"}, false},
		{"too many years", source.Job{Title: "Backend Engineer", Location: "Bangalore, India", Description: "4+ years with Go."}, false},
		{"wrong country", source.Job{Title: "Go Engineer", Location: "San Francisco, CA", Description: "1+ years"}, false},
		{"remote open to India", source.Job{Title: "Go Engineer", Location: "Remote", Description: "Open to candidates in India."}, true},
		{"intern", source.Job{Title: "Software Engineering Intern (Go)", Location: "India"}, false},
		{"level 3", source.Job{Title: "Software Engineer 3", Location: "India", Description: "Go"}, false},
		{"senior hidden behind smaller figure", source.Job{Title: "Infrastructure SRE", Location: "Bengaluru", Description: "5+ years in SRE, including 2+ years with Go tooling."}, false},
		{"range up to 4 is fine", source.Job{Title: "Software Engineer", Location: "Bengaluru", Description: "0-4 years experience in Go."}, true},
		{"title names another stack", source.Job{Title: "Software Engineer, Backend (Python)", Location: "India", Description: "We also use Go, Rust."}, false},
		{"title names Go and Python", source.Job{Title: "Backend Engineer (Python/Go)", Location: "India", Description: "1+ years"}, true},
		{"kerala backend title", source.Job{Title: "Node.js Developer", Location: "Technopark, Trivandrum, Kerala", TitleOnly: true}, true},
		{"kerala full stack", source.Job{Title: "Full Stack Developer (MERN)", Location: "Infopark, Kochi, Kerala", TitleOnly: true}, true},
		{"kerala non-backend", source.Job{Title: "Salesforce Functional Consultant", Location: "Technopark, Trivandrum, Kerala", TitleOnly: true}, false},
		{"kerala java", source.Job{Title: "Java Full Stack Developer [Java Angular]", Location: "Infopark, Kochi, Kerala", TitleOnly: true}, false},
		{"kerala python react", source.Job{Title: "Full Stack Developer - Python+ReactJS", Location: "Infopark, Kochi, Kerala", TitleOnly: true}, true},
		{"kerala senior", source.Job{Title: "Senior Backend Developer", Location: "Technopark, Trivandrum, Kerala", TitleOnly: true}, false},
		{"support", source.Job{Title: "Technical Support Engineer", Location: "India", Description: "Go"}, false},
	}
	for _, tt := range tests {
		if got := Check(r, tt.job); got.OK != tt.wantOK {
			t.Errorf("%s: OK = %v (%s), want %v", tt.name, got.OK, got.Reason, tt.wantOK)
		}
	}
}
