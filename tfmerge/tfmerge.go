package tfmerge

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/go-version"
	install "github.com/hashicorp/hc-install"
	"github.com/hashicorp/hc-install/fs"
	"github.com/hashicorp/hc-install/product"
	"github.com/hashicorp/hc-install/src"
	"github.com/hashicorp/terraform-exec/tfexec"
)

type Option struct {
	// The path to an initialized Terraform working directory.
	Wd string
}

func Merge(ctx context.Context, stateFiles []string, opt Option) ([]byte, error) {
	if len(stateFiles) == 1 {
		return os.ReadFile(stateFiles[0])
	}

	absStateFiles := []string{}
	for _, stateFile := range stateFiles {
		absPath, err := filepath.Abs(stateFile)
		if err != nil {
			return nil, err
		}
		absStateFiles = append(absStateFiles, absPath)
	}
	stateFiles = absStateFiles

	log.Println("Initialize Terraform instance")
	tf, err := initTerraform(ctx, opt.Wd)
	if err != nil {
		return nil, fmt.Errorf("initializing terraform instance: %v", err)
	}

	log.Println("List resources/modules to be merged")
	var result *multierror.Error
	resmap := map[string]string{}
	modmap := map[string]string{}
	for _, stateFile := range stateFiles {
		state, err := tf.ShowStateFile(ctx, stateFile)
		if err != nil {
			result = multierror.Append(result, fmt.Errorf("showing state file %s: %v", stateFile, err))
			continue
		}
		if state.Values == nil {
			continue
		}
		if state.Values.RootModule == nil {
			continue
		}
		for _, res := range state.Values.RootModule.Resources {
			// Ensure there is no resource address overlaps across all the state files
			if oStateFile, ok := resmap[res.Address]; ok {
				result = multierror.Append(result, fmt.Errorf(`resource %s is defined in both state files %s and %s`, res.Address, stateFile, oStateFile))
				continue
			}
			resmap[res.Address] = stateFile
		}
		for _, mod := range state.Values.RootModule.ChildModules {
			// Ensure there is no module address overlaps across all the state files
			if oStateFile, ok := modmap[mod.Address]; ok {
				result = multierror.Append(result, fmt.Errorf(`module %s is defined in both state files %s and %s`, mod.Address, stateFile, oStateFile))
				continue
			}
			modmap[mod.Address] = stateFile
		}
	}
	if err := result.ErrorOrNil(); err != nil {
		return nil, err
	}

	stateItems := map[string][]string{}
	for k, v := range resmap {
		stateItems[v] = append(stateItems[v], k)
	}
	for k, v := range modmap {
		stateItems[v] = append(stateItems[v], k)
	}

	// Remove the items that belongs to the base state file
	baseStateFile := stateFiles[0]
	delete(stateItems, baseStateFile)

	// Debug output only
	log.Println("Items to be moved:")
	for stateFile, items := range stateItems {
		log.Printf("\t%s: %v\n", stateFile, items)
	}

	// Create an empty directory to hold the state files' copies and the merged state file
	tmpdir, err := os.MkdirTemp("", "")
	if err != nil {
		return nil, fmt.Errorf("creating an empty directory as the terraform working directroy: %v", err)
	}
	defer os.RemoveAll(tmpdir)

	ofpath := filepath.Join(tmpdir, "terraform.tfstate")
	if err := copyFile(baseStateFile, ofpath); err != nil {
		return nil, fmt.Errorf("creating the base state file: %v", err)
	}

	for stateFile, items := range stateItems {
		log.Printf("Run `terraform state move` for %s\n", stateFile)
		if err := move(ctx, tf, tmpdir, stateFile, ofpath, items); err != nil {
			return nil, fmt.Errorf("terraform state move from %s: %v", stateFile, err)
		}
	}

	b, err := os.ReadFile(ofpath)
	if err != nil {
		return nil, fmt.Errorf("reading from merged state file %s: %v", ofpath, err)
	}
	return b, nil
}

func initTerraform(ctx context.Context, tfwd string) (*tfexec.Terraform, error) {
	i := install.NewInstaller()
	tfpath, err := i.Ensure(ctx, []src.Source{
		&fs.Version{
			Product: product.Terraform,
			// `terraform stat mv` is introducd since v1.1.0: https://github.com/hashicorp/terraform/releases/tag/v1.1.0
			Constraints: version.MustConstraints(version.NewConstraint(">=1.1.0")),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("finding a terraform executable: %v", err)
	}

	tf, err := tfexec.NewTerraform(tfwd, tfpath)
	if err != nil {
		return nil, fmt.Errorf("error running NewTerraform: %w", err)
	}
	if v, ok := os.LookupEnv("TF_LOG_PATH"); ok {
		tf.SetLogPath(v)
	}
	if v, ok := os.LookupEnv("TF_LOG"); ok {
		tf.SetLog(v)
	}
	return tf, nil
}

func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("reading from %s: %v", src, err)
	}

	if err := os.WriteFile(dst, b, 0644); err != nil {
		return fmt.Errorf("writing to %s: %v", dst, err)
	}

	return nil
}

func move(ctx context.Context, tf *tfexec.Terraform, tmpdir, src, dst string, items []string) error {
	// Copy the state file to another one, to avoid `terraform state mv` mutating the original state file.
	f, err := os.CreateTemp(tmpdir, "")
	if err != nil {
		return fmt.Errorf("creating a temp state file for %s: %v", src, err)
	}
	f.Close()
	srcTmp := f.Name()
	if err := copyFile(src, srcTmp); err != nil {
		return fmt.Errorf("copying the source state file: %v", err)
	}

	for _, item := range items {
		if err := tf.StateMv(ctx, item, item, tfexec.State(srcTmp), tfexec.StateOut(dst)); err != nil {
			return fmt.Errorf(`terraform state move for %s`, item)
		}
	}
	return nil
}
