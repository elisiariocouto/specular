package mirror

import (
	"errors"
	"testing"
)

func TestArchiveValidateURL(t *testing.T) {
	tests := []struct {
		name    string
		archive Archive
		wantErr bool
		errType error
	}{
		{
			name: "valid HTTPS URL",
			archive: Archive{
				URL: "https://releases.hashicorp.com/terraform-provider-aws/6.26.0/terraform-provider-aws_6.26.0_darwin_arm64.zip",
			},
			wantErr: false,
		},
		{
			name: "valid HTTP URL",
			archive: Archive{
				URL: "http://example.com/provider.zip",
			},
			wantErr: false,
		},
		{
			name: "valid file URL",
			archive: Archive{
				URL: "file:///tmp/provider.zip",
			},
			wantErr: false,
		},
		{
			name: "empty URL",
			archive: Archive{
				URL: "",
			},
			wantErr: true,
			errType: ErrInvalidURL,
		},
		{
			name: "invalid URL format",
			archive: Archive{
				URL: "ht!tp://[invalid",
			},
			wantErr: true,
			errType: ErrInvalidURL,
		},
		{
			name: "URL with hashes",
			archive: Archive{
				URL:    "https://example.com/provider.zip",
				Hashes: []string{"h1:abc123", "h1:def456"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.archive.ValidateURL()
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateURL() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.errType != nil {
				if !errors.Is(err, tt.errType) {
					t.Errorf("ValidateURL() error = %v, want error type %v", err, tt.errType)
				}
			}
		})
	}
}

func TestProviderAddressValidate(t *testing.T) {
	tests := []struct {
		name      string
		address   ProviderAddress
		wantErr   bool
		errType   error
		errSubstr string
	}{
		{
			name: "valid address",
			address: ProviderAddress{
				Hostname:  "registry.terraform.io",
				Namespace: "hashicorp",
				Type:      "aws",
			},
			wantErr: false,
		},
		{
			name: "valid custom registry",
			address: ProviderAddress{
				Hostname:  "private.registry.example.com",
				Namespace: "mycompany",
				Type:      "custom-provider",
			},
			wantErr: false,
		},
		{
			name: "missing hostname",
			address: ProviderAddress{
				Hostname:  "",
				Namespace: "hashicorp",
				Type:      "aws",
			},
			wantErr:   true,
			errType:   ErrInvalidAddress,
			errSubstr: "hostname is required",
		},
		{
			name: "missing namespace",
			address: ProviderAddress{
				Hostname:  "registry.terraform.io",
				Namespace: "",
				Type:      "aws",
			},
			wantErr:   true,
			errType:   ErrInvalidAddress,
			errSubstr: "namespace is required",
		},
		{
			name: "missing type",
			address: ProviderAddress{
				Hostname:  "registry.terraform.io",
				Namespace: "hashicorp",
				Type:      "",
			},
			wantErr:   true,
			errType:   ErrInvalidAddress,
			errSubstr: "type is required",
		},
		{
			name: "all fields empty",
			address: ProviderAddress{
				Hostname:  "",
				Namespace: "",
				Type:      "",
			},
			wantErr:   true,
			errType:   ErrInvalidAddress,
			errSubstr: "hostname is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.address.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.errType != nil {
				if !errors.Is(err, tt.errType) {
					t.Errorf("Validate() error = %v, want error type %v", err, tt.errType)
				}
			}
			if tt.wantErr && tt.errSubstr != "" {
				if err == nil || !errors.Is(err, tt.errType) {
					t.Errorf("Validate() error message should contain %q, got %v", tt.errSubstr, err)
				}
			}
		})
	}
}

func TestIndexResponseStructure(t *testing.T) {
	t.Run("create index response with versions", func(t *testing.T) {
		indexResp := IndexResponse{
			Versions: map[string]VersionInfo{
				"1.0.0": {},
				"2.0.0": {},
				"3.0.0": {},
			},
		}

		if len(indexResp.Versions) != 3 {
			t.Errorf("expected 3 versions, got %d", len(indexResp.Versions))
		}

		for _, version := range []string{"1.0.0", "2.0.0", "3.0.0"} {
			if _, ok := indexResp.Versions[version]; !ok {
				t.Errorf("version %s not found in index response", version)
			}
		}
	})

	t.Run("empty index response", func(t *testing.T) {
		indexResp := IndexResponse{
			Versions: make(map[string]VersionInfo),
		}

		if len(indexResp.Versions) != 0 {
			t.Errorf("expected 0 versions, got %d", len(indexResp.Versions))
		}
	})
}

func TestVersionResponseStructure(t *testing.T) {
	t.Run("create version response with archives", func(t *testing.T) {
		versionResp := VersionResponse{
			Archives: map[string]Archive{
				"terraform-provider-aws_6.26.0_darwin_arm64": {
					URL:    "https://releases.hashicorp.com/terraform-provider-aws/6.26.0/terraform-provider-aws_6.26.0_darwin_arm64.zip",
					Hashes: []string{"h1:abc123"},
				},
				"terraform-provider-aws_6.26.0_linux_amd64": {
					URL:    "https://releases.hashicorp.com/terraform-provider-aws/6.26.0/terraform-provider-aws_6.26.0_linux_amd64.zip",
					Hashes: []string{"h1:def456"},
				},
			},
		}

		if len(versionResp.Archives) != 2 {
			t.Errorf("expected 2 archives, got %d", len(versionResp.Archives))
		}

		for _, archive := range versionResp.Archives {
			if err := archive.ValidateURL(); err != nil {
				t.Errorf("archive URL validation failed: %v", err)
			}
		}
	})
}

func TestDownloadInfo(t *testing.T) {
	t.Run("create download info", func(t *testing.T) {
		downloadInfo := DownloadInfo{
			DownloadURL: "https://example.com/provider.zip",
			Shasum:      "abc123def456",
		}

		if downloadInfo.DownloadURL == "" {
			t.Error("download URL should not be empty")
		}
		if downloadInfo.Shasum == "" {
			t.Error("shasum should not be empty")
		}
	})
}
