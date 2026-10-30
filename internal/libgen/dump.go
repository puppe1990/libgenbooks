package libgen

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// DumpFile is one entry of the mirror's /dbdumps/ directory listing.
type DumpFile struct {
	Name     string
	URL      string
	Size     int64
	Modified string
}

var dumpLinkRE = regexp.MustCompile(`(?is)<a href="([^"]+\.(?:rar|sql|gz|zip|7z))">[^<]*</a>\s*(\d{2}-\w{3}-\d{4} \d{2}:\d{2})\s+(\d+)`)

// Dumps lists the raw catalog dumps published by the mirror. The old
// single-file libgen_books.sql is gone; the main catalog now ships as
// libgen_new-<date>.partNNN.rar (about 1 GiB per part).
func (c *Client) Dumps(ctx context.Context) ([]DumpFile, error) {
	indexURL := c.BaseURL + "/dbdumps/"
	body, err := c.get(ctx, indexURL)
	if err != nil {
		return nil, err
	}
	dumps := ParseDumps(body, indexURL)
	if len(dumps) == 0 {
		return nil, fmt.Errorf("libgen: no dump files found at %s", indexURL)
	}
	return dumps, nil
}

// ParseDumps reads the Apache directory listing into dump files.
func ParseDumps(body []byte, indexURL string) []DumpFile {
	var dumps []DumpFile
	for _, match := range dumpLinkRE.FindAllStringSubmatch(string(body), -1) {
		size, _ := strconv.ParseInt(match[3], 10, 64)
		dumps = append(dumps, DumpFile{
			Name:     match[1],
			URL:      resolveURL(indexURL, match[1]),
			Size:     size,
			Modified: match[2],
		})
	}
	sort.Slice(dumps, func(i, j int) bool { return dumps[i].Name < dumps[j].Name })
	return dumps
}

// MainDumpParts returns the newest libgen_new-<date> part set, part 1 first.
func MainDumpParts(dumps []DumpFile) []DumpFile {
	return dumpSetWithPrefix(dumps, "libgen_new-")
}

func dumpSetWithPrefix(dumps []DumpFile, prefix string) []DumpFile {
	var matching []DumpFile
	for _, dump := range dumps {
		if strings.HasPrefix(dump.Name, prefix) && strings.Contains(dump.Name, ".part") {
			matching = append(matching, dump)
		}
	}
	if len(matching) == 0 {
		return nil
	}
	newest := newestDumpDate(matching)
	var parts []DumpFile
	for _, dump := range matching {
		if dumpDate(dump.Name) == newest {
			parts = append(parts, dump)
		}
	}
	sort.Slice(parts, func(i, j int) bool { return partNumber(parts[i].Name) < partNumber(parts[j].Name) })
	return parts
}

func newestDumpDate(dumps []DumpFile) string {
	newest := ""
	for _, dump := range dumps {
		if date := dumpDate(dump.Name); date > newest {
			newest = date
		}
	}
	return newest
}

// dumpDate pulls the YYYY-MM-DD from libgen_new-<date>.partNNN.rar.
func dumpDate(name string) string {
	start := strings.Index(name, "-")
	if start < 0 {
		return ""
	}
	rest := name[start+1:]
	if len(rest) < len("2006-01-02") {
		return ""
	}
	return rest[:len("2006-01-02")]
}

func partNumber(name string) int {
	index := strings.Index(name, ".part")
	if index < 0 {
		return 0
	}
	number, _ := strconv.Atoi(strings.TrimSuffix(name[index+len(".part"):], ".rar"))
	return number
}
