package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"

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

			opt := tfmerge.Option{
				Wd: cwd,
			}

			if v := ctx.String("chdir"); v != "" {
				opt.Wd = v
			}

			b, err := tfmerge.Merge(context.Background(), ctx.Args().Slice(), opt)
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
