// Package console — SEC-18/19/20/21/39 acceptance tests for the scaffolder.
//
// These tests exercise the extraction security controls directly by calling
// the internal extraction logic with crafted ZIP archives in memory — no live
// network required. Each test corresponds to an Accept criterion in the work order.
package console

import (
	"archive/zip"
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ── helpers ───────────────────────────────────────────────────────────────────

type zipEntry struct {
	name string
	mode os.FileMode
	body string
}

// buildTestZip creates an in-memory ZIP archive from a slice of entries.
func buildTestZip(t *testing.T, entries []zipEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, e := range entries {
		fh := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		fh.SetMode(e.mode)
		fw, err := w.CreateHeader(fh)
		if err != nil {
			t.Fatalf("buildTestZip: CreateHeader(%q): %v", e.name, err)
		}
		if _, err := fw.Write([]byte(e.body)); err != nil {
			t.Fatalf("buildTestZip: Write(%q): %v", e.name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("buildTestZip: Close: %v", err)
	}
	return buf.Bytes()
}

// runExtraction invokes the production extractArchive function on in-memory zip data.
func runExtraction(t *testing.T, zipData []byte, projectDir string) error {
	t.Helper()
	r, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return err
	}
	return extractArchive(r, projectDir)
}

// ── SEC-19: Zip Slip ──────────────────────────────────────────────────────────

func TestScaffolder_SEC19_ZipSlipRejected(t *testing.T) {
	dir := t.TempDir()
	zipData := buildTestZip(t, []zipEntry{
		{name: "gostack-main/../../../evil.go", mode: 0644, body: "package evil"},
	})
	if err := runExtraction(t, zipData, filepath.Join(dir, "myproject")); err == nil {
		t.Fatal("expected Zip Slip to be rejected, got nil error")
	}
}

func TestScaffolder_SEC19_LegitimateEntryAccepted(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "myproject")
	zipData := buildTestZip(t, []zipEntry{
		{name: "gostack-main/", mode: os.ModeDir | 0755, body: ""},
		{name: "gostack-main/main.go", mode: 0644, body: "package main"},
	})
	if err := runExtraction(t, zipData, target); err != nil {
		t.Fatalf("legitimate entry rejected: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "main.go")); err != nil {
		t.Fatalf("expected main.go to be extracted: %v", err)
	}
}

// ── SEC-20: Non-regular entry named error ─────────────────────────────────────

func TestScaffolder_SEC20_SymlinkRejectedWithNamedError(t *testing.T) {
	dir := t.TempDir()
	zipData := buildTestZip(t, []zipEntry{
		{name: "gostack-main/link", mode: os.ModeSymlink | 0644, body: "/etc/passwd"},
	})
	err := runExtraction(t, zipData, filepath.Join(dir, "myproject"))
	if err == nil {
		t.Fatal("expected symlink entry to be rejected, got nil")
	}
	if !strings.Contains(err.Error(), "gostack-main/link") {
		t.Errorf("error must name the offending entry, got: %v", err)
	}
}

func TestScaffolder_SEC20_ExecutableModePreserved(t *testing.T) {
	if runtime.GOOS == "windows" {
		// NTFS does not support POSIX execute bits; os.Chmod is a no-op on
		// Windows. The extraction itself still runs — this test verifies the
		// file is created successfully even when chmod cannot be applied.
		dir := t.TempDir()
		target := filepath.Join(dir, "myproject")
		zipData := buildTestZip(t, []zipEntry{
			{name: "gostack-main/", mode: os.ModeDir | 0755, body: ""},
			{name: "gostack-main/run.sh", mode: 0755, body: "#!/bin/sh"},
		})
		if err := runExtraction(t, zipData, target); err != nil {
			t.Fatalf("extraction on Windows failed: %v", err)
		}
		if _, err := os.Stat(filepath.Join(target, "run.sh")); err != nil {
			t.Fatalf("run.sh not created on Windows: %v", err)
		}
		return
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "myproject")
	zipData := buildTestZip(t, []zipEntry{
		{name: "gostack-main/", mode: os.ModeDir | 0755, body: ""},
		{name: "gostack-main/run.sh", mode: 0755, body: "#!/bin/sh"},
	})
	if err := runExtraction(t, zipData, target); err != nil {
		t.Fatalf("extraction failed: %v", err)
	}
	info, err := os.Stat(filepath.Join(target, "run.sh"))
	if err != nil {
		t.Fatalf("run.sh not found after extraction: %v", err)
	}
	if info.Mode()&0111 == 0 {
		t.Errorf("expected executable mode on run.sh, got %v", info.Mode())
	}
}

// ── SEC-21: Decompression bomb ────────────────────────────────────────────────

func TestScaffolder_SEC21_DeclaredSizePreScanAbortsBeforeWrite(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "myproject")

	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	// Create an entry with real header
	fh := &zip.FileHeader{
		Name:   "gostack-main/huge_bomb.bin",
		Method: zip.Store,
	}
	fw, err := w.CreateHeader(fh)
	if err != nil {
		t.Fatalf("CreateHeader: %v", err)
	}
	fw.Write([]byte("tiny"))
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data := buf.Bytes()
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	// Simulate an archive whose declared uncompressed size in zip directory metadata is > 250MB
	r.File[0].UncompressedSize64 = uint64(260 << 20)

	err = extractArchive(r, target)
	if err == nil {
		t.Fatal("expected declared uncompressed size pre-scan to abort, got nil error")
	}
	if !strings.Contains(err.Error(), "declared uncompressed size exceeds limit") {
		t.Errorf("expected error to mention declared size limit, got: %v", err)
	}

	// Verify nothing was written to disk
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Errorf("expected target dir to not exist, stat: %v", statErr)
	}
}

func TestScaffolder_SEC21_PerFileLimitEnforced(t *testing.T) {
	dir := t.TempDir()
	// 26 MB body — over the 25 MB per-file cap
	bigBody := strings.Repeat("x", 26<<20)
	zipData := buildTestZip(t, []zipEntry{
		{name: "gostack-main/", mode: os.ModeDir | 0755, body: ""},
		{name: "gostack-main/big.bin", mode: 0644, body: bigBody},
	})
	if err := runExtraction(t, zipData, filepath.Join(dir, "myproject")); err == nil {
		t.Fatal("expected per-file size limit to be enforced, got nil")
	}
}

// ── SEC-39: --force overwrite protection ─────────────────────────────────────

func TestScaffolder_SEC39_RefusesOverwriteWithoutForce(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "existing.go")
	if err := os.WriteFile(target, []byte("package existing"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := writeGeneratedFile(target, []byte("package new"), false); err == nil {
		t.Fatal("expected error when overwriting without --force, got nil")
	}
	got, _ := os.ReadFile(target)
	if string(got) != "package existing" {
		t.Error("file was overwritten despite no --force flag")
	}
}

func TestScaffolder_SEC39_ForceOverwritesExistingFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "existing.go")
	if err := os.WriteFile(target, []byte("package old"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := writeGeneratedFile(target, []byte("package new"), true); err != nil {
		t.Fatalf("expected --force to allow overwrite, got: %v", err)
	}
	got, _ := os.ReadFile(target)
	if string(got) != "package new" {
		t.Errorf("file not updated after --force: got %q", got)
	}
}

// ── SEC-18: frameworkReleaseTag & verifyArchiveChecksum ───────────────────────────────

func TestScaffolder_SEC18_ReleaseTagNeverBlankOrDevel(t *testing.T) {
	tag := frameworkReleaseTag()
	if tag == "" {
		t.Fatal("frameworkReleaseTag returned empty string")
	}
	if tag == "(devel)" {
		t.Fatal("frameworkReleaseTag returned '(devel)' — fallback not applied")
	}
	if strings.Contains(tag, "main") {
		t.Fatalf("frameworkReleaseTag must never return a branch name, got: %q", tag)
	}
}

func TestScaffolder_SEC18_VerifyArchiveChecksum_TamperDetected(t *testing.T) {
	// Start a mock server serving a known checksum
	expectedHash := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  gostack-framework-v1.0.14.zip\n", expectedHash)
	}))
	defer server.Close()

	client := server.Client()

	// 1. Matching checksum passes
	err := verifyArchiveChecksumWithURL(client, server.URL, "v1.0.14", expectedHash)
	if err != nil {
		t.Fatalf("expected valid checksum to pass, got: %v", err)
	}

	// 2. Mismatched checksum fails with tamper error
	tamperedHash := "badhash000000000000000000000000000000000000000000000000000000000"
	err = verifyArchiveChecksumWithURL(client, server.URL, "v1.0.14", tamperedHash)
	if err == nil {
		t.Fatal("expected tampered checksum to be rejected, got nil")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("expected checksum mismatch error, got: %v", err)
	}

	// 3. Empty or whitespace-only checksum file fails safely without panicking
	emptyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "   \n\t  ")
	}))
	defer emptyServer.Close()

	err = verifyArchiveChecksumWithURL(emptyServer.Client(), emptyServer.URL, "v1.0.14", expectedHash)
	if err == nil {
		t.Fatal("expected empty checksum file to return error, got nil")
	}
	if !strings.Contains(err.Error(), "empty or malformed") {
		t.Errorf("expected 'empty or malformed' error, got: %v", err)
	}

	// 4. Missing/404 checksum file fails closed
	notFoundServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Not found", http.StatusNotFound)
	}))
	defer notFoundServer.Close()

	err = verifyArchiveChecksumWithURL(notFoundServer.Client(), notFoundServer.URL, "v1.0.14", expectedHash)
	if err == nil {
		t.Fatal("expected 404 checksum file to fail closed, got nil")
	}
}
