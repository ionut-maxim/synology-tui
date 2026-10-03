package dsm

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// Package is an installed DSM package.
type Package struct {
	ID      string // "HyperBackup": what start/stop take
	Name    string
	Version string
	Status  string // running, stop, broken …
}

// Running reports whether the package is up.
func (p Package) Running() bool { return p.Status == "running" }

// Packages lists installed packages, sorted by ID (SYNO.Core.Package v2 list).
func (c *Client) Packages(ctx context.Context) ([]Package, error) {
	var out struct {
		Packages []struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			Version    string `json:"version"`
			Additional struct {
				Status string `json:"status"`
			} `json:"additional"`
		} `json:"packages"`
	}
	err := c.Call(ctx, "SYNO.Core.Package", 2, "list", url.Values{"additional": {jsonParam([]string{"status"})}}, &out)
	if err != nil {
		return nil, err
	}
	pkgs := make([]Package, 0, len(out.Packages))
	for _, p := range out.Packages {
		name := p.Name
		if name == "" {
			name = p.ID
		}
		pkgs = append(pkgs, Package{ID: p.ID, Name: name, Version: p.Version, Status: p.Additional.Status})
	}
	sort.Slice(pkgs, func(i, j int) bool { return strings.ToLower(pkgs[i].ID) < strings.ToLower(pkgs[j].ID) })
	return pkgs, nil
}

var packageIDRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func checkPackage(id string) error {
	if !packageIDRe.MatchString(id) {
		return fmt.Errorf("dsm: bad package id %q", id)
	}
	return nil
}

// StartPackage starts a package (SYNO.Core.Package.Control start). DSM may
// take minutes; pass a context with a long enough deadline.
func (c *Client) StartPackage(ctx context.Context, id string) error {
	if err := checkPackage(id); err != nil {
		return err
	}
	return c.Call(ctx, "SYNO.Core.Package.Control", 1, "start",
		url.Values{"id": {id}, "dsm_apps": {jsonParam([]string{})}}, nil)
}

// StopPackage stops a package (SYNO.Core.Package.Control stop).
func (c *Client) StopPackage(ctx context.Context, id string) error {
	if err := checkPackage(id); err != nil {
		return err
	}
	return c.Call(ctx, "SYNO.Core.Package.Control", 1, "stop", url.Values{"id": {id}}, nil)
}

// RestartPackage stops then starts a package.
func (c *Client) RestartPackage(ctx context.Context, id string) error {
	if err := c.StopPackage(ctx, id); err != nil {
		return err
	}
	return c.StartPackage(ctx, id)
}
