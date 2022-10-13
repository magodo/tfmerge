package tfmerge

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/terraform-exec/tfexec"
)

// Merge merges the state files to the base state. If there is any address conflicts for either resource or module, it will error.
// baseState can be empty.
func Merge(ctx context.Context, tf *tfexec.Terraform, baseState []byte, stateFiles ...string) ([]byte, error) {
	absStateFiles := []string{}
	for _, stateFile := range stateFiles {
		absPath, err := filepath.Abs(stateFile)
		if err != nil {
			return nil, err
		}
		absStateFiles = append(absStateFiles, absPath)
	}
	stateFiles = absStateFiles

	// Create an empty directory to hold the state files' copies and the merged state file
	tmpdir, err := os.MkdirTemp("", "")
	if err != nil {
		return nil, fmt.Errorf("creating an empty directory as the terraform working directroy: %v", err)
	}
	defer os.RemoveAll(tmpdir)

	baseStateFile := filepath.Join(tmpdir, "terraform.tfstate")
	if err := os.WriteFile(baseStateFile, baseState, 0644); err != nil {
		return nil, fmt.Errorf("creating the base state file: %v", err)
	}

	log.Println("List resources/modules to be merged")
	var result *multierror.Error
	resmap := map[string]string{}
	modmap := map[string]string{}

	// If there is no state file in the current working directory, "terraform state pull" returns an empty string.
	// In this case, we don't append it into the state file list for listing move items.
	stl := stateFiles[:]
	if len(baseState) != 0 {
		stl = append(stl, baseStateFile)
	}
	for _, stateFile := range stl {
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
	delete(stateItems, baseStateFile)

	// Debug output only
	log.Println("Items to be moved:")
	for stateFile, items := range stateItems {
		log.Printf("\t%s: %v\n", stateFile, items)
	}

	for stateFile, items := range stateItems {
		log.Printf("Run `terraform state move` for %s\n", stateFile)
		if err := move(ctx, tf, tmpdir, stateFile, baseStateFile, items); err != nil {
			return nil, fmt.Errorf("terraform state move from %s: %v", stateFile, err)
		}
	}

	b, err := os.ReadFile(baseStateFile)
	if err != nil {
		return nil, fmt.Errorf("reading from merged state file %s: %v", baseStateFile, err)
	}
	return b, nil
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
