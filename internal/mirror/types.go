package mirror

import (
	"errors"
	"fmt"
	"net/url"
)

var (
	// ErrNotFound is returned when a provider is not found upstream
	ErrNotFound = errors.New("provider not found")
	// ErrInvalidURL is returned when a URL is invalid
	ErrInvalidURL = errors.New("invalid URL")
	// ErrInvalidAddress is returned when a provider address is invalid
	ErrInvalidAddress = errors.New("invalid provider address")
)

// VersionInfo contains metadata about a provider version
type VersionInfo struct{}

// IndexResponse represents the response to a provider index request
// Returned by GET /:hostname/:namespace/:type/index.json
type IndexResponse struct {
	Versions map[string]VersionInfo `json:"versions"`
}

// VersionResponse represents the response to a provider version request
// Returned by GET /:hostname/:namespace/:type/:version.json
type VersionResponse struct {
	Archives map[string]Archive `json:"archives"`
}

// Archive represents a downloadable provider package
type Archive struct {
	URL    string   `json:"url"`
	Hashes []string `json:"hashes,omitempty"`
}

// ValidateURL checks if the archive URL is valid
func (a *Archive) ValidateURL() error {
	if a.URL == "" {
		return ErrInvalidURL
	}
	_, err := url.Parse(a.URL)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidURL, err)
	}
	return nil
}

// RegistryVersionsResponse is the full response from the registry /versions API
type RegistryVersionsResponse struct {
	Versions []RegistryVersion `json:"versions"`
}

// RegistryVersion represents a single version in the registry versions response
type RegistryVersion struct {
	Version   string             `json:"version"`
	Platforms []RegistryPlatform `json:"platforms"`
}

// RegistryPlatform represents a platform in the registry versions response
type RegistryPlatform struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
}

// DownloadInfo holds the download metadata from registry
type DownloadInfo struct {
	DownloadURL string `json:"download_url"`
	Shasum      string `json:"shasum"`
}

// ProviderAddress represents a provider's network address
type ProviderAddress struct {
	Hostname  string
	Namespace string
	Type      string
}

// Validate checks if the provider address is valid
func (p *ProviderAddress) Validate() error {
	if p.Hostname == "" {
		return fmt.Errorf("%w: hostname is required", ErrInvalidAddress)
	}
	if p.Namespace == "" {
		return fmt.Errorf("%w: namespace is required", ErrInvalidAddress)
	}
	if p.Type == "" {
		return fmt.Errorf("%w: type is required", ErrInvalidAddress)
	}
	return nil
}
