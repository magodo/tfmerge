package tfmerge

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func initTest(ctx context.Context, t *testing.T) string {
	// Discard log output
	log.SetOutput(io.Discard)

	// Init terraform with null provider
	dir := t.TempDir()
	tf, err := initTerraform(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "terraform.tf"), []byte(`terraform {
  required_providers {
    null = {
      source = "hashicorp/null"
    }
  }
}
`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := tf.Init(ctx); err != nil {
		t.Fatal(err)
	}

	return dir
}

func testFixture(t *testing.T, name string) (stateFiles []string, expectState []byte) {
	dir := filepath.Join("./testdata", name)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading dir entries: %v", err)
	}
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if entry.Name() == "expect" {
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("reading file %s: %v", path, err)
			}
			expectState = b
			continue
		}
		stateFiles = append(stateFiles, path)
	}
	return
}

func assertStateEqual(t *testing.T, actual, expect []byte) {
	var actualState, expectState map[string]interface{}
	if err := json.Unmarshal(actual, &actualState); err != nil {
		t.Fatalf("unmarshal actual state\n%s\n: %v", string(actual), err)
	}
	if err := json.Unmarshal(expect, &expectState); err != nil {
		t.Fatalf("unmarshal expect state\n%s\n: %v", string(expect), err)
	}

	// The terraform version used to create the testdata might be different than the one running this test.
	delete(actualState, "terraform_version")
	delete(expectState, "terraform_version")

	actualJson, err := json.Marshal(actualState)
	if err != nil {
		t.Fatalf("marshal modified actual state: %v", err)
	}
	expectJson, err := json.Marshal(expectState)
	if err != nil {
		t.Fatalf("marshal modified expect state: %v", err)
	}
	require.JSONEq(t, string(expectJson), string(actualJson))
}

func TestMerge_resourceOnly(t *testing.T) {
	ctx := context.Background()
	wd := initTest(ctx, t)

	stateFiles, expect := testFixture(t, "resource_only")
	actual, err := Merge(context.Background(), stateFiles, Option{Wd: wd})
	if err != nil {
		t.Fatal(err)
	}
	assertStateEqual(t, actual, expect)
}

func TestMerge_modules(t *testing.T) {
	ctx := context.Background()
	wd := initTest(ctx, t)

	stateFiles, expect := testFixture(t, "module")
	actual, err := Merge(context.Background(), stateFiles, Option{Wd: wd})
	if err != nil {
		t.Fatal(err)
	}
	assertStateEqual(t, actual, expect)
}
