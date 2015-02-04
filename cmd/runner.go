package cmd

import (
	boshlog "github.com/cloudfoundry/bosh-agent/logger"
	boshtime "github.com/cloudfoundry/bosh-agent/time"

	bmui "github.com/cloudfoundry/bosh-micro-cli/ui"
)

type Runner struct {
	factory     Factory
	ui          bmui.UI
	timeService boshtime.Service
	logger      boshlog.Logger
}

func NewRunner(factory Factory, ui bmui.UI, timeService boshtime.Service, logger boshlog.Logger) *Runner {
	return &Runner{
		factory:     factory,
		ui:          ui,
		timeService: timeService,
		logger:      logger,
	}
}

func (r *Runner) Run(args ...string) {
	if len(args) == 0 {
		r.ui.ErrorLinef("Invalid usage: No command specified")
	}

	commandName := args[0]
	cmd, err := r.factory.CreateCommand(commandName)
	if err != nil {
		r.ui.ErrorLinef("Command '%s' unknown: %s", commandName, err)
	}

	stage := bmui.NewStage(r.ui, r.timeService, r.logger)

	err = cmd.Run(stage, args[1:])
	if err != nil {
		r.ui.ErrorLinef("Command '%s' failed: %s", commandName, err)
	}
}
