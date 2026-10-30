package main

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/puppe1990/libgenbooks/internal/libgen"
)

func printEntries(w io.Writer, entries []libgen.Entry) {
	table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(table, "ID\tMD5\tTÍTULO\tAUTOR\tANO\tEXT\tTAMANHO")
	for _, entry := range entries {
		_, _ = fmt.Fprintf(table, "%d\t%s\t%s\t%s\t%s\t%s\t%s\n",
			entry.ID,
			entry.MD5,
			truncate(entry.Title, 60),
			truncate(entry.Author, 32),
			yearText(entry.Year),
			entry.Extension,
			sizeText(entry),
		)
	}
	_ = table.Flush()
}

func truncate(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit-1]) + "…"
}

func yearText(year int) string {
	if year == 0 {
		return "—"
	}
	return fmt.Sprint(year)
}

func sizeText(entry libgen.Entry) string {
	if entry.FilesizeText != "" {
		return entry.FilesizeText
	}
	return humanBytes(entry.Filesize)
}

// humanBytes formats with the same decimal units the mirror uses.
func humanBytes(bytes int64) string {
	switch {
	case bytes >= 1000*1000*1000:
		return fmt.Sprintf("%.1f GB", float64(bytes)/(1000*1000*1000))
	case bytes >= 1000*1000:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1000*1000))
	case bytes >= 1000:
		return fmt.Sprintf("%.0f kB", float64(bytes)/1000)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
