# tfmerge

A tool to merge multiple Terrafrom state files into one. The process is inspired by https://support.hashicorp.com/hc/en-us/articles/4418624552339-How-to-Merge-State-Files.

## Usage

Given you have three state files separated in different Terraform working directories: `state1`, `state2`, `state3`, wherer the module and resource addresses among these state files have no overlaps. `tfmerge` helps you merging these state files into one by following steps below:

1. Create a new directory as a Terraform working directory
2. Run `terrafrom init` with in above directory, with the required `terraform` settings (e.g. provider settings)
3. Run `tfmerge -o terraform.tfstate state1 state2 state3` in this directory, which merges the specified state files into one `terraform.tfstate`
