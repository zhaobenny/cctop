package output

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"unsafe"

	"github.com/zhaobenny/cctop/internal/model"
)

const (
	compactThreshold = 100 // Terminal width below which compact mode kicks in
	defaultWidth     = 120
)

// TableOptions controls table display behavior
type TableOptions struct {
	ForceCompact bool
}

// winsize struct for ioctl TIOCGWINSZ
type winsize struct {
	Row    uint16
	Col    uint16
	Xpixel uint16
	Ypixel uint16
}

// getTerminalWidth returns the current terminal width
func getTerminalWidth() int {
	// Check COLUMNS env var first
	if cols := os.Getenv("COLUMNS"); cols != "" {
		var width int
		if _, err := fmt.Sscanf(cols, "%d", &width); err == nil && width > 0 {
			return width
		}
	}

	// Try to get from terminal using ioctl
	ws := &winsize{}
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL,
		uintptr(syscall.Stdout),
		uintptr(syscall.TIOCGWINSZ),
		uintptr(unsafe.Pointer(ws)))
	if errno == 0 && ws.Col > 0 {
		return int(ws.Col)
	}

	return defaultWidth
}

// shouldUseCompact determines if compact mode should be used
func shouldUseCompact(opts TableOptions) bool {
	if opts.ForceCompact {
		return true
	}
	return getTerminalWidth() < compactThreshold
}

// FormatNumber formats a number with thousand separators
func FormatNumber(n int64) string {
	if n == 0 {
		return "0"
	}

	str := fmt.Sprintf("%d", n)
	negative := n < 0
	if negative {
		str = str[1:]
	}

	result := ""
	for i, c := range str {
		if i > 0 && (len(str)-i)%3 == 0 {
			result += ","
		}
		result += string(c)
	}

	if negative {
		return "-" + result
	}
	return result
}

// FormatCost formats a cost value as currency
func FormatCost(cost float64) string {
	return fmt.Sprintf("$%.2f", cost)
}

// shortenModelName converts full model names to short form
// claude-sonnet-4-5-20250929 -> sonnet-4-5
// claude-opus-4-20250514 -> opus-4
func shortenModelName(name string) string {
	// Pattern: claude-{type}-{version}-{date}
	// e.g., claude-sonnet-4-5-20250929 -> sonnet-4-5
	re := regexp.MustCompile(`^claude-(\w+)-([\d-]+)-(\d{8})$`)
	if matches := re.FindStringSubmatch(name); matches != nil {
		return fmt.Sprintf("%s-%s", matches[1], matches[2])
	}

	// Pattern without date: claude-{type}-{version}
	// e.g., claude-opus-4-5 -> opus-4-5
	re2 := regexp.MustCompile(`^claude-(\w+)-([\d-]+)$`)
	if matches := re2.FindStringSubmatch(name); matches != nil {
		return fmt.Sprintf("%s-%s", matches[1], matches[2])
	}

	// Pattern: anthropic/claude-{type}-{version}
	// e.g., anthropic/claude-opus-4.5 -> opus-4.5
	re3 := regexp.MustCompile(`^anthropic/claude-(\w+)-([\d.]+)$`)
	if matches := re3.FindStringSubmatch(name); matches != nil {
		return fmt.Sprintf("%s-%s", matches[1], matches[2])
	}

	return name
}

// shortenSessionID truncates session UUID to first 8 chars
func shortenSessionID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// PrintTable prints aggregated usage as a formatted table
func PrintTable(results []model.AggregatedUsage, title string, showTotal bool) {
	PrintTableWithOptions(results, title, showTotal, TableOptions{})
}

// PrintTableWithOptions prints table with display options
func PrintTableWithOptions(results []model.AggregatedUsage, title string, showTotal bool, opts TableOptions) {
	if len(results) == 0 {
		fmt.Println("No usage data found.")
		return
	}

	compact := shouldUseCompact(opts)

	// Determine if this is a session view (UUIDs need shortening)
	isSessionView := title == "Session"

	// Calculate key column width
	keyWidth := len(title)
	for _, r := range results {
		key := r.Key
		if isSessionView && compact {
			key = shortenSessionID(key)
		}
		if len(key) > keyWidth {
			keyWidth = len(key)
		}
	}
	if keyWidth < 10 {
		keyWidth = 10
	}
	// Cap key width in compact mode
	if compact && keyWidth > 12 {
		keyWidth = 12
	}

	fmt.Println()

	if compact {
		// Compact: Key, Input, Output, Cost
		fmt.Printf("%-*s  %12s  %12s  %10s\n",
			keyWidth, title, "Input", "Output", "Cost")
		fmt.Println(strings.Repeat("─", keyWidth+2+12+2+12+2+10))

		for _, r := range results {
			key := r.Key
			if isSessionView {
				key = shortenSessionID(key)
			}
			if len(key) > keyWidth {
				key = key[:keyWidth]
			}
			fmt.Printf("%-*s  %12s  %12s  %10s\n",
				keyWidth, key,
				FormatNumber(r.Usage.InputTokens),
				FormatNumber(r.Usage.OutputTokens),
				FormatCost(r.Cost))
		}

		if showTotal && len(results) > 1 {
			fmt.Println(strings.Repeat("─", keyWidth+2+12+2+12+2+10))

			var total model.TokenUsage
			var totalCost float64
			for _, r := range results {
				total.InputTokens += r.Usage.InputTokens
				total.OutputTokens += r.Usage.OutputTokens
				totalCost += r.Cost
			}

			fmt.Printf("%-*s  %12s  %12s  %10s\n",
				keyWidth, "Total",
				FormatNumber(total.InputTokens),
				FormatNumber(total.OutputTokens),
				FormatCost(totalCost))
		}

		fmt.Println()
		fmt.Println("(Compact mode - expand terminal for full view)")
	} else {
		// Full: Key, Input, Output, Cache Create, Cache Read, Cost
		fmt.Printf("%-*s  %12s  %12s  %14s  %14s  %10s\n",
			keyWidth, title, "Input", "Output", "Cache Create", "Cache Read", "Cost")
		fmt.Println(strings.Repeat("─", keyWidth+2+12+2+12+2+14+2+14+2+10))

		for _, r := range results {
			key := r.Key
			if isSessionView {
				key = shortenSessionID(key)
			}
			fmt.Printf("%-*s  %12s  %12s  %14s  %14s  %10s\n",
				keyWidth, key,
				FormatNumber(r.Usage.InputTokens),
				FormatNumber(r.Usage.OutputTokens),
				FormatNumber(r.Usage.CacheCreationInputTokens),
				FormatNumber(r.Usage.CacheReadInputTokens),
				FormatCost(r.Cost))
		}

		if showTotal && len(results) > 1 {
			fmt.Println(strings.Repeat("─", keyWidth+2+12+2+12+2+14+2+14+2+10))

			var total model.TokenUsage
			var totalCost float64
			for _, r := range results {
				total.InputTokens += r.Usage.InputTokens
				total.OutputTokens += r.Usage.OutputTokens
				total.CacheCreationInputTokens += r.Usage.CacheCreationInputTokens
				total.CacheReadInputTokens += r.Usage.CacheReadInputTokens
				totalCost += r.Cost
			}

			fmt.Printf("%-*s  %12s  %12s  %14s  %14s  %10s\n",
				keyWidth, "Total",
				FormatNumber(total.InputTokens),
				FormatNumber(total.OutputTokens),
				FormatNumber(total.CacheCreationInputTokens),
				FormatNumber(total.CacheReadInputTokens),
				FormatCost(totalCost))
		}

		fmt.Println()
	}
}

// PrintTableWithBreakdown prints table with per-model breakdown
func PrintTableWithBreakdown(results []model.AggregatedUsage, title string) {
	PrintTableWithBreakdownOpts(results, title, TableOptions{})
}

// PrintTableWithBreakdownOpts prints table with breakdown and options
func PrintTableWithBreakdownOpts(results []model.AggregatedUsage, title string, opts TableOptions) {
	PrintTableWithOptions(results, title, true, opts)

	// Print model breakdown with shortened names
	modelsMap := make(map[string]bool)
	for _, r := range results {
		for _, m := range r.Models {
			modelsMap[shortenModelName(m)] = true
		}
	}

	if len(modelsMap) > 0 {
		var models []string
		for m := range modelsMap {
			models = append(models, m)
		}
		sort.Strings(models)

		fmt.Println("Models used:")
		for _, m := range models {
			fmt.Printf("  - %s\n", m)
		}
		fmt.Println()
	}
}

// JSONOutput represents the JSON output structure
type JSONOutput struct {
	Results []JSONResult `json:"results"`
	Total   JSONResult   `json:"total"`
}

// JSONResult represents a single result in JSON format
type JSONResult struct {
	Key                      string   `json:"key"`
	InputTokens              int64    `json:"input_tokens"`
	OutputTokens             int64    `json:"output_tokens"`
	CacheCreationInputTokens int64    `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64    `json:"cache_read_input_tokens"`
	Cost                     float64  `json:"cost"`
	Models                   []string `json:"models,omitempty"`
}

// PrintJSON outputs results as JSON
func PrintJSON(results []model.AggregatedUsage) {
	output := JSONOutput{
		Results: make([]JSONResult, len(results)),
	}

	var total model.TokenUsage
	var totalCost float64
	modelsMap := make(map[string]bool)

	for i, r := range results {
		output.Results[i] = JSONResult{
			Key:                      r.Key,
			InputTokens:              r.Usage.InputTokens,
			OutputTokens:             r.Usage.OutputTokens,
			CacheCreationInputTokens: r.Usage.CacheCreationInputTokens,
			CacheReadInputTokens:     r.Usage.CacheReadInputTokens,
			Cost:                     r.Cost,
			Models:                   r.Models,
		}

		total.InputTokens += r.Usage.InputTokens
		total.OutputTokens += r.Usage.OutputTokens
		total.CacheCreationInputTokens += r.Usage.CacheCreationInputTokens
		total.CacheReadInputTokens += r.Usage.CacheReadInputTokens
		totalCost += r.Cost

		for _, m := range r.Models {
			modelsMap[m] = true
		}
	}

	var models []string
	for m := range modelsMap {
		models = append(models, m)
	}

	output.Total = JSONResult{
		Key:                      "total",
		InputTokens:              total.InputTokens,
		OutputTokens:             total.OutputTokens,
		CacheCreationInputTokens: total.CacheCreationInputTokens,
		CacheReadInputTokens:     total.CacheReadInputTokens,
		Cost:                     totalCost,
		Models:                   models,
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	encoder.Encode(output)
}
