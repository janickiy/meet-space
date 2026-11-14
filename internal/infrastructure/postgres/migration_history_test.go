package postgres

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func TestHistoricalSchemaRecognizesOnlyReviewedDumpRestoreVariant(t *testing.T) {
	var fixture struct {
		Definitions []string `json:"definitions"`
	}
	raw, err := os.ReadFile(filepath.Join("testdata", "migration_schema_v49.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	checksum := func(definitions []string) string {
		t.Helper()
		ordered := append([]string(nil), definitions...)
		sort.Strings(ordered)
		return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(ordered, "\n")+"\n")))
	}
	if len(fixture.Definitions) != 1099 || checksum(fixture.Definitions) != historicalSchemaChecksum || !allowedHistoricalSchemaChecksum(checksum(fixture.Definitions)) {
		t.Fatal("frozen historical fixture identity changed")
	}
	// Derive the known restore rendering from the independently frozen fixture;
	// this rewrite is test-only and never weakens production catalog validation.
	array := regexp.MustCompile(`\(ARRAY\[((?:'(?:[^']|'')*'::character varying)(?:, '(?:[^']|'')*'::character varying)*)\]\)::text\[\]`)
	literal := regexp.MustCompile(`'(?:[^']|'')*'::character varying`)
	restored := append([]string(nil), fixture.Definitions...)
	casts, constraints, indexes := 0, 0, 0
	for i, definition := range restored {
		if !array.MatchString(definition) {
			continue
		}
		switch {
		case strings.HasPrefix(definition, "constraint:"):
			constraints++
		case strings.HasPrefix(definition, "index:"):
			indexes++
		default:
			t.Fatal("restore rewrite unexpectedly affects another catalog category")
		}
		restored[i] = array.ReplaceAllStringFunc(definition, func(expression string) string {
			casts++
			values := literal.FindAllString(array.FindStringSubmatch(expression)[1], -1)
			for j, value := range values {
				values[j] = "(" + value + ")::text"
			}
			return "ARRAY[" + strings.Join(values, ", ") + "]"
		})
	}
	if casts != 37 || constraints != 27 || indexes != 9 {
		t.Fatalf("restore rewrite scope changed: casts=%d constraints=%d indexes=%d", casts, constraints, indexes)
	}
	if got := checksum(restored); got != historicalRestoredSchemaChecksum || !allowedHistoricalSchemaChecksum(got) {
		t.Fatalf("reviewed restored catalog identity mismatch: %s", got)
	}
	for _, variant := range []struct{ name, prefix, from, to string }{
		{"changed-allowed-value", "constraint:background_jobs:", "'queued'", "'unexpected'"},
		{"unvalidated-constraint", "constraint:background_jobs:", ":true", ":false"},
		{"invalid-index", "index:", ":true:true", ":false:true"},
		{"disabled-integrity-trigger", "trigger:CREATE CONSTRAINT TRIGGER direct_pair_", ":O", ":D"},
	} {
		t.Run(variant.name, func(t *testing.T) {
			changed := append([]string(nil), restored...)
			found := false
			for i, definition := range changed {
				if strings.HasPrefix(definition, variant.prefix) && strings.Contains(definition, variant.from) {
					changed[i] = strings.Replace(definition, variant.from, variant.to, 1)
					found = true
					break
				}
			}
			if !found {
				t.Fatal("tamper fixture did not change a definition")
			}
			if allowedHistoricalSchemaChecksum(checksum(changed)) {
				t.Fatal("modified schema was accepted as the reviewed restored catalog")
			}
		})
	}
}
