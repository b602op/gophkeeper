package buildinfo

import (
	"strings"
	"testing"
)

func TestString(t *testing.T) {
	got := String()
	for _, want := range []string{"version=", "build_date=", "commit=", Version, BuildDate, Commit} {
		if !strings.Contains(got, want) {
			t.Errorf("String() = %q, не содержит %q", got, want)
		}
	}
}
