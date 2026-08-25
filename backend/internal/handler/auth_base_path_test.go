package handler

import "testing"

func TestPublicAppPathUsesConfiguredBasePath(t *testing.T) {
	t.Setenv("CANVAS_PUBLIC_BASE_PATH", "/kai/")
	if got := publicAppPath("/create?from=oauth"); got != "/kai/create?from=oauth" {
		t.Fatalf("publicAppPath() = %q", got)
	}
	if got := publicAppPath("/kai/projects"); got != "/kai/projects" {
		t.Fatalf("publicAppPath() duplicated base path: %q", got)
	}
	if got := sessionCookiePath(); got != "/kai/" {
		t.Fatalf("sessionCookiePath() = %q", got)
	}
}

func TestPublicAppPathDefaultsToRootAndRejectsInvalidBase(t *testing.T) {
	t.Setenv("CANVAS_PUBLIC_BASE_PATH", "//outside.example")
	if got := publicAppPath("/login"); got != "/login" {
		t.Fatalf("publicAppPath() accepted invalid base: %q", got)
	}
	if got := sessionCookiePath(); got != "/" {
		t.Fatalf("sessionCookiePath() = %q", got)
	}
}
