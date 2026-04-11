package report

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hjma/probex/internal/model"
	"github.com/hjma/probex/internal/store"
)

type Generator struct {
	store   store.Store
	dataDir string
	logger  *slog.Logger
}

func NewGenerator(s store.Store, dataDir string, logger *slog.Logger) *Generator {
	return &Generator{store: s, dataDir: dataDir, logger: logger}
}

type reportData struct {
	Report    *model.Report                `json:"report"`
	Tasks     []*model.Task                `json:"tasks"`
	Summaries map[string]*taskSummary      `json:"summaries"`
	Results   map[string][]*model.ProbeResult `json:"results"`
}

type taskSummary struct {
	TaskID      string  `json:"task_id"`
	TaskName    string  `json:"task_name"`
	Target      string  `json:"target"`
	ProbeType   string  `json:"probe_type"`
	Count       int     `json:"count"`
	SuccessRate float64 `json:"success_rate"`
	AvgLatency  float64 `json:"avg_latency_ms"`
	MinLatency  float64 `json:"min_latency_ms"`
	MaxLatency  float64 `json:"max_latency_ms"`
	AvgJitter   float64 `json:"avg_jitter_ms"`
	AvgLoss     float64 `json:"avg_loss_pct"`
}

func (g *Generator) Generate(rpt *model.Report) {
	ctx := context.Background()

	// Set status to generating
	g.store.UpdateReportStatus(ctx, rpt.ID, model.ReportStatusGenerating, "")

	data, err := g.collectData(ctx, rpt)
	if err != nil {
		g.logger.Error("report collect data", "id", rpt.ID, "error", err)
		g.store.UpdateReportStatus(ctx, rpt.ID, model.ReportStatusFailed, "")
		return
	}

	outDir := filepath.Join(g.dataDir, "reports")
	os.MkdirAll(outDir, 0755)

	var outPath string
	switch rpt.Format {
	case model.ReportFormatJSON:
		outPath = filepath.Join(outDir, rpt.ID+".json")
		err = g.generateJSON(outPath, data)
	case model.ReportFormatHTML:
		outPath = filepath.Join(outDir, rpt.ID+".html")
		err = g.generateHTML(outPath, data)
	default:
		outPath = filepath.Join(outDir, rpt.ID+".html")
		err = g.generateHTML(outPath, data)
	}

	if err != nil {
		g.logger.Error("report generate", "id", rpt.ID, "error", err)
		g.store.UpdateReportStatus(ctx, rpt.ID, model.ReportStatusFailed, "")
		return
	}

	g.store.UpdateReportStatus(ctx, rpt.ID, model.ReportStatusCompleted, outPath)
	g.logger.Info("report generated", "id", rpt.ID, "path", outPath)
}

func (g *Generator) collectData(ctx context.Context, rpt *model.Report) (*reportData, error) {
	data := &reportData{
		Report:    rpt,
		Tasks:     make([]*model.Task, 0),
		Summaries: make(map[string]*taskSummary),
		Results:   make(map[string][]*model.ProbeResult),
	}

	for _, taskID := range rpt.TaskIDs {
		task, err := g.store.GetTask(ctx, taskID)
		if err != nil {
			continue
		}
		data.Tasks = append(data.Tasks, task)

		filter := model.ResultFilter{
			TaskID: taskID,
			From:   rpt.TimeRangeStart,
			To:     rpt.TimeRangeEnd,
			Limit:  10000,
		}
		results, _, err := g.store.QueryResults(ctx, filter)
		if err != nil {
			continue
		}
		data.Results[taskID] = results

		summary := computeSummary(task, results)
		data.Summaries[taskID] = summary
	}

	return data, nil
}

func computeSummary(task *model.Task, results []*model.ProbeResult) *taskSummary {
	s := &taskSummary{
		TaskID:    task.ID,
		TaskName:  task.Name,
		Target:    task.Target,
		ProbeType: task.ProbeType,
		Count:     len(results),
	}
	if len(results) == 0 {
		return s
	}

	var successCount int
	var totalLatency, minLat, maxLat float64
	var totalJitter, totalLoss float64
	var jitterCount, lossCount int
	minLat = math.MaxFloat64

	for _, r := range results {
		if r.Success {
			successCount++
		}
		lat := 0.0
		if r.LatencyMs != nil {
			lat = *r.LatencyMs
		}
		totalLatency += lat
		if lat < minLat {
			minLat = lat
		}
		if lat > maxLat {
			maxLat = lat
		}
		if r.JitterMs != nil {
			totalJitter += *r.JitterMs
			jitterCount++
		}
		if r.PacketLossPct != nil {
			totalLoss += *r.PacketLossPct
			lossCount++
		}
	}

	n := float64(len(results))
	s.SuccessRate = float64(successCount) / n * 100
	s.AvgLatency = totalLatency / n
	s.MinLatency = minLat
	s.MaxLatency = maxLat
	if jitterCount > 0 {
		s.AvgJitter = totalJitter / float64(jitterCount)
	}
	if lossCount > 0 {
		s.AvgLoss = totalLoss / float64(lossCount)
	}
	return s
}

func (g *Generator) generateJSON(path string, data *reportData) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}

func (g *Generator) generateHTML(path string, data *reportData) error {
	var b strings.Builder

	b.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>ProbeX Report - ` + html.EscapeString(data.Report.Name) + `</title>
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; background: #f9fafb; color: #1f2937; padding: 2rem; }
  .container { max-width: 960px; margin: 0 auto; }
  h1 { font-size: 1.75rem; margin-bottom: 0.5rem; }
  h2 { font-size: 1.25rem; margin: 1.5rem 0 0.75rem; color: #374151; }
  .meta { color: #6b7280; font-size: 0.875rem; margin-bottom: 1.5rem; }
  .card { background: #fff; border: 1px solid #e5e7eb; border-radius: 8px; padding: 1.25rem; margin-bottom: 1rem; }
  table { width: 100%; border-collapse: collapse; font-size: 0.875rem; }
  th, td { padding: 0.5rem 0.75rem; text-align: left; border-bottom: 1px solid #f3f4f6; }
  th { font-weight: 600; background: #f9fafb; }
  .badge { display: inline-block; padding: 2px 8px; border-radius: 4px; font-size: 0.75rem; font-weight: 500; }
  .badge-ok { background: #dcfce7; color: #166534; }
  .badge-warn { background: #fef3c7; color: #92400e; }
  .badge-crit { background: #fee2e2; color: #991b1b; }
  .bar-chart { display: flex; align-items: flex-end; gap: 4px; height: 120px; margin-top: 0.5rem; }
  .bar { background: #3b82f6; border-radius: 2px 2px 0 0; min-width: 24px; position: relative; }
  .bar-label { font-size: 0.65rem; color: #6b7280; text-align: center; margin-top: 2px; }
  .stats-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(140px, 1fr)); gap: 0.75rem; }
  .stat { text-align: center; }
  .stat-value { font-size: 1.5rem; font-weight: 700; color: #1e293b; }
  .stat-label { font-size: 0.75rem; color: #6b7280; }
</style>
</head>
<body>
<div class="container">
`)

	// Header
	b.WriteString(fmt.Sprintf(`<h1>%s</h1>`, html.EscapeString(data.Report.Name)))
	b.WriteString(fmt.Sprintf(`<p class="meta">Generated: %s &bull; Time Range: %s to %s &bull; Tasks: %d</p>`,
		time.Now().Format("2006-01-02 15:04:05"),
		data.Report.TimeRangeStart.Format("2006-01-02 15:04"),
		data.Report.TimeRangeEnd.Format("2006-01-02 15:04"),
		len(data.Tasks),
	))

	// Summary overview
	b.WriteString(`<h2>Summary</h2>`)
	for _, task := range data.Tasks {
		s := data.Summaries[task.ID]
		if s == nil {
			continue
		}

		successBadge := "badge-ok"
		if s.SuccessRate < 95 {
			successBadge = "badge-warn"
		}
		if s.SuccessRate < 80 {
			successBadge = "badge-crit"
		}

		b.WriteString(`<div class="card">`)
		b.WriteString(fmt.Sprintf(`<h2 style="margin-top:0">%s <span style="font-size:0.8rem;color:#6b7280">(%s → %s)</span></h2>`,
			html.EscapeString(s.TaskName), html.EscapeString(s.ProbeType), html.EscapeString(s.Target)))

		b.WriteString(`<div class="stats-grid">`)
		b.WriteString(fmt.Sprintf(`<div class="stat"><div class="stat-value">%d</div><div class="stat-label">Probes</div></div>`, s.Count))
		b.WriteString(fmt.Sprintf(`<div class="stat"><div class="stat-value"><span class="badge %s">%.1f%%</span></div><div class="stat-label">Success Rate</div></div>`, successBadge, s.SuccessRate))
		b.WriteString(fmt.Sprintf(`<div class="stat"><div class="stat-value">%.1f</div><div class="stat-label">Avg Latency (ms)</div></div>`, s.AvgLatency))
		b.WriteString(fmt.Sprintf(`<div class="stat"><div class="stat-value">%.1f</div><div class="stat-label">Min Latency (ms)</div></div>`, s.MinLatency))
		b.WriteString(fmt.Sprintf(`<div class="stat"><div class="stat-value">%.1f</div><div class="stat-label">Max Latency (ms)</div></div>`, s.MaxLatency))
		b.WriteString(fmt.Sprintf(`<div class="stat"><div class="stat-value">%.2f</div><div class="stat-label">Avg Jitter (ms)</div></div>`, s.AvgJitter))
		b.WriteString(fmt.Sprintf(`<div class="stat"><div class="stat-value">%.2f%%</div><div class="stat-label">Avg Loss</div></div>`, s.AvgLoss))
		b.WriteString(`</div>`)

		// Latency bar chart (sample up to 30 data points)
		results := data.Results[task.ID]
		if len(results) > 0 {
			b.WriteString(`<h2>Latency Over Time</h2>`)
			sampled := sampleResults(results, 30)
			maxLat := 0.0
			for _, r := range sampled {
				lat := 0.0
				if r.LatencyMs != nil {
					lat = *r.LatencyMs
				}
				if lat > maxLat {
					maxLat = lat
				}
			}
			if maxLat == 0 {
				maxLat = 1
			}
			b.WriteString(`<div class="bar-chart">`)
			for _, r := range sampled {
				lat := 0.0
				if r.LatencyMs != nil {
					lat = *r.LatencyMs
				}
				h := lat / maxLat * 100
				if h < 2 {
					h = 2
				}
				color := "#3b82f6"
				if !r.Success {
					color = "#ef4444"
				}
				b.WriteString(fmt.Sprintf(`<div style="flex:1;text-align:center"><div class="bar" style="height:%.0f%%;background:%s" title="%.1fms at %s"></div><div class="bar-label">%s</div></div>`,
					h, color, lat, r.Timestamp.Format("15:04"), r.Timestamp.Format("15:04")))
			}
			b.WriteString(`</div>`)
		}

		b.WriteString(`</div>`)
	}

	// Detail tables
	b.WriteString(`<h2>Detailed Results</h2>`)
	for _, task := range data.Tasks {
		results := data.Results[task.ID]
		if len(results) == 0 {
			continue
		}
		b.WriteString(fmt.Sprintf(`<div class="card"><h2 style="margin-top:0">%s</h2>`, html.EscapeString(task.Name)))
		b.WriteString(`<div style="max-height:400px;overflow-y:auto">`)
		b.WriteString(`<table><thead><tr><th>Time</th><th>Status</th><th>Latency</th><th>Jitter</th><th>Loss</th><th>Error</th></tr></thead><tbody>`)
		display := results
		if len(display) > 100 {
			display = display[:100]
		}
		for _, r := range display {
			status := `<span style="color:#22c55e">OK</span>`
			if !r.Success {
				status = `<span style="color:#ef4444">FAIL</span>`
			}
			jitter := "-"
			if r.JitterMs != nil {
				jitter = fmt.Sprintf("%.2fms", *r.JitterMs)
			}
			loss := "-"
			if r.PacketLossPct != nil {
				loss = fmt.Sprintf("%.1f%%", *r.PacketLossPct)
			}
			errStr := "-"
			if r.Error != "" {
				errStr = html.EscapeString(r.Error)
				if len(errStr) > 50 {
					errStr = errStr[:50] + "..."
				}
			}
			latStr := "-"
			if r.LatencyMs != nil {
				latStr = fmt.Sprintf("%.1fms", *r.LatencyMs)
			}
			b.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>`,
				r.Timestamp.Format("2006-01-02 15:04:05"), status, latStr, jitter, loss, errStr))
		}
		b.WriteString(`</tbody></table>`)
		if len(results) > 100 {
			b.WriteString(fmt.Sprintf(`<p style="padding:0.5rem;color:#6b7280;font-size:0.8rem">Showing 100 of %d results</p>`, len(results)))
		}
		b.WriteString(`</div></div>`)
	}

	b.WriteString(`
</div>
</body>
</html>`)

	return os.WriteFile(path, []byte(b.String()), 0644)
}

func sampleResults(results []*model.ProbeResult, maxPoints int) []*model.ProbeResult {
	if len(results) <= maxPoints {
		return results
	}
	step := float64(len(results)) / float64(maxPoints)
	sampled := make([]*model.ProbeResult, 0, maxPoints)
	for i := 0; i < maxPoints; i++ {
		idx := int(float64(i) * step)
		if idx >= len(results) {
			idx = len(results) - 1
		}
		sampled = append(sampled, results[idx])
	}
	return sampled
}
