package templates

import (
	"embed"
	"fmt"
	"html/template"
	"strings"
	"time"
)

//go:embed *.html partials/*.html
var FS embed.FS

// Parse returns the parsed templates with custom functions
func Parse() (*template.Template, error) {
	funcMap := template.FuncMap{
		"formatNumber": formatNumber,
		"formatCost":   formatCost,
		"formatDate":   formatDate,
		"seq":          seq,
	}

	return template.New("").Funcs(funcMap).ParseFS(FS, "*.html", "partials/*.html")
}

// seq generates a sequence from start to end (inclusive)
func seq(start, end int) []int {
	result := make([]int, 0, end-start+1)
	for i := start; i <= end; i++ {
		result = append(result, i)
	}
	return result
}

func formatNumber(n int64) string {
	if n == 0 {
		return "0"
	}

	str := fmt.Sprintf("%d", n)
	negative := n < 0
	if negative {
		str = str[1:]
	}

	var result strings.Builder
	for i, c := range str {
		if i > 0 && (len(str)-i)%3 == 0 {
			result.WriteRune(',')
		}
		result.WriteRune(c)
	}

	if negative {
		return "-" + result.String()
	}
	return result.String()
}

func formatCost(cost float64) string {
	return fmt.Sprintf("$%.2f", cost)
}

func formatDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("Jan 2")
}
