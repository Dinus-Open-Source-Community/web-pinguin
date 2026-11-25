package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestOperatorCatalogManifestValidates(t *testing.T) {
	data, err := os.ReadFile("../../catalog/pinguin-public-snapshot.json")
	if err != nil {
		t.Fatal(err)
	}
	var input manifest
	if err := json.Unmarshal(data, &input); err != nil {
		t.Fatal(err)
	}
	if len(input.Repositories) != 89 {
		t.Fatalf("manifest has %d repositories, want 89", len(input.Repositories))
	}
	if err := validate(input); err != nil {
		t.Fatalf("operator catalog manifest is invalid: %v", err)
	}
}

func TestValidateCatalogPathPrefixes(t *testing.T) {
	validFile := file{Path: "iso/debian/debian13/netinst.iso", SizeBytes: 10, DownloadURL: "https://mirror.example.test/iso/debian/debian13/netinst.iso"}
	base := func() manifest {
		return manifest{Repositories: []repository{{
			Slug: "debian", PathPrefix: "iso/debian", Name: "Debian", Distribution: "Debian",
			Category: "iso", Status: "active", Releases: []release{{Version: "13", Files: []file{validFile}}},
		}}}
	}

	t.Run("accepts nested mirror paths", func(t *testing.T) {
		if err := validate(base()); err != nil {
			t.Fatalf("validate() error = %v", err)
		}
	})
	t.Run("defaults path prefix to slug", func(t *testing.T) {
		input := base()
		input.Repositories[0].PathPrefix = ""
		input.Repositories[0].Releases[0].Files[0].Path = "debian/debian13/netinst.iso"
		if err := validate(input); err != nil {
			t.Fatalf("validate() error = %v", err)
		}
	})
	t.Run("rejects traversal prefix", func(t *testing.T) {
		input := base()
		input.Repositories[0].PathPrefix = "../debian"
		if err := validate(input); err == nil {
			t.Fatal("validate() accepted a traversal path prefix")
		}
	})
	t.Run("rejects file outside path prefix", func(t *testing.T) {
		input := base()
		input.Repositories[0].Releases[0].Files[0].Path = "iso/ubuntu/file.iso"
		if err := validate(input); err == nil {
			t.Fatal("validate() accepted a file outside its repository path prefix")
		}
	})
	t.Run("rejects duplicate path prefixes", func(t *testing.T) {
		input := base()
		second := input.Repositories[0]
		second.Slug = "debian-mirror"
		input.Repositories = append(input.Repositories, second)
		if err := validate(input); err == nil {
			t.Fatal("validate() accepted duplicate repository path prefixes")
		}
	})
}

func TestRepositoryUpsertDoesNotSilentlyChangePathPrefix(t *testing.T) {
	if !strings.Contains(repositoryUpsertSQL, "WHERE repositories.path_prefix = EXCLUDED.path_prefix") {
		t.Fatal("repository upsert must reject an existing slug with a changed path_prefix")
	}
}
