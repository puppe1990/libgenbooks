package libgen

import "testing"

const dumpListingFixture = `<html><body><pre>
<a href="../">../</a>
<a href="libgen_new-2026-10-20.part001.rar">libgen_new-2026-10-20.part001.rar</a>                  20-Oct-2026 03:16          1073741824
<a href="libgen_new-2026-10-20.part002.rar">libgen_new-2026-10-20.part002.rar</a>                  20-Oct-2026 03:20          1073741824
<a href="libgen_new-2026-10-25.part001.rar">libgen_new-2026-10-25.part001.rar</a>                  25-Oct-2026 03:16          1073741824
<a href="libgen_new-2026-10-25.part002.rar">libgen_new-2026-10-25.part002.rar</a>                  25-Oct-2026 03:20          1073741824
<a href="libgen_new-2026-10-25.part003.rar">libgen_new-2026-10-25.part003.rar</a>                  25-Oct-2026 03:24          1073741824
<a href="libgen_new_add_nodata.sql">libgen_new_add_nodata.sql</a>                            14-May-2022 05:39                6956
</pre></body></html>`

func TestParseDumps_RealListingShape(t *testing.T) {
	dumps := ParseDumps([]byte(dumpListingFixture), "https://libgen.li/dbdumps/")
	if len(dumps) != 6 {
		t.Fatalf("dumps = %d, want 6", len(dumps))
	}
	first := dumps[0]
	if first.Name != "libgen_new-2026-10-20.part001.rar" {
		t.Errorf("first name = %q (want name order)", first.Name)
	}
	if first.URL != "https://libgen.li/dbdumps/libgen_new-2026-10-20.part001.rar" {
		t.Errorf("URL = %q", first.URL)
	}
	if first.Size != 1073741824 || first.Modified != "20-Oct-2026 03:16" {
		t.Errorf("dump = %+v", first)
	}
}

func TestMainDumpParts_LatestDateOnly(t *testing.T) {
	parts := MainDumpParts(ParseDumps([]byte(dumpListingFixture), "https://libgen.li/dbdumps/"))
	if len(parts) != 3 {
		t.Fatalf("parts = %d, want the 3 newest parts", len(parts))
	}
	wantParts := []string{"part001", "part002", "part003"}
	for i, want := range wantParts {
		if partNumber(parts[i].Name) != i+1 {
			t.Errorf("parts[%d] = %q, want %s", i, parts[i].Name, want)
		}
		if dumpDate(parts[i].Name) != "2026-10-25" {
			t.Errorf("parts[%d] date = %q", i, dumpDate(parts[i].Name))
		}
	}
}

func TestMainDumpParts_EmptyListing(t *testing.T) {
	if parts := MainDumpParts(nil); len(parts) != 0 {
		t.Fatalf("parts = %+v, want none", parts)
	}
}
