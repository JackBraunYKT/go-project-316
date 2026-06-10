package report

import (
	"encoding/json"
	"sort"
	"sync"
	"time"

	"code/internal/checker"
	"code/internal/parser"
)

type BrokenLink = checker.BrokenLink
type Asset = checker.Asset
type SEOReport = parser.SEOReport

type PageReport struct {
	URL          string       `json:"url"`
	Depth        int          `json:"depth"`
	HTTPStatus   int          `json:"http_status"`
	Status       string       `json:"status"`
	Error        string       `json:"error,omitempty"`
	BrokenLinks  []BrokenLink `json:"broken_links"`
	DiscoveredAt string       `json:"discovered_at"`
	SEO          *SEOReport   `json:"seo"`
	Assets       []Asset      `json:"assets"`
}

type Report struct {
	RootURL     string       `json:"root_url"`
	Depth       int          `json:"depth"`
	GeneratedAt string       `json:"generated_at"`
	Pages       []PageReport `json:"pages"`
}

// Sorted возвращает отчет с отсортированными страницами, ресурсами и битыми ссылками.
func (r Report) Sorted() Report {
	out := r

	sort.SliceStable(out.Pages, func(i, j int) bool {
		if out.Pages[i].Depth != out.Pages[j].Depth {
			return out.Pages[i].Depth < out.Pages[j].Depth
		}
		return out.Pages[i].URL < out.Pages[j].URL
	})

	for i := range out.Pages {
		page := &out.Pages[i]

		sort.SliceStable(page.Assets, func(i, j int) bool {
			return page.Assets[i].Type < page.Assets[j].Type
		})

		sort.SliceStable(page.BrokenLinks, func(i, j int) bool {
			return page.BrokenLinks[i].URL < page.BrokenLinks[j].URL
		})
	}

	return out
}

// Encode кодирует отчет в JSON с опциональными отступами.
func (r Report) Encode(indent bool) ([]byte, error) {
	sorted := r.Sorted()

	if indent {
		return json.MarshalIndent(sorted, "", "  ")
	}
	return json.Marshal(sorted)
}

type Builder struct {
	mu     sync.Mutex
	report Report
}

// NewBuilder создает построитель отчета для корневого URL и глубины обхода.
func NewBuilder(rootURL string, depth int) *Builder {
	return &Builder{
		report: Report{
			RootURL:     rootURL,
			Depth:       depth,
			GeneratedAt: now(),
			Pages:       make([]PageReport, 0),
		},
	}
}

// AddPage добавляет страницу в отчет.
func (b *Builder) AddPage(page PageReport) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if page.Error != "" {
		page.BrokenLinks = nil
		page.Assets = nil
	} else {
		if page.BrokenLinks == nil {
			page.BrokenLinks = []BrokenLink{}
		}
		if page.Assets == nil {
			page.Assets = []Asset{}
		}
	}
	if page.SEO == nil {
		page.SEO = &SEOReport{}
	}

	b.report.Pages = append(b.report.Pages, page)
}

// Build возвращает собранный отчет.
func (b *Builder) Build() Report {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.report
}

// NewPageReport создает отчет о странице с URL и глубиной обхода.
func NewPageReport(url string, depth int) PageReport {
	return PageReport{
		URL:          url,
		Depth:        depth,
		DiscoveredAt: now(),
	}
}

// NewErrorReport создает отчет с ошибкой для корневого URL.
func NewErrorReport(rootURL string, depth int, errMsg string) Report {
	return Report{
		RootURL:     rootURL,
		Depth:       depth,
		GeneratedAt: now(),
		Pages: []PageReport{
			{
				URL:          rootURL,
				Depth:        0,
				Status:       "error",
				Error:        errMsg,
				DiscoveredAt: now(),
			},
		},
	}
}

func now() string {
	return time.Now().UTC().Format(time.RFC3339)
}
