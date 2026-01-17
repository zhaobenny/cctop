package aggregator

import (
	"sort"
	"time"

	"github.com/zhaobenny/cctop/internal/model"
	"github.com/zhaobenny/cctop/internal/pricing"
)

// Options for aggregation
type Options struct {
	Since    time.Time
	Until    time.Time
	Timezone *time.Location
	Offline  bool
}

// FilterRecords filters records based on date range
func FilterRecords(records []model.UsageRecord, opts Options) []model.UsageRecord {
	var filtered []model.UsageRecord
	for _, r := range records {
		ts := r.Timestamp
		if opts.Timezone != nil {
			ts = ts.In(opts.Timezone)
		}
		if !opts.Since.IsZero() && ts.Before(opts.Since) {
			continue
		}
		if !opts.Until.IsZero() && ts.After(opts.Until) {
			continue
		}
		filtered = append(filtered, r)
	}
	return filtered
}

// ByDay aggregates usage by day
func ByDay(records []model.UsageRecord, opts Options) []model.AggregatedUsage {
	grouped := make(map[string]*model.AggregatedUsage)
	modelsMap := make(map[string]map[string]bool)

	for _, r := range records {
		ts := r.Timestamp
		if opts.Timezone != nil {
			ts = ts.In(opts.Timezone)
		}
		key := ts.Format("2006-01-02")

		if _, ok := grouped[key]; !ok {
			grouped[key] = &model.AggregatedUsage{Key: key}
			modelsMap[key] = make(map[string]bool)
		}

		agg := grouped[key]
		agg.Usage.InputTokens += r.Usage.InputTokens
		agg.Usage.OutputTokens += r.Usage.OutputTokens
		agg.Usage.CacheCreationInputTokens += r.Usage.CacheCreationInputTokens
		agg.Usage.CacheReadInputTokens += r.Usage.CacheReadInputTokens
		agg.RecordCount++

		p := pricing.GetPricing(r.Model, opts.Offline)
		agg.Cost += pricing.CalculateCost(r.Usage, p)

		modelsMap[key][r.Model] = true
	}

	// Convert models map to slice and sort results
	var results []model.AggregatedUsage
	for key, agg := range grouped {
		for m := range modelsMap[key] {
			agg.Models = append(agg.Models, m)
		}
		sort.Strings(agg.Models)
		results = append(results, *agg)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Key > results[j].Key // Newest first
	})

	return results
}

// ByMonth aggregates usage by month
func ByMonth(records []model.UsageRecord, opts Options) []model.AggregatedUsage {
	grouped := make(map[string]*model.AggregatedUsage)
	modelsMap := make(map[string]map[string]bool)

	for _, r := range records {
		ts := r.Timestamp
		if opts.Timezone != nil {
			ts = ts.In(opts.Timezone)
		}
		key := ts.Format("2006-01")

		if _, ok := grouped[key]; !ok {
			grouped[key] = &model.AggregatedUsage{Key: key}
			modelsMap[key] = make(map[string]bool)
		}

		agg := grouped[key]
		agg.Usage.InputTokens += r.Usage.InputTokens
		agg.Usage.OutputTokens += r.Usage.OutputTokens
		agg.Usage.CacheCreationInputTokens += r.Usage.CacheCreationInputTokens
		agg.Usage.CacheReadInputTokens += r.Usage.CacheReadInputTokens
		agg.RecordCount++

		p := pricing.GetPricing(r.Model, opts.Offline)
		agg.Cost += pricing.CalculateCost(r.Usage, p)

		modelsMap[key][r.Model] = true
	}

	var results []model.AggregatedUsage
	for key, agg := range grouped {
		for m := range modelsMap[key] {
			agg.Models = append(agg.Models, m)
		}
		sort.Strings(agg.Models)
		results = append(results, *agg)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Key > results[j].Key
	})

	return results
}

// BySession aggregates usage by session ID
func BySession(records []model.UsageRecord, opts Options) []model.AggregatedUsage {
	grouped := make(map[string]*model.AggregatedUsage)
	modelsMap := make(map[string]map[string]bool)
	sessionTimes := make(map[string]time.Time)

	for _, r := range records {
		key := r.SessionID
		if key == "" {
			key = "unknown"
		}

		if _, ok := grouped[key]; !ok {
			grouped[key] = &model.AggregatedUsage{Key: key}
			modelsMap[key] = make(map[string]bool)
			sessionTimes[key] = r.Timestamp
		}

		// Track the most recent timestamp for sorting
		if r.Timestamp.After(sessionTimes[key]) {
			sessionTimes[key] = r.Timestamp
		}

		agg := grouped[key]
		agg.Usage.InputTokens += r.Usage.InputTokens
		agg.Usage.OutputTokens += r.Usage.OutputTokens
		agg.Usage.CacheCreationInputTokens += r.Usage.CacheCreationInputTokens
		agg.Usage.CacheReadInputTokens += r.Usage.CacheReadInputTokens
		agg.RecordCount++

		p := pricing.GetPricing(r.Model, opts.Offline)
		agg.Cost += pricing.CalculateCost(r.Usage, p)

		modelsMap[key][r.Model] = true
	}

	var results []model.AggregatedUsage
	for key, agg := range grouped {
		for m := range modelsMap[key] {
			agg.Models = append(agg.Models, m)
		}
		sort.Strings(agg.Models)
		results = append(results, *agg)
	}

	// Sort by most recent activity
	sort.Slice(results, func(i, j int) bool {
		return sessionTimes[results[i].Key].After(sessionTimes[results[j].Key])
	})

	return results
}

// ByBlock aggregates usage by 5-hour billing windows
// Blocks start at midnight UTC: 00:00, 05:00, 10:00, 15:00, 20:00
func ByBlock(records []model.UsageRecord, opts Options) []model.AggregatedUsage {
	grouped := make(map[string]*model.AggregatedUsage)
	modelsMap := make(map[string]map[string]bool)

	for _, r := range records {
		ts := r.Timestamp.UTC()

		// Calculate block start time
		hour := ts.Hour()
		blockHour := (hour / 5) * 5
		blockStart := time.Date(ts.Year(), ts.Month(), ts.Day(), blockHour, 0, 0, 0, time.UTC)
		key := blockStart.Format("2006-01-02 15:04")

		if _, ok := grouped[key]; !ok {
			grouped[key] = &model.AggregatedUsage{Key: key}
			modelsMap[key] = make(map[string]bool)
		}

		agg := grouped[key]
		agg.Usage.InputTokens += r.Usage.InputTokens
		agg.Usage.OutputTokens += r.Usage.OutputTokens
		agg.Usage.CacheCreationInputTokens += r.Usage.CacheCreationInputTokens
		agg.Usage.CacheReadInputTokens += r.Usage.CacheReadInputTokens
		agg.RecordCount++

		p := pricing.GetPricing(r.Model, opts.Offline)
		agg.Cost += pricing.CalculateCost(r.Usage, p)

		modelsMap[key][r.Model] = true
	}

	var results []model.AggregatedUsage
	for key, agg := range grouped {
		for m := range modelsMap[key] {
			agg.Models = append(agg.Models, m)
		}
		sort.Strings(agg.Models)
		results = append(results, *agg)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Key > results[j].Key
	})

	return results
}

// CalculateTotal returns the total aggregated usage
func CalculateTotal(results []model.AggregatedUsage) model.AggregatedUsage {
	total := model.AggregatedUsage{Key: "Total"}
	modelsMap := make(map[string]bool)

	for _, r := range results {
		total.Usage.InputTokens += r.Usage.InputTokens
		total.Usage.OutputTokens += r.Usage.OutputTokens
		total.Usage.CacheCreationInputTokens += r.Usage.CacheCreationInputTokens
		total.Usage.CacheReadInputTokens += r.Usage.CacheReadInputTokens
		total.Cost += r.Cost
		total.RecordCount += r.RecordCount

		for _, m := range r.Models {
			modelsMap[m] = true
		}
	}

	for m := range modelsMap {
		total.Models = append(total.Models, m)
	}
	sort.Strings(total.Models)

	return total
}
