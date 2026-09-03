package version

import (
	"runtime"
	"strings"
	"testing"
)

func TestGetAlwaysReportsRuntimeIdentity(t *testing.T) {
	info := Get()

	if info.Version == "" {
		t.Fatal("Version is empty")
	}
	if info.GoVersion != runtime.Version() {
		t.Fatalf("GoVersion = %q, want %q", info.GoVersion, runtime.Version())
	}
	want := runtime.GOOS + "/" + runtime.GOARCH
	if info.Platform != want {
		t.Fatalf("Platform = %q, want %q", info.Platform, want)
	}
}

func TestStringIncludesVersionAndPlatform(t *testing.T) {
	info := Info{Version: "v1.2.3", Commit: "abc123", BuildDate: "2026-01-01T00:00:00Z", GoVersion: "go1.25", Platform: "linux/amd64"}

	got := info.String()
	for _, want := range []string{"sys-sentient", "v1.2.3", "abc123", "2026-01-01", "linux/amd64"} {
		if !strings.Contains(got, want) {
			t.Errorf("String() = %q, missing %q", got, want)
		}
	}
}

func TestStringOmitsEmptyOptionalFields(t *testing.T) {
	got := Info{Version: "dev", GoVersion: "go1.25", Platform: "linux/amd64"}.String()

	if strings.Contains(got, "()") || strings.Contains(got, "built ") {
		t.Fatalf("String() = %q, should omit empty commit and build date", got)
	}
}
