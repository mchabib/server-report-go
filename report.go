package main

import (
	"fmt"
	"strings"
	"time"
)

// diskWarningPercent is the used-disk threshold above which an UP instance
// is still flagged (🟡) even though it's not DOWN.
const diskWarningPercent = 90.0

// buildReport renders one client's report as Telegram Markdown text.
func buildReport(clientName string, metrics []instanceMetrics, now time.Time) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "📊 *Laporan Kondisi Server — %s*\n", escapeMarkdown(clientName))
	fmt.Fprintf(&sb, "🕒 %s\n\n", now.Format("2 January 2006, 15:04"))

	for _, m := range metrics {
		label := escapeMarkdown(m.Instance)
		if m.Job != "" {
			label = fmt.Sprintf("%s (%s)", escapeMarkdown(m.Job), label)
		}

		if !m.Up {
			fmt.Fprintf(&sb, "🔴 *%s* — DOWN\n\n", label)
			continue
		}

		icon := "🟢"
		if m.HasStats && m.DiskUsedPercent > diskWarningPercent {
			icon = "🟡"
		}

		fmt.Fprintf(&sb, "%s *%s* — UP\n", icon, label)
		if m.HasStats {
			fmt.Fprintf(&sb, "   CPU: %.1f%% | RAM: %.1f%% | Disk: %.1f%% (%.1f GB tersisa)\n\n", m.CPU, m.Mem, m.DiskUsedPercent, m.DiskFreeGB)
		} else {
			sb.WriteString("   (metrik CPU/RAM/Disk tidak tersedia)\n\n")
		}
	}

	return strings.TrimRight(sb.String(), "\n")
}

// escapeMarkdown neutralizes Telegram legacy-Markdown control characters
// that could otherwise leak from a client/instance name into formatting.
func escapeMarkdown(s string) string {
	r := strings.NewReplacer(
		"_", `\_`,
		"*", `\*`,
		"`", "\\`",
		"[", `\[`,
	)
	return r.Replace(s)
}
