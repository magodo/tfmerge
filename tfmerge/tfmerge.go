package tfmerge

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/terraform-exec/tfexec"
	tfjson "github.com/hashicorp/terraform-json"
)

// Merge merges the state files to the base state.
//
// In case there is resource address conflict, if the conflict resources have the same "id", they are regarded as the same, and only one of them is kept.
// Otherwise, it will error.
//
// baseState can be nil to indicate no base state file.
func Merge(ctx context.Context, tf *tfexec.Terraform, baseState []byte, stateFiles ...string) ([]byte, error) {
	if baseState == nil {
		baseState = []byte{}
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

	var result *multierror.Error

	type resourceInfo struct {
		stateFile string
		id        string
	}

	resmap := map[string]resourceInfo{}

	// If there is no state file in the current working directory, "terraform state pull" returns an empty string.
	// In this case, we don't append it into the state file list for listing move items.
	stl := stateFiles[:]
	if len(baseState) != 0 {
		stl = append(stl, baseStateFile)
	}

	var checkConflict func(stateFile string, module *tfjson.StateModule)
	checkConflict = func(stateFile string, module *tfjson.StateModule) {
		if module == nil {
			return
		}
		for _, res := range module.Resources {
			// Ensure there is no resource address overlaps across all the state files
			if oResInfo, ok := resmap[res.Address]; ok {
				// Further check if the resource id are the same, in which case we regard they are the same resource and skip it
				if oResInfo.id != "" && oResInfo.id == getResourceId(res) {
					continue
				}
				result = multierror.Append(result, fmt.Errorf(`resource %s is defined in both state files %s and %s`, res.Address, stateFile, oResInfo.stateFile))
				continue
			}

			resmap[res.Address] = resourceInfo{
				stateFile: stateFile,
				id:        getResourceId(res),
			}
		}
		for _, mod := range module.ChildModules {
			checkConflict(stateFile, mod)
		}
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
		checkConflict(stateFile, state.Values.RootModule)

	}
	if err := result.ErrorOrNil(); err != nil {
		return nil, err
	}

	// key: state file name; value: resource address
	stateItems := map[string][]string{}
	for k, v := range resmap {
		stateItems[v.stateFile] = append(stateItems[v.stateFile], k)
	}

	// Remove the items that belongs to the base state file
	delete(stateItems, baseStateFile)

	for stateFile, items := range stateItems {
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

func getResourceId(res *tfjson.StateResource) string {
	var id string
	if idRaw, ok := res.AttributeValues["id"]; ok {
		if idStr, ok := idRaw.(string); ok {
			id = idStr
		}
	}
	return id
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
