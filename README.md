# tfmerge

A tool to merge multiple Terrafrom state files into one.

## Usage

Given you have an initialized Terraform working directory (abbr. *wd*) (create one and run `terraform init` in it if not existed yet). The state file in *wd* will be used as the *base state file* (i.e. the `lineage` will be reserved, and the `serial` will be incremented). Meanwhile, you have other three state files to be merged: `state1`, `state2`, `state3`, where the module and resource addresses among these state files together with the *base state file* have no overlaps.

`tfmerge` helps you merging these state files into the *base state file* by simply running `tfmerge -o terraform.tfstate state1 state2 state3` within the *wd*.

If your *wd* is using [a non-local backend](https://www.terraform.io/language/settings/backends/configuration), you'll need to manually upload the merged state file via `terraform state push`.

## How

*The process is inspired by https://support.hashicorp.com/hc/en-us/articles/4418624552339-How-to-Merge-State-Files*

`tfmerge` will simply do followings:

- Run `terraform state pull` to retrieve the *base state file*, works for both local and non-local backends. Especially, the output can be an empty string if there is no state file in the working directory, in this case a new state file will be created with a new lineage.
- Run `terraform state list` on the *base state file* and the to-be-merged state files, to list all the items to be moved. Meanwhile, ensure there is no resource/module address overlap.
- Copy all the state files to a temporary directory, to avoid mutation on existing state files.
- Repeatedly run `terraform state mv -state-out=<base statefile copy> -state=<statefile1 copy> <item address> <item address>`
- Return the merged base state file
