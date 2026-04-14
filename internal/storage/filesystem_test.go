package storage

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFilesystemStorage_IndexAge_NotFound(t *testing.T) {
	fs, _ := NewFilesystemStorage(t.TempDir())
	ctx := context.Background()

	age, exists, err := fs.IndexAge(ctx, "registry.terraform.io", "hashicorp", "aws")
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

func TestFilesystemStorage_IndexAge_AfterPut(t *testing.T) {
	fs, _ := NewFilesystemStorage(t.TempDir())
	ctx := context.Background()

	err := fs.PutIndex(ctx, "registry.terraform.io", "hashicorp", "aws", []byte(`{"versions":{}}`))
	if err != nil {
		t.Fatalf("PutIndex() error = %v", err)
	}

	age, exists, err := fs.IndexAge(ctx, "registry.terraform.io", "hashicorp", "aws")
	if err != nil {
		t.Fatalf("IndexAge() error = %v", err)
	}
	if !exists {
		t.Error("IndexAge() exists = false, want true after PutIndex")
	}
	// Age should be very small (just written)
	if age > 5*time.Second {
		t.Errorf("IndexAge() age = %v, expected < 5s for freshly written index", age)
	}
}

func TestFilesystemStorage_IndexAge_InvalidPath(t *testing.T) {
	fs, _ := NewFilesystemStorage(t.TempDir())
	ctx := context.Background()

	_, _, err := fs.IndexAge(ctx, "", "hashicorp", "aws")
	if err == nil {
		t.Error("IndexAge() with empty hostname expected error but got nil")
	}
}

func TestNewFilesystemStorage(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "successful creation",
			path:    filepath.Join(tmpDir, "cache"),
			wantErr: false,
		},
		{
			name:    "nested directory creation",
			path:    filepath.Join(tmpDir, "a", "b", "c"),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs, err := NewFilesystemStorage(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewFilesystemStorage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && fs == nil {
				t.Error("expected non-nil FilesystemStorage")
			}
			if !tt.wantErr {
				// Verify directory was created
				info, err := os.Stat(tt.path)
				if err != nil {
					t.Errorf("cache directory not created: %v", err)
				}
				if !info.IsDir() {
					t.Error("cache path is not a directory")
				}
			}
		})
	}
}

func TestGetIndex_NotFound(t *testing.T) {
	fs, _ := NewFilesystemStorage(t.TempDir())
	ctx := context.Background()

	_, err := fs.GetIndex(ctx, "registry.terraform.io", "hashicorp", "aws")
	if err != io.EOF {
		t.Errorf("GetIndex() error = %v, want io.EOF", err)
	}
}

func TestPutGetIndex(t *testing.T) {
	fs, _ := NewFilesystemStorage(t.TempDir())
	ctx := context.Background()

	hostname := "registry.terraform.io"
	namespace := "hashicorp"
	providerType := "aws"
	data := []byte(`{"versions": ["1.0.0"]}`)

	// Put index
	err := fs.PutIndex(ctx, hostname, namespace, providerType, data)
	if err != nil {
		t.Errorf("PutIndex() error = %v", err)
		return
	}

	// Get index
	got, err := fs.GetIndex(ctx, hostname, namespace, providerType)
	if err != nil {
		t.Errorf("GetIndex() error = %v", err)
		return
	}

	if !bytes.Equal(got, data) {
		t.Errorf("GetIndex() = %q, want %q", got, data)
	}
}

func TestGetVersion_NotFound(t *testing.T) {
	fs, _ := NewFilesystemStorage(t.TempDir())
	ctx := context.Background()

	_, err := fs.GetVersion(ctx, "registry.terraform.io", "hashicorp", "aws", "1.0.0")
	if err != io.EOF {
		t.Errorf("GetVersion() error = %v, want io.EOF", err)
	}
}

func TestPutGetVersion(t *testing.T) {
	fs, _ := NewFilesystemStorage(t.TempDir())
	ctx := context.Background()

	hostname := "registry.terraform.io"
	namespace := "hashicorp"
	providerType := "aws"
	version := "1.0.0"
	data := []byte(`{"packages": []}`)

	err := fs.PutVersion(ctx, hostname, namespace, providerType, version, data)
	if err != nil {
		t.Errorf("PutVersion() error = %v", err)
		return
	}

	got, err := fs.GetVersion(ctx, hostname, namespace, providerType, version)
	if err != nil {
		t.Errorf("GetVersion() error = %v", err)
		return
	}

	if !bytes.Equal(got, data) {
		t.Errorf("GetVersion() = %q, want %q", got, data)
	}
}

func TestPutGetVersionsResponse(t *testing.T) {
	fs, _ := NewFilesystemStorage(t.TempDir())
	ctx := context.Background()

	hostname := "registry.terraform.io"
	namespace := "hashicorp"
	providerType := "aws"
	data := []byte(`{"versions": [{"version": "1.0.0"}]}`)

	err := fs.PutVersionsResponse(ctx, hostname, namespace, providerType, data)
	if err != nil {
		t.Errorf("PutVersionsResponse() error = %v", err)
		return
	}

	got, err := fs.GetVersionsResponse(ctx, hostname, namespace, providerType)
	if err != nil {
		t.Errorf("GetVersionsResponse() error = %v", err)
		return
	}

	if !bytes.Equal(got, data) {
		t.Errorf("GetVersionsResponse() = %q, want %q", got, data)
	}
}

func TestPutGetArchive(t *testing.T) {
	fs, _ := NewFilesystemStorage(t.TempDir())
	ctx := context.Background()

	path := "registry.terraform.io/hashicorp/aws/terraform-provider-aws_5.0.0_linux_amd64.zip"
	archiveData := []byte("fake zip content")

	err := fs.PutArchive(ctx, path, bytes.NewReader(archiveData))
	if err != nil {
		t.Errorf("PutArchive() error = %v", err)
		return
	}

	rc, err := fs.GetArchive(ctx, path)
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

func TestGetArchive_NotFound(t *testing.T) {
	fs, _ := NewFilesystemStorage(t.TempDir())
	ctx := context.Background()

	_, err := fs.GetArchive(ctx, "nonexistent/file.zip")
	if err != io.EOF {
		t.Errorf("GetArchive() error = %v, want io.EOF", err)
	}
}

func TestExistsArchive(t *testing.T) {
	fs, _ := NewFilesystemStorage(t.TempDir())
	ctx := context.Background()

	path := "registry.terraform.io/hashicorp/aws/terraform-provider-aws_5.0.0_linux_amd64.zip"

	// Should not exist initially
	exists, err := fs.ExistsArchive(ctx, path)
	if err != nil {
		t.Errorf("ExistsArchive() error = %v", err)
		return
	}
	if exists {
		t.Error("ExistsArchive() returned true for non-existent archive")
	}

	// Put archive
	archiveData := []byte("content")
	err = fs.PutArchive(ctx, path, bytes.NewReader(archiveData))
	if err != nil {
		t.Errorf("PutArchive() error = %v", err)
		return
	}

	// Should exist now
	exists, err = fs.ExistsArchive(ctx, path)
	if err != nil {
		t.Errorf("ExistsArchive() error = %v", err)
		return
	}
	if !exists {
		t.Error("ExistsArchive() returned false for existing archive")
	}
}

func TestValidateProviderPath_Invalid(t *testing.T) {
	tests := []struct {
		name     string
		hostname string
		ns       string
		pType    string
	}{
		{
			name:     "empty hostname",
			hostname: "",
			ns:       "hashicorp",
			pType:    "aws",
		},
		{
			name:     "empty namespace",
			hostname: "registry.terraform.io",
			ns:       "",
			pType:    "aws",
		},
		{
			name:     "empty provider type",
			hostname: "registry.terraform.io",
			ns:       "hashicorp",
			pType:    "",
		},
		{
			name:     "slash in hostname",
			hostname: "registry/terraform.io",
			ns:       "hashicorp",
			pType:    "aws",
		},
		{
			name:     "slash in namespace",
			hostname: "registry.terraform.io",
			ns:       "hash/corp",
			pType:    "aws",
		},
		{
			name:     "backslash in provider type",
			hostname: "registry.terraform.io",
			ns:       "hashicorp",
			pType:    "aw\\s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProviderPath(tt.hostname, tt.ns, tt.pType)
			if err == nil {
				t.Error("validateProviderPath() expected error but got nil")
			}
		})
	}
}

func TestValidateProviderPath_Valid(t *testing.T) {
	tests := []struct {
		name     string
		hostname string
		ns       string
		pType    string
	}{
		{
			name:     "standard provider",
			hostname: "registry.terraform.io",
			ns:       "hashicorp",
			pType:    "aws",
		},
		{
			name:     "custom registry",
			hostname: "my-registry.example.com",
			ns:       "my-namespace",
			pType:    "my-provider",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProviderPath(tt.hostname, tt.ns, tt.pType)
			if err != nil {
				t.Errorf("validateProviderPath() error = %v", err)
			}
		})
	}
}

func TestGetIndex_InvalidInput(t *testing.T) {
	fs, _ := NewFilesystemStorage(t.TempDir())
	ctx := context.Background()

	_, err := fs.GetIndex(ctx, "", "hashicorp", "aws")
	if err == nil {
		t.Error("GetIndex() with empty hostname expected error but got nil")
	}
}

func TestPutIndex_InvalidInput(t *testing.T) {
	fs, _ := NewFilesystemStorage(t.TempDir())
	ctx := context.Background()

	err := fs.PutIndex(ctx, "registry.terraform.io", "", "aws", []byte("data"))
	if err == nil {
		t.Error("PutIndex() with empty namespace expected error but got nil")
	}
}

func TestPutArchive_EmptyPath(t *testing.T) {
	fs, _ := NewFilesystemStorage(t.TempDir())
	ctx := context.Background()

	err := fs.PutArchive(ctx, "", bytes.NewReader([]byte("data")))
	if err == nil {
		t.Error("PutArchive() with empty path expected error but got nil")
	}
}

func TestGetVersion_EmptyVersion(t *testing.T) {
	fs, _ := NewFilesystemStorage(t.TempDir())
	ctx := context.Background()

	_, err := fs.GetVersion(ctx, "registry.terraform.io", "hashicorp", "aws", "")
	if err == nil {
		t.Error("GetVersion() with empty version expected error but got nil")
	}
}

func TestPutVersion_EmptyVersion(t *testing.T) {
	fs, _ := NewFilesystemStorage(t.TempDir())
	ctx := context.Background()

	err := fs.PutVersion(ctx, "registry.terraform.io", "hashicorp", "aws", "", []byte("data"))
	if err == nil {
		t.Error("PutVersion() with empty version expected error but got nil")
	}
}

func TestContextCancellation(t *testing.T) {
	fs, _ := NewFilesystemStorage(t.TempDir())

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// PutIndex first so we have something to read
	goodCtx := context.Background()
	fs.PutIndex(goodCtx, "registry.terraform.io", "hashicorp", "aws", []byte("data"))

	// Try to read with cancelled context
	_, err := fs.GetIndex(ctx, "registry.terraform.io", "hashicorp", "aws")
	if err != context.Canceled {
		t.Errorf("GetIndex() with cancelled context error = %v, want context.Canceled", err)
	}
}

func TestDirectoryTraversal_Prevented(t *testing.T) {
	cacheDir := t.TempDir()
	fs, _ := NewFilesystemStorage(cacheDir)
	ctx := context.Background()

	// Try to put archive with path traversal
	maliciousPath := "../../../etc/passwd"
	err := fs.PutArchive(ctx, maliciousPath, bytes.NewReader([]byte("evil")))
	if err != nil {
		t.Errorf("PutArchive() with traversal path error = %v", err)
		return
	}

	// Verify file was written inside cache dir, not outside
	fullPath := filepath.Join(cacheDir, "etc", "passwd")
	data, err := os.ReadFile(fullPath)
	if err != nil {
		t.Errorf("archive file not found at expected location: %v", err)
		return
	}

	if !bytes.Equal(data, []byte("evil")) {
		t.Errorf("archive content mismatch: got %q, want %q", data, []byte("evil"))
	}

	// Verify it wasn't written outside cache dir
	if _, err := os.Stat("/etc/passwd.new"); err == nil {
		t.Error("file was written outside cache directory")
	}
}

func TestLargeArchive(t *testing.T) {
	fs, _ := NewFilesystemStorage(t.TempDir())
	ctx := context.Background()

	// Create a 10MB archive
	largeData := bytes.Repeat([]byte("x"), 10*1024*1024)
	path := "registry.terraform.io/hashicorp/aws/large.zip"

	err := fs.PutArchive(ctx, path, bytes.NewReader(largeData))
	if err != nil {
		t.Errorf("PutArchive() with large file error = %v", err)
		return
	}

	rc, err := fs.GetArchive(ctx, path)
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

	if !bytes.Equal(got, largeData) {
		t.Errorf("archive size mismatch: got %d, want %d", len(got), len(largeData))
	}
}
