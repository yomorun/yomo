// Package config provides configurations for cascading zippers.
package config

// MeshZipper describes mesh configurations.
type MeshZipper struct {
	Name       string `json:"name"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Credential string `json:"credential,omitempty"`
}
