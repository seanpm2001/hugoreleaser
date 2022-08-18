package allcmd

import (
	"context"
	"flag"

	"github.com/bep/hugoreleaser/cmd/archivecmd"
	"github.com/bep/hugoreleaser/cmd/buildcmd"
	"github.com/bep/hugoreleaser/cmd/corecmd"
	"github.com/bep/hugoreleaser/cmd/releasecmd"

	"github.com/bep/logg"
	"github.com/peterbourgon/ff/v3/ffcli"
)

const commandName = "all"

// New returns a usable ffcli.Command for the archive subcommand.
func New(core *corecmd.Core) *ffcli.Command {

	fs := flag.NewFlagSet(corecmd.CommandName+" "+commandName, flag.ExitOnError)

	a := &all{
		core:      core,
		builder:   buildcmd.NewBuilder(core, fs),
		archivist: archivecmd.NewArchivist(core, fs),
		releaser:  releasecmd.NewReleaser(core, fs),
	}

	core.RegisterFlags(fs)

	return &ffcli.Command{
		Name:       commandName,
		ShortUsage: corecmd.CommandName + " " + commandName + " [flags] <action>",
		ShortHelp:  "Runs build, archive and release in sequence",
		FlagSet:    fs,
		Exec:       a.Exec,
	}
}

type all struct {
	infoLog logg.LevelLogger
	core    *corecmd.Core

	builder   *buildcmd.Builder
	archivist *archivecmd.Archivist
	releaser  *releasecmd.Releaser
}

func (a *all) Init() error {
	a.infoLog = a.core.InfoLog.WithField("all", commandName)
	return nil
}

// TODO(bep) add some contextual startup checks for GITHUB_TOKEN etc. (fail early)
func (a *all) Exec(ctx context.Context, args []string) error {
	if err := a.Init(); err != nil {
		return err
	}

	commandHandlers := []corecmd.CommandHandler{
		a.builder,
		a.archivist,
		a.releaser,
	}

	for _, commandHandler := range commandHandlers {
		if err := commandHandler.Init(); err != nil {
			return err
		}
	}

	for _, commandHandler := range commandHandlers {
		if err := commandHandler.Exec(ctx, args); err != nil {
			return err
		}
	}

	return nil

}