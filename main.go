package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/hashicorp/go-version"
	install "github.com/hashicorp/hc-install"
	"github.com/hashicorp/hc-install/fs"
	"github.com/hashicorp/hc-install/product"
	"github.com/hashicorp/hc-install/src"
	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/magodo/tfmerge/tfmerge"
	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:      "tfmerge",
		Usage:     `Merge Terraform state files into the state file of the current working directory`,
		UsageText: "tfmerge [option] statefile ...",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "output",
				EnvVars: []string{"TFMERGE_OUTPUT"},
				Aliases: []string{"o"},
				Usage:   "The output merged state file name",
			},
			&cli.BoolFlag{
				Name:    "debug",
				EnvVars: []string{"TFMERGE_DEBUG"},
				Aliases: []string{"d"},
				Usage:   "Show debug log",
			},
			&cli.StringFlag{
				Name:    "chdir",
				EnvVars: []string{"TFMERGE_CHDIR"},
				Usage:   "Switch to a different working directory before executing",
			},
		},
		Action: func(ctx *cli.Context) error {
			log.SetOutput(io.Discard)
			if ctx.Bool("debug") {
				log.SetPrefix("[tfmerge] ")
				log.SetOutput(os.Stderr)
			}
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			if v := ctx.String("chdir"); v != "" {
				cwd = v
			}

			tf, err := initTerraform(context.Background(), cwd)
			if err != nil {
				return err
			}

			baseState, err := tf.StatePull(ctx.Context)
			if err != nil {
				return fmt.Errorf("pulling state file of the working directory: %v", err)
			}

			b, err := tfmerge.Merge(ctx.Context, tf, []byte(baseState), ctx.Args().Slice()...)
			if err != nil {
				return err
			}

			if v := ctx.String("output"); v != "" {
				return os.WriteFile(v, b, 0644)
			}
			fmt.Println(string(b))
			return nil
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
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
