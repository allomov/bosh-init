package ui_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/cloudfoundry/bosh-micro-cli/ui"

	"bytes"
	"time"

	bosherr "github.com/cloudfoundry/bosh-agent/errors"
	boshlog "github.com/cloudfoundry/bosh-agent/logger"

	faketime "github.com/cloudfoundry/bosh-agent/time/fakes"
)

var _ = Describe("Stage", func() {
	var (
		logOutBuffer, logErrBuffer *bytes.Buffer
		logger                     boshlog.Logger

		stage       Stage
		ui          UI
		timeService *faketime.FakeService

		uiOut, uiErr *bytes.Buffer
	)

	BeforeEach(func() {
		uiOut = bytes.NewBufferString("")
		uiErr = bytes.NewBufferString("")

		logOutBuffer = bytes.NewBufferString("")
		logErrBuffer = bytes.NewBufferString("")
		logger = boshlog.NewWriterLogger(boshlog.LevelDebug, logOutBuffer, logErrBuffer)

		ui = NewWriterUI(uiOut, uiErr, logger)
		timeService = &faketime.FakeService{
			NowTimes: []time.Time{},
		}

		stage = NewStage(ui, timeService, logger)
	})

	Describe("Perform", func() {
		It("prints a single-line stage", func() {
			actionsPerformed := []string{}
			now := time.Now()
			timeService.NowTimes = []time.Time{
				now, // start stage 1
				now.Add(1 * time.Minute), // stop stage 1
			}

			err := stage.Perform("simple stage 1", func() error {
				actionsPerformed = append(actionsPerformed, "1")
				return nil
			})
			Expect(err).ToNot(HaveOccurred())

			expectedOutput := "Commencing simple stage 1... Completed (00:01:00)\n"

			Expect(uiOut.String()).To(Equal(expectedOutput))

			Expect(actionsPerformed).To(Equal([]string{"1"}))
		})

		It("fails on error", func() {
			actionsPerformed := []string{}
			now := time.Now()
			timeService.NowTimes = []time.Time{
				now, // start stage 1
				now.Add(1 * time.Minute), // stop stage 1
			}

			err := stage.Perform("simple stage 1", func() error {
				actionsPerformed = append(actionsPerformed, "1")
				return bosherr.Error("fake-stage-1-error")
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("fake-stage-1-error"))

			expectedOutput := "Commencing simple stage 1... Failed (00:01:00)\n"

			Expect(uiOut.String()).To(Equal(expectedOutput))

			Expect(actionsPerformed).To(Equal([]string{"1"}))
		})

		It("logs skip errors", func() {
			actionsPerformed := []string{}
			now := time.Now()
			timeService.NowTimes = []time.Time{
				now, // start stage 1
				now.Add(1 * time.Minute), // stop stage 1
			}

			err := stage.Perform("simple stage 1", func() error {
				actionsPerformed = append(actionsPerformed, "1")
				cause := bosherr.Error("fake-skip-error")
				return NewSkipStageError(cause, "fake-skip-message")
			})
			Expect(err).ToNot(HaveOccurred())

			expectedOutput := "Commencing simple stage 1... Skipped [fake-skip-message] (00:01:00)\n"

			Expect(uiOut.String()).To(Equal(expectedOutput))

			Expect(logOutBuffer.String()).To(ContainSubstring("fake-skip-message: fake-skip-error"))

			Expect(actionsPerformed).To(Equal([]string{"1"}))
		})
	})

	Describe("PerformComplex", func() {
		It("prints a multi-line stage (depth: 1)", func() {
			actionsPerformed := []string{}
			now := time.Now()
			timeService.NowTimes = []time.Time{
				now, // start stage 1
				now, // start stage A
				now.Add(1 * time.Minute), // stop stage A
				now.Add(1 * time.Minute), // start stage B
				now.Add(2 * time.Minute), // stop stage B
				now.Add(2 * time.Minute), // stop stage 1
			}

			err := stage.PerformComplex("complex stage 1", func(stage Stage) error {
				err := stage.Perform("simple stage A", func() error {
					actionsPerformed = append(actionsPerformed, "A")
					return nil
				})
				if err != nil {
					return err
				}

				err = stage.Perform("simple stage B", func() error {
					actionsPerformed = append(actionsPerformed, "B")
					return nil
				})
				if err != nil {
					return err
				}

				return nil
			})
			Expect(err).ToNot(HaveOccurred())

			expectedOutput := `Commencing complex stage 1
  Commencing simple stage A... Completed (00:01:00)
  Commencing simple stage B... Completed (00:01:00)
Completed complex stage 1 (00:02:00)
`

			Expect(uiOut.String()).To(Equal(expectedOutput))

			Expect(actionsPerformed).To(Equal([]string{"A", "B"}))
		})

		It("prints a multi-line stage (depth: >1)", func() {
			actionsPerformed := []string{}
			now := time.Now()
			timeService.NowTimes = []time.Time{
				now, // start stage 1
				now, // start stage A
				now.Add(1 * time.Minute), // stop stage A
				now.Add(1 * time.Minute), // start stage B
				now.Add(1 * time.Minute), // start stage X
				now.Add(2 * time.Minute), // stop stage X
				now.Add(2 * time.Minute), // start stage Y
				now.Add(3 * time.Minute), // stop stage Y
				now.Add(3 * time.Minute), // stop stage B
				now.Add(3 * time.Minute), // stop stage 1
			}

			err := stage.PerformComplex("complex stage 1", func(stage Stage) error {
				err := stage.Perform("simple stage A", func() error {
					actionsPerformed = append(actionsPerformed, "A")
					return nil
				})
				if err != nil {
					return err
				}

				err = stage.PerformComplex("complex stage B", func(stage Stage) error {
					err := stage.Perform("simple stage X", func() error {
						actionsPerformed = append(actionsPerformed, "X")
						return nil
					})
					if err != nil {
						return err
					}

					err = stage.Perform("simple stage Y", func() error {
						actionsPerformed = append(actionsPerformed, "Y")
						return nil
					})
					if err != nil {
						return err
					}

					return nil
				})
				if err != nil {
					return err
				}

				return nil
			})
			Expect(err).ToNot(HaveOccurred())

			expectedOutput := `Commencing complex stage 1
  Commencing simple stage A... Completed (00:01:00)
  Commencing complex stage B
    Commencing simple stage X... Completed (00:01:00)
    Commencing simple stage Y... Completed (00:01:00)
  Completed complex stage B (00:02:00)
Completed complex stage 1 (00:03:00)
`

			Expect(uiOut.String()).To(Equal(expectedOutput))

			Expect(actionsPerformed).To(Equal([]string{"A", "X", "Y"}))
		})

		It("fails on error", func() {
			actionsPerformed := []string{}
			now := time.Now()
			timeService.NowTimes = []time.Time{
				now, // start stage 1
				now.Add(1 * time.Minute), // stop stage 1
			}

			err := stage.PerformComplex("complex stage 1", func(stage Stage) error {
				actionsPerformed = append(actionsPerformed, "1")
				return bosherr.Error("fake-stage-1-error")
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("fake-stage-1-error"))

			expectedOutput := `Commencing complex stage 1
Failed complex stage 1 (00:01:00)
`

			Expect(uiOut.String()).To(Equal(expectedOutput))

			Expect(actionsPerformed).To(Equal([]string{"1"}))
		})
	})
})
