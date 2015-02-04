package job_test

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	bmreljob "github.com/cloudfoundry/bosh-micro-cli/release/job"
	bmrelpkg "github.com/cloudfoundry/bosh-micro-cli/release/pkg"
	bmtemplate "github.com/cloudfoundry/bosh-micro-cli/templatescompiler"
	bmui "github.com/cloudfoundry/bosh-micro-cli/ui"

	fakesys "github.com/cloudfoundry/bosh-agent/system/fakes"
	faketime "github.com/cloudfoundry/bosh-agent/time/fakes"
	fakebmlog "github.com/cloudfoundry/bosh-micro-cli/eventlogger/fakes"
	fakebminstallblob "github.com/cloudfoundry/bosh-micro-cli/installation/blob/fakes"
	fakebminstallpkg "github.com/cloudfoundry/bosh-micro-cli/installation/pkg/fakes"
	fakebmtemcomp "github.com/cloudfoundry/bosh-micro-cli/templatescompiler/fakes"

	. "github.com/cloudfoundry/bosh-micro-cli/installation/job"
)

var _ = Describe("Installer", func() {
	var (
		fs               *fakesys.FakeFileSystem
		jobInstaller     Installer
		job              bmreljob.Job
		packageInstaller *fakebminstallpkg.FakePackageInstaller
		blobExtractor    *fakebminstallblob.FakeExtractor
		templateRepo     *fakebmtemcomp.FakeTemplatesRepo
		jobsPath         string
		packagesPath     string
		fakeStage        *fakebmlog.FakeStage
		timeService      *faketime.FakeService
	)

	Context("Installing the job", func() {
		BeforeEach(func() {
			fs = fakesys.NewFakeFileSystem()
			packageInstaller = fakebminstallpkg.NewFakePackageInstaller()
			blobExtractor = fakebminstallblob.NewFakeExtractor()
			templateRepo = fakebmtemcomp.NewFakeTemplatesRepo()

			jobsPath = "/fake/jobs"
			packagesPath = "/fake/packages"
			fakeStage = fakebmlog.NewFakeStage()
			timeService = &faketime.FakeService{}

			jobInstaller = NewInstaller(fs, packageInstaller, blobExtractor, templateRepo, jobsPath, packagesPath, timeService)
			job = bmreljob.Job{
				Name: "cpi",
			}

			templateRepo.SetFindBehavior(job, bmtemplate.TemplateRecord{BlobID: "fake-blob-id", BlobSHA1: "fake-sha1"}, true, nil)
			blobExtractor.SetExtractBehavior("fake-blob-id", "fake-sha1", "/fake/jobs/cpi", nil)
		})

		It("makes the files in the job's bin directory executable", func() {
			cpiExecutablePath := "/fake/jobs/cpi/bin/cpi"
			fs.SetGlob("/fake/jobs/cpi/bin/*", []string{cpiExecutablePath})
			fs.WriteFileString(cpiExecutablePath, "contents")
			_, err := jobInstaller.Install(job, fakeStage)
			Expect(err).ToNot(HaveOccurred())
			Expect(fs.GetFileTestStat(cpiExecutablePath).FileMode).To(Equal(os.FileMode(0755)))
		})

		It("returns a record of the installed job", func() {
			installedJob, err := jobInstaller.Install(job, fakeStage)
			Expect(err).ToNot(HaveOccurred())
			Expect(installedJob).To(Equal(
				InstalledJob{
					Name: "cpi",
					Path: "/fake/jobs/cpi",
				},
			))
		})

		It("creates basic job layout", func() {
			_, err := jobInstaller.Install(job, fakeStage)
			Expect(err).ToNot(HaveOccurred())
			Expect(fs.FileExists(filepath.Join(jobsPath, job.Name))).To(BeTrue())
			Expect(fs.FileExists(packagesPath)).To(BeTrue())
		})

		It("finds the rendered templates for the job from the repo", func() {
			_, err := jobInstaller.Install(job, fakeStage)
			Expect(err).ToNot(HaveOccurred())
			Expect(templateRepo.FindInputs).To(ContainElement(fakebmtemcomp.FindInput{Job: job}))
		})

		It("tells the blobExtractor to extract the templates into the installed job dir", func() {
			_, err := jobInstaller.Install(job, fakeStage)
			Expect(err).ToNot(HaveOccurred())
			Expect(blobExtractor.ExtractInputs).To(ContainElement(fakebminstallblob.ExtractInput{
				BlobID:    "fake-blob-id",
				BlobSHA1:  "fake-sha1",
				TargetDir: filepath.Join(jobsPath, job.Name),
			}))
		})

		It("logs events to the event logger", func() {
			installStart := time.Now()
			installFinish := installStart.Add(1 * time.Second)
			timeService.NowTimes = []time.Time{installStart, installFinish}

			_, err := jobInstaller.Install(job, fakeStage)
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeStage.Steps).To(ContainElement(&fakebmlog.FakeStep{
				Name: "Installing job 'cpi'",
				States: []bmui.EventState{
					bmui.Started,
					bmui.Finished,
				},
			}))
		})

		It("logs failure event", func() {
			fs.MkdirAllError = errors.New("fake-mkdir-error")

			installStart := time.Now()
			installFail := installStart.Add(1 * time.Second)
			timeService.NowTimes = []time.Time{installStart, installFail}

			_, err := jobInstaller.Install(job, fakeStage)
			Expect(err).To(HaveOccurred())

			Expect(fakeStage.Steps).To(ContainElement(&fakebmlog.FakeStep{
				Name: "Installing job 'cpi'",
				States: []bmui.EventState{
					bmui.Started,
					bmui.Failed,
				},
				FailMessage: "Creating job directory '/fake/jobs/cpi': fake-mkdir-error",
			}))
		})

		Context("when the job has packages", func() {
			var pkg1 bmrelpkg.Package

			BeforeEach(func() {
				pkg1 = bmrelpkg.Package{Name: "fake-pkg-name"}
				job.Packages = []*bmrelpkg.Package{&pkg1}
				packageInstaller.SetInstallBehavior(&pkg1, packagesPath, nil)
				templateRepo.SetFindBehavior(job, bmtemplate.TemplateRecord{BlobID: "fake-blob-id", BlobSHA1: "fake-sha1"}, true, nil)
			})

			It("install packages correctly", func() {
				_, err := jobInstaller.Install(job, fakeStage)
				Expect(err).ToNot(HaveOccurred())
				Expect(packageInstaller.InstallInputs).To(ContainElement(
					fakebminstallpkg.InstallInput{Package: &pkg1, Target: packagesPath},
				))
			})

			It("return err when package installation fails", func() {
				packageInstaller.SetInstallBehavior(
					&pkg1,
					packagesPath,
					errors.New("Installation failed, yo"),
				)
				_, err := jobInstaller.Install(job, fakeStage)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Installation failed"))
			})
		})
	})
})
