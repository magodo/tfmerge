package tfmerge

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/go-version"
	install "github.com/hashicorp/hc-install"
	"github.com/hashicorp/hc-install/fs"
	"github.com/hashicorp/hc-install/product"
	"github.com/hashicorp/hc-install/src"
	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/stretchr/testify/require"
)

func initTest(ctx context.Context, t *testing.T, genBaseState bool) *tfexec.Terraform {
	// Discard log output
	log.SetOutput(io.Discard)

	// Init terraform with null provider
	dir := t.TempDir()
	i := install.NewInstaller()
	tfpath, err := i.Ensure(ctx, []src.Source{
		&fs.Version{
			Product:     product.Terraform,
			Constraints: version.MustConstraints(version.NewConstraint(">=1.1.0")),
		},
	})
	if err != nil {
		t.Fatalf("finding a terraform executable: %v", err)
	}
	tf, err := tfexec.NewTerraform(dir, tfpath)
	if err != nil {
		t.Fatalf("error running NewTerraform: %v", err)
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
	if genBaseState {
		if err := os.WriteFile(filepath.Join(dir, "terraform.tfstate"), []byte(`{
  "version": 4,
  "terraform_version": "1.2.8",
  "serial": 1,
  "lineage": "00000000-0000-0000-0000-000000000000",
  "outputs": {},
  "resources": []
}
`), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := tf.Init(ctx); err != nil {
		t.Fatal(err)
	}

	return tf
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

func assertStateEqual(t *testing.T, actual, expect []byte, mergedCount int, hasBaseState bool) {
	var actualState, expectState map[string]interface{}
	if err := json.Unmarshal(actual, &actualState); err != nil {
		t.Fatalf("unmarshal actual state\n%s\n: %v", string(actual), err)
	}
	if err := json.Unmarshal(expect, &expectState); err != nil {
		t.Fatalf("unmarshal expect state\n%s\n: %v", string(expect), err)
	}

	if !hasBaseState {
		delete(actualState, "lineage")
		delete(expectState, "lineage")
	}
	if hasBaseState {
		mergedCount += 1
	}
	expectState["serial"] = mergedCount

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

func TestMerge(t *testing.T) {
	cases := []struct {
		name         string
		dir          string
		hasBaseState bool
	}{
		{
			name:         "Resource Only (no base state)",
			dir:          "resource_only",
			hasBaseState: false,
		},
		{
			name:         "Resource Only (base state)",
			dir:          "resource_only",
			hasBaseState: true,
		},
		{
			name:         "Module (no base state)",
			dir:          "module",
			hasBaseState: false,
		},
		{
			name:         "Module (base state)",
			dir:          "module",
			hasBaseState: true,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			tf := initTest(ctx, t, tt.hasBaseState)
			stateFiles, expect := testFixture(t, tt.dir)
			actual, err := Merge(context.Background(), tf, stateFiles)
			if err != nil {
				t.Fatal(err)
			}
			assertStateEqual(t, actual, expect, len(stateFiles), tt.hasBaseState)
		})
	}
}
