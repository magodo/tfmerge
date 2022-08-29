package tfmerge

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"testing"
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

func stateEqual(src, dst []byte) (bool, error) {
	var srcState, dstState map[string]interface{}
	if err := json.Unmarshal(src, &srcState); err != nil {
		return false, fmt.Errorf("unmarshal source state\n%s\n: %v", string(src), err)
	}
	if err := json.Unmarshal(dst, &dstState); err != nil {
		return false, fmt.Errorf("unmarshal dest state\n%s\n: %v", string(dst), err)
	}

	delete(srcState, "lineage")
	delete(dstState, "lineage")

	// The terraform version used to create the testdata might be different than the one running this test.
	delete(srcState, "terraform_version")
	delete(dstState, "terraform_version")

	return reflect.DeepEqual(srcState, dstState), nil
}

func TestMerge_resourceOnly(t *testing.T) {
	ctx := context.Background()
	wd := initTest(ctx, t)

	stateFiles, expect := testFixture(t, "resource_only")
	actual, err := Merge(context.Background(), stateFiles, Option{Wd: wd})
	if err != nil {
		t.Fatal(err)
	}
	ok, err := stateEqual(actual, expect)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("state not equals to the expected.\n\nActual:\n%s\n\nExpected:\n%s\n", actual, expect)
	}
}

func TestMerge_modules(t *testing.T) {
	ctx := context.Background()
	wd := initTest(ctx, t)

	stateFiles, expect := testFixture(t, "module")
	actual, err := Merge(context.Background(), stateFiles, Option{Wd: wd})
	if err != nil {
		t.Fatal(err)
	}
	ok, err := stateEqual(actual, expect)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("state not equals to the expected.\n\nActual:\n%s\n\nExpected:\n%s\n", actual, expect)
	}
}
