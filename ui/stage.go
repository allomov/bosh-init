package ui

import (
	"time"

	boshlog "github.com/cloudfoundry/bosh-agent/logger"
	boshtime "github.com/cloudfoundry/bosh-agent/time"

	bmtime "github.com/cloudfoundry/bosh-micro-cli/ui/time"
)

type Stage interface {
	Perform(name string, closure func() error) error
	PerformComplex(name string, closure func(Stage) error) error
}

type stage struct {
	ui          UI
	timeService boshtime.Service
	logger      boshlog.Logger
	logTag      string
}

func NewStage(ui UI, timeService boshtime.Service, logger boshlog.Logger) Stage {
	return &stage{
		ui:          ui,
		timeService: timeService,
		logger:      logger,
		logTag:      "stage",
	}
}

func (s *stage) Perform(name string, closure func() error) error {
	s.ui.BeginLinef("Commencing %s...", name)
	startTime := s.timeService.Now()
	err := closure()
	if err != nil {
		if skipErr, ok := err.(SkipStageError); ok {
			s.ui.EndLinef(" Skipped [%s] (%s)", skipErr.SkipMessage(), s.elapsedSince(startTime))
			s.logger.Info("Skipped stage '%s': %s", name, skipErr.Error())
			return nil
		}
		s.ui.EndLinef(" Failed (%s)", s.elapsedSince(startTime))
		return err
	}
	s.ui.EndLinef(" Completed (%s)", s.elapsedSince(startTime))
	return nil
}

func (s *stage) PerformComplex(name string, closure func(Stage) error) error {
	s.ui.PrintLinef("Commencing %s", name)
	startTime := s.timeService.Now()
	err := closure(s.newSubStage())
	if err != nil {
		s.ui.PrintLinef("Failed %s (%s)", name, s.elapsedSince(startTime))
		return err
	}
	s.ui.PrintLinef("Completed %s (%s)", name, s.elapsedSince(startTime))
	return nil
}

func (s *stage) elapsedSince(startTime time.Time) string {
	stopTime := s.timeService.Now()
	duration := stopTime.Sub(startTime)
	return bmtime.Format(duration)
}

func (s *stage) newSubStage() Stage {
	return NewStage(NewIndentingUI(s.ui), s.timeService, s.logger)
}
