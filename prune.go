package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/containerd/containerd/namespaces"
	"github.com/docker/go-units"
	"github.com/genuinetools/img/client"
	controlapi "github.com/moby/buildkit/api/services/control"
	"github.com/moby/buildkit/identity"
	"github.com/moby/buildkit/session"
	"github.com/spf13/cobra"
)

const pruneUsageShortHelp = `Prune and clean up the build cache.`
const pruneUsageLongHelp = `Prune and clean up the build cache.`

func newPruneCommand() *cobra.Command {
	prune := &pruneCommand{
		filters: newListValue(),
	}

	cmd := &cobra.Command{
		Use:                   "prune [OPTIONS]",
		DisableFlagsInUseLine: true,
		SilenceUsage:          true,
		Short:                 pruneUsageShortHelp,
		Long:                  pruneUsageLongHelp,
		Args:                  validateHasNoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return prune.Run(args)
		},
	}

	fs := cmd.Flags()

	fs.DurationVar(&prune.keepDuration, "keep-duration", 0, "Keep data newer than this limit")
	fs.Float64Var(&prune.keepStorageMB, "keep-storage", 0, "Keep data below this limit (in MB)")
	fs.VarP(prune.filters, "filter", "f", "Filter based on conditions provided")
	fs.BoolVar(&prune.all, "all", false, "Include internal/frontend references")

	return cmd
}

type pruneCommand struct {
	keepDuration  time.Duration
	keepStorageMB float64
	filters       *listValue
	all           bool
}

func (cmd *pruneCommand) Run(args []string) (err error) {
	reexec()

	// Create the context.
	id := identity.NewID()
	ctx := session.NewContext(context.Background(), id)
	ctx = namespaces.WithNamespace(ctx, "buildkit")

	// Create the client.
	c, err := client.New(stateDir, backend, nil)
	if err != nil {
		return err
	}
	defer c.Close()

	usage, err := c.Prune(ctx, &controlapi.PruneRequest{
		Filter:       cmd.filters.GetAll(),
		All:          cmd.all,
		KeepDuration: int64(cmd.keepDuration),
		KeepBytes:    int64(cmd.keepStorageMB * 1e6),
	})
	if err != nil {
		return err
	}

	tw := tabwriter.NewWriter(os.Stdout, 1, 8, 1, '\t', 0)

	if debug {
		printDebug(tw, usage)
	} else {
		fmt.Fprintln(tw, "ID\tRECLAIMABLE\tSIZE\tDESCRIPTION")

		for _, di := range usage {
			id := di.ID
			if di.Mutable {
				id += "*"
			}
			desc := di.Description
			if len(desc) > 50 {
				desc = desc[0:50] + "..."
			}
			fmt.Fprintf(tw, "%s\t%t\t%s\t%s\n", id, !di.InUse, units.BytesSize(float64(di.Size_)), desc)
		}

		tw.Flush()
	}

	total := int64(0)
	reclaimable := int64(0)

	for _, di := range usage {
		if di.Size_ > 0 {
			total += di.Size_
			if !di.InUse {
				reclaimable += di.Size_
			}
		}
	}

	tw = tabwriter.NewWriter(os.Stdout, 1, 8, 1, '\t', 0)
	fmt.Fprintf(tw, "Reclaimed:\t%s\n", units.BytesSize(float64(reclaimable)))
	fmt.Fprintf(tw, "Total:\t%s\n", units.BytesSize(float64(total)))
	tw.Flush()

	return nil
}
