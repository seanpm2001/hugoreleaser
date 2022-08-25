// Copyright 2022 The Hugoreleaser Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/bep/logg"
	"github.com/gohugoio/hugoreleaser/cmd/allcmd"
	"github.com/gohugoio/hugoreleaser/cmd/archivecmd"
	"github.com/gohugoio/hugoreleaser/cmd/buildcmd"
	"github.com/gohugoio/hugoreleaser/cmd/corecmd"
	"github.com/gohugoio/hugoreleaser/cmd/releasecmd"
	"github.com/gohugoio/hugoreleaser/internal/common/logging"
	"github.com/peterbourgon/ff/v3"
	"github.com/peterbourgon/ff/v3/ffcli"
)

func main() {
	if err := parseAndRun(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func parseAndRun(args []string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("stacktrace from panic: \n" + string(debug.Stack()))
			err = fmt.Errorf("%v", r)
		}
	}()

	start := time.Now()

	var (
		coreCommand, core = corecmd.New()
		buildCommand      = buildcmd.New(core)
		archiveCommand    = archivecmd.New(core)
		releaseCommand    = releasecmd.New(core)
		allCommand        = allcmd.New(core)
	)

	coreCommand.Subcommands = []*ffcli.Command{
		buildCommand,
		archiveCommand,
		releaseCommand,
		allCommand,
	}

	opts := []ff.Option{
		ff.WithEnvVarPrefix(corecmd.EnvPrefix),
	}

	coreCommand.Options = opts
	for _, subCommand := range coreCommand.Subcommands {
		subCommand.Options = opts
	}

	releaseCommand.Options = []ff.Option{
		ff.WithEnvVarPrefix(corecmd.EnvPrefix),
	}

	defer func() {
		if closeErr := core.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("error closing app: %w", err)
		}
		elapsed := time.Since(start)
		s := logg.String(fmt.Sprintf("Total in %s …", logging.FormatBuildDuration(elapsed)))
		if core.InfoLog != nil {
			core.InfoLog.Log(s)
		} else {
			log.Print(s)
		}
	}()

	if err := core.PreInit(); err != nil {
		return fmt.Errorf("error in foo: %w", err)
	}

	if err := coreCommand.Parse(args); err != nil {
		return fmt.Errorf("error parsing command line: %w", err)
	}

	if core.Try {
		os.Setenv("GITHUB_TOKEN", "faketoken")
	}

	// Pass any non-empty flag value into the HUGORELEASER_ prefix in OS environment if not already set.
	coreCommand.FlagSet.VisitAll(func(f *flag.Flag) {
		envName := fmt.Sprintf("%s_%s", corecmd.EnvPrefix, strings.ToUpper(f.Name))
		if os.Getenv(envName) == "" {
			if s := f.Value.String(); s != "" {
				os.Setenv(envName, f.Value.String())
			}
		}

	})

	ctx, cancel := context.WithTimeout(context.Background(), core.Timeout)
	defer cancel()

	if err := core.Init(); err != nil {
		return fmt.Errorf("error initializing config: %w", err)
	}

	if err := coreCommand.Run(ctx); err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return fmt.Errorf("command timed out after %s; increase -timeout if needed", core.Timeout)
		}
		return fmt.Errorf("error running command: %w", err)
	}

	return err
}
