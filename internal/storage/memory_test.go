package storage

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"
)

func TestMemoryStorage_IndexAge_NotFound(t *testing.T) {
	m := NewMemoryStorage()
	ctx := context.Background()

	age, exists, err := m.IndexAge(ctx, "registry.terraform.io", "hashicorp", "aws")
	if err != nil {
		t.Fatalf("IndexAge() error = %v", err)
	}
	if exists {
		t.Error("IndexAge() exists = true, want false for missing index")
	}
	if age != 0 {
		t.Errorf("IndexAge() age = %v, want 0 for missing index", age)
	}
}

func TestMemoryStorage_IndexAge_AfterPut(t *testing.T) {
	m := NewMemoryStorage()
	ctx := context.Background()

	err := m.PutIndex(ctx, "registry.terraform.io", "hashicorp", "aws", []byte(`{"versions":{}}`))
	if err != nil {
		t.Fatalf("PutIndex() error = %v", err)
	}

	age, exists, err := m.IndexAge(ctx, "registry.terraform.io", "hashicorp", "aws")
	if err != nil {
		t.Fatalf("IndexAge() error = %v", err)
	}
	if !exists {
		t.Error("IndexAge() exists = false, want true after PutIndex")
	}
	if age > 5*time.Second {
		t.Errorf("IndexAge() age = %v, expected < 5s for freshly written index", age)
	}
}

func TestMemoryStorage_IndexAge_AfterClear(t *testing.T) {
	m := NewMemoryStorage()
	ctx := context.Background()

	m.PutIndex(ctx, "registry.terraform.io", "hashicorp", "aws", []byte("data"))
	m.Clear()

	_, exists, err := m.IndexAge(ctx, "registry.terraform.io", "hashicorp", "aws")
	if err != nil {
		t.Fatalf("IndexAge() error = %v", err)
	}
	if exists {
		t.Error("IndexAge() exists = true, want false after Clear")
	}
}

func TestNewMemoryStorage(t *testing.T) {
	m := NewMemoryStorage()
	if m == nil {
		t.Error("NewMemoryStorage() returned nil")
	}
}

func TestMemoryStorage_GetIndex_NotFound(t *testing.T) {
	m := NewMemoryStorage()
	ctx := context.Background()

	_, err := m.GetIndex(ctx, "registry.terraform.io", "hashicorp", "aws")
	if err != io.EOF {
		t.Errorf("GetIndex() error = %v, want io.EOF", err)
	}
}

func TestMemoryStorage_PutGetIndex(t *testing.T) {
	m := NewMemoryStorage()
	ctx := context.Background()

	hostname := "registry.terraform.io"
	namespace := "hashicorp"
	providerType := "aws"
	data := []byte(`{"versions": ["1.0.0"]}`)

	err := m.PutIndex(ctx, hostname, namespace, providerType, data)
	if err != nil {
		t.Errorf("PutIndex() error = %v", err)
		return
	}

	got, err := m.GetIndex(ctx, hostname, namespace, providerType)
	if err != nil {
		t.Errorf("GetIndex() error = %v", err)
		return
	}

	if !bytes.Equal(got, data) {
		t.Errorf("GetIndex() = %q, want %q", got, data)
	}
}

func TestMemoryStorage_GetVersion_NotFound(t *testing.T) {
	m := NewMemoryStorage()
	ctx := context.Background()

	_, err := m.GetVersion(ctx, "registry.terraform.io", "hashicorp", "aws", "1.0.0")
	if err != io.EOF {
		t.Errorf("GetVersion() error = %v, want io.EOF", err)
	}
}

func TestMemoryStorage_PutGetVersion(t *testing.T) {
	m := NewMemoryStorage()
	ctx := context.Background()

	hostname := "registry.terraform.io"
	namespace := "hashicorp"
	providerType := "aws"
	version := "1.0.0"
	data := []byte(`{"packages": []}`)

	err := m.PutVersion(ctx, hostname, namespace, providerType, version, data)
	if err != nil {
		t.Errorf("PutVersion() error = %v", err)
		return
	}

	got, err := m.GetVersion(ctx, hostname, namespace, providerType, version)
	if err != nil {
		t.Errorf("GetVersion() error = %v", err)
		return
	}

	if !bytes.Equal(got, data) {
		t.Errorf("GetVersion() = %q, want %q", got, data)
	}
}

func TestMemoryStorage_PutGetVersionsResponse(t *testing.T) {
	m := NewMemoryStorage()
	ctx := context.Background()

	hostname := "registry.terraform.io"
	namespace := "hashicorp"
	providerType := "aws"
	data := []byte(`{"versions": [{"version": "1.0.0"}]}`)

	err := m.PutVersionsResponse(ctx, hostname, namespace, providerType, data)
	if err != nil {
		t.Errorf("PutVersionsResponse() error = %v", err)
		return
	}

	got, err := m.GetVersionsResponse(ctx, hostname, namespace, providerType)
	if err != nil {
		t.Errorf("GetVersionsResponse() error = %v", err)
		return
	}

	if !bytes.Equal(got, data) {
		t.Errorf("GetVersionsResponse() = %q, want %q", got, data)
	}
}

func TestMemoryStorage_PutGetArchive(t *testing.T) {
	m := NewMemoryStorage()
	ctx := context.Background()

	path := "registry.terraform.io/hashicorp/aws/terraform-provider-aws_5.0.0_linux_amd64.zip"
	archiveData := []byte("fake zip content")

	err := m.PutArchive(ctx, path, bytes.NewReader(archiveData))
	if err != nil {
		t.Errorf("PutArchive() error = %v", err)
		return
	}

	rc, err := m.GetArchive(ctx, path)
	if err != nil {
		t.Errorf("GetArchive() error = %v", err)
		return
	}
	defer rc.Close()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Errorf("reading archive error = %v", err)
		return
	}

	if !bytes.Equal(got, archiveData) {
		t.Errorf("archive content = %q, want %q", got, archiveData)
	}
}

func TestMemoryStorage_GetArchive_NotFound(t *testing.T) {
	m := NewMemoryStorage()
	ctx := context.Background()

	_, err := m.GetArchive(ctx, "nonexistent/file.zip")
	if err != io.EOF {
		t.Errorf("GetArchive() error = %v, want io.EOF", err)
	}
}

func TestMemoryStorage_ExistsArchive(t *testing.T) {
	m := NewMemoryStorage()
	ctx := context.Background()

	path := "registry.terraform.io/hashicorp/aws/terraform-provider-aws_5.0.0_linux_amd64.zip"

	// Should not exist initially
	exists, err := m.ExistsArchive(ctx, path)
	if err != nil {
		t.Errorf("ExistsArchive() error = %v", err)
		return
	}
	if exists {
		t.Error("ExistsArchive() returned true for non-existent archive")
	}

	// Put archive
	archiveData := []byte("content")
	err = m.PutArchive(ctx, path, bytes.NewReader(archiveData))
	if err != nil {
		t.Errorf("PutArchive() error = %v", err)
		return
	}

	// Should exist now
	exists, err = m.ExistsArchive(ctx, path)
	if err != nil {
		t.Errorf("ExistsArchive() error = %v", err)
		return
	}
	if !exists {
		t.Error("ExistsArchive() returned false for existing archive")
	}
}

func TestMemoryStorage_Clear(t *testing.T) {
	m := NewMemoryStorage()
	ctx := context.Background()

	// Put some data
	m.PutIndex(ctx, "registry.terraform.io", "hashicorp", "aws", []byte("data"))
	m.PutArchive(ctx, "path/to/archive.zip", bytes.NewReader([]byte("archive")))

	// Verify data exists
	_, err := m.GetIndex(ctx, "registry.terraform.io", "hashicorp", "aws")
	if err != nil {
		t.Errorf("before Clear: GetIndex() error = %v", err)
		return
	}

	exists, _ := m.ExistsArchive(ctx, "path/to/archive.zip")
	if !exists {
		t.Error("before Clear: archive should exist")
	}

	// Clear
	m.Clear()

	// Verify data is gone
	_, err = m.GetIndex(ctx, "registry.terraform.io", "hashicorp", "aws")
	if err != io.EOF {
		t.Errorf("after Clear: GetIndex() error = %v, want io.EOF", err)
	}

	exists, _ = m.ExistsArchive(ctx, "path/to/archive.zip")
	if exists {
		t.Error("after Clear: archive should not exist")
	}
}

func TestMemoryStorage_DataIsolation(t *testing.T) {
	m := NewMemoryStorage()
	ctx := context.Background()

	// Put original data
	originalData := []byte("original")
	m.PutIndex(ctx, "registry.terraform.io", "hashicorp", "aws", originalData)

	// Get data and modify it
	retrieved, _ := m.GetIndex(ctx, "registry.terraform.io", "hashicorp", "aws")
	retrieved[0] = 'X' // Try to modify

	// Get data again - should still be original due to cloning
	retrieved2, _ := m.GetIndex(ctx, "registry.terraform.io", "hashicorp", "aws")
	if !bytes.Equal(retrieved2, originalData) {
		t.Errorf("data isolation failed: got %q, want %q", retrieved2, originalData)
	}
}

func TestMemoryStorage_ArchiveDataIsolation(t *testing.T) {
	m := NewMemoryStorage()
	ctx := context.Background()

	path := "path/to/archive.zip"
	originalData := []byte("original archive")

	// Put archive
	m.PutArchive(ctx, path, bytes.NewReader(originalData))

	// Get archive and read it
	rc1, _ := m.GetArchive(ctx, path)
	data1, _ := io.ReadAll(rc1)
	rc1.Close()

	// Get archive again and verify data is the same
	rc2, _ := m.GetArchive(ctx, path)
	data2, _ := io.ReadAll(rc2)
	rc2.Close()

	if !bytes.Equal(data1, data2) {
		t.Errorf("archive data mismatch: got %q, want %q", data2, data1)
	}
}

func TestMemoryStorage_MultipleProviders(t *testing.T) {
	m := NewMemoryStorage()
	ctx := context.Background()

	// Put data for multiple providers
	data1 := []byte("aws data")
	data2 := []byte("azure data")

	m.PutIndex(ctx, "registry.terraform.io", "hashicorp", "aws", data1)
	m.PutIndex(ctx, "registry.terraform.io", "hashicorp", "azurerm", data2)

	// Retrieve and verify
	got1, _ := m.GetIndex(ctx, "registry.terraform.io", "hashicorp", "aws")
	got2, _ := m.GetIndex(ctx, "registry.terraform.io", "hashicorp", "azurerm")

	if !bytes.Equal(got1, data1) {
		t.Errorf("aws data mismatch: got %q, want %q", got1, data1)
	}
	if !bytes.Equal(got2, data2) {
		t.Errorf("azurerm data mismatch: got %q, want %q", got2, data2)
	}
}
