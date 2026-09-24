package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const unpinnableWarning = "task audit\ncannot pin it"

// TestSyncWarnsWhenRecordingAnUnpinnableVersion pins spec 0022's 3a: a
// dev or +dirty vibe records a version task audit's fallback cannot fetch,
// so CI's conformance check will fail unless vibe is on PATH there. Sync
// says so when it writes that version, not a push later, and still
// succeeds: the files it wrote are correct.
func TestSyncWarnsWhenRecordingAnUnpinnableVersion(t *testing.T) {
	for _, version := range []string{"dev", "v0.3.0-alpha.1.0.20260924000000-abcdefabcdef+dirty"} {
		t.Run(version, func(t *testing.T) {
			dir := t.TempDir()
			writeManifest(t, dir)
			stubHookInstall(t)

			out, err := runAs(t, version, "sync", "--repo-root", dir)
			if err != nil {
				t.Fatalf("an unpinnable version must not fail a sync: %v\n%s", err, out)
			}
			if !strings.Contains(out, "warning: vibe "+version+" is not a released or pseudo-version") {
				t.Errorf("sync did not warn about recording %s\n%s", version, out)
			}
		})
	}
}

// TestSyncDoesNotWarnForPinnableVersion is the other half: a release or
// clean pseudo-version is exactly what task audit can fetch.
func TestSyncDoesNotWarnForPinnableVersion(t *testing.T) {
	for _, version := range []string{"v0.3.0", "v0.3.0-alpha.1.0.20260924000000-abcdefabcdef"} {
		t.Run(version, func(t *testing.T) {
			dir := t.TempDir()
			writeManifest(t, dir)
			stubHookInstall(t)

			out, err := runAs(t, version, "sync", "--repo-root", dir)
			if err != nil {
				t.Fatalf("sync: %v\n%s", err, out)
			}
			if strings.Contains(out, "is not a released or pseudo-version") {
				t.Errorf("sync warned about a pinnable version\n%s", out)
			}
		})
	}
}

// TestSyncDoesNotWarnWhenSelfHosting covers the repository that provides
// cmd/vibe — VibeConform itself. Its conformance job builds vibe from
// source and never pins (ADR 0008), and its maintainer syncs with a +dirty
// build every time a template changes; a warning there would only teach
// people to ignore it.
func TestSyncDoesNotWarnWhenSelfHosting(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir)
	stubHookInstall(t)
	if err := os.MkdirAll(filepath.Join(dir, "cmd", "vibe"), 0o755); err != nil {
		t.Fatal(err)
	}

	out, err := runAs(t, "dev", "sync", "--repo-root", dir)
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if strings.Contains(out, "is not a released or pseudo-version") {
		t.Errorf("sync warned in a self-hosting repository\n%s", out)
	}
}

// TestUnpinnableWarningGoesToStderr keeps sync's stdout a report of what it
// did, as the missing-tool warnings already do.
func TestUnpinnableWarningGoesToStderr(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir)
	stubHookInstall(t)

	out, errOut, err := runSyncCapturing(t, dir) // version "test": unpinnable
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if strings.Contains(out, "is not a released or pseudo-version") {
		t.Errorf("warning on stdout\n%s", out)
	}
	for want := range strings.SplitSeq(unpinnableWarning, "\n") {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr missing %q\n%s", want, errOut)
		}
	}
}
