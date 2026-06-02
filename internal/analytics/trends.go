package analytics

import (
	"context"
	"time"
)

type TrendPoint struct {
	Date    string  `json:"date"`
	CostUSD float64 `json:"cost_usd"`
	MA7     float64 `json:"ma7"`
}

// Trends returns per-day cost over the last `days` days, with a 7-day moving average.
func (e *Engine) Trends(ctx context.Context, days int) ([]TrendPoint, error) {
	if days <= 0 {
		days = 30
	}
	rows, err := e.Daily(ctx, days+7) // need 6 extra days of leading buffer for MA
	if err != nil {
		return nil, err
	}

	// rows are DESC by date — reverse for chronological MA computation.
	asc := make([]DailyRow, len(rows))
	for i, r := range rows {
		asc[len(rows)-1-i] = r
	}

	pts := make([]TrendPoint, 0, len(asc))
	for i, r := range asc {
		pt := TrendPoint{Date: r.Date, CostUSD: r.CostUSD}
		// 7-day trailing MA (inclusive of current day).
		start := i - 6
		if start < 0 {
			start = 0
		}
		var sum float64
		for j := start; j <= i; j++ {
			sum += asc[j].CostUSD
		}
		pt.MA7 = sum / float64(i-start+1)
		pts = append(pts, pt)
	}

	// Trim leading buffer so the response is exactly `days` long if available.
	if len(pts) > days {
		pts = pts[len(pts)-days:]
	}
	return pts, nil
}

type Projection struct {
	BasisDays         int     `json:"basis_days"`
	BasisDailyAvgUSD  float64 `json:"basis_daily_avg_usd"`
	DaysInMonth       int     `json:"days_in_month"`
	DaysElapsed       int     `json:"days_elapsed"`
	MonthToDateUSD    float64 `json:"month_to_date_usd"`
	ProjectedMonthUSD float64 `json:"projected_month_usd"`
	ProjectedAt       string  `json:"projected_at"`
}

// Projection computes:
//
//	basis = trailing 7-day average daily cost
//	projected_month_usd = month_to_date + basis × (days_in_month − days_elapsed)
func (e *Engine) Projection(ctx context.Context) (*Projection, error) {
	loc := e.Cfg().Location()
	now := time.Now().In(loc)
	year, month, day := now.Date()
	startOfMonth := time.Date(year, month, 1, 0, 0, 0, 0, loc).UTC()
	startOfWindow := time.Date(year, month, day, 0, 0, 0, 0, loc).Add(-7 * 24 * time.Hour).UTC()

	// MTD cost.
	mtd, err := e.totals(ctx, startOfMonth, time.Time{})
	if err != nil {
		return nil, err
	}
	// Trailing-7-day cost.
	t7, err := e.totals(ctx, startOfWindow, time.Time{})
	if err != nil {
		return nil, err
	}

	dim := daysInMonth(year, month)
	avg := t7.CostUSD / 7.0
	remaining := dim - day

	return &Projection{
		BasisDays:         7,
		BasisDailyAvgUSD:  avg,
		DaysInMonth:       dim,
		DaysElapsed:       day,
		MonthToDateUSD:    mtd.CostUSD,
		ProjectedMonthUSD: mtd.CostUSD + avg*float64(remaining),
		ProjectedAt:       now.UTC().Format(time.RFC3339),
	}, nil
}

func daysInMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}
