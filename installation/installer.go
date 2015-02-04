package installation

import (
	bosherr "github.com/cloudfoundry/bosh-agent/errors"
	boshlog "github.com/cloudfoundry/bosh-agent/logger"

	bmcpirel "github.com/cloudfoundry/bosh-micro-cli/cpi/release"
	bminstalljob "github.com/cloudfoundry/bosh-micro-cli/installation/job"
	bminstallmanifest "github.com/cloudfoundry/bosh-micro-cli/installation/manifest"
	bminstallpkg "github.com/cloudfoundry/bosh-micro-cli/installation/pkg"
	bmregistry "github.com/cloudfoundry/bosh-micro-cli/registry"
	bmrelset "github.com/cloudfoundry/bosh-micro-cli/release/set"
	bmui "github.com/cloudfoundry/bosh-micro-cli/ui"
)

type Installer interface {
	Install(bminstallmanifest.Manifest, bmui.Stage) (Installation, error)
}

type installer struct {
	target                Target
	ui                    bmui.UI
	releaseResolver       bmrelset.Resolver
	releaseCompiler       bminstallpkg.ReleaseCompiler
	jobInstaller          bminstalljob.Installer
	registryServerManager bmregistry.ServerManager
	logger                boshlog.Logger
	logTag                string
}

func NewInstaller(
	target Target,
	ui bmui.UI,
	releaseResolver bmrelset.Resolver,
	releaseCompiler bminstallpkg.ReleaseCompiler,
	jobInstaller bminstalljob.Installer,
	registryServerManager bmregistry.ServerManager,
	logger boshlog.Logger,
) Installer {
	return &installer{
		target:                target,
		ui:                    ui,
		releaseResolver:       releaseResolver,
		releaseCompiler:       releaseCompiler,
		jobInstaller:          jobInstaller,
		registryServerManager: registryServerManager,
		logger:                logger,
		logTag:                "installer",
	}
}

func (i *installer) Install(manifest bminstallmanifest.Manifest, stage bmui.Stage) (Installation, error) {
	i.logger.Info(i.logTag, "Installing CPI deployment '%s'", manifest.Name)
	i.logger.Debug(i.logTag, "Installing CPI deployment '%s' with manifest: %#v", manifest.Name, manifest)

	releaseName := manifest.Release
	release, err := i.releaseResolver.Find(releaseName)
	if err != nil {
		i.ui.ErrorLinef("Could not find CPI release '%s'", releaseName)
		return nil, bosherr.WrapErrorf(err, "CPI release '%s' not found", releaseName)
	}

	if !release.Exists() {
		i.ui.ErrorLinef("Could not find extracted CPI release")
		return nil, bosherr.Errorf("Extracted CPI release does not exist")
	}

	err = i.releaseCompiler.Compile(release, manifest, stage)
	if err != nil {
		i.ui.ErrorLinef("Could not compile CPI release")
		return nil, bosherr.WrapError(err, "Compiling CPI release")
	}

	cpiJobName := bmcpirel.ReleaseJobName
	releaseJob, found := release.FindJobByName(cpiJobName)

	if !found {
		i.ui.ErrorLinef("Could not find CPI job '%s' in release '%s'", cpiJobName, release.Name())
		return nil, bosherr.Errorf("Invalid CPI release: job '%s' not found in release '%s'", cpiJobName, release.Name())
	}

	installedJob, err := i.jobInstaller.Install(releaseJob, stage)
	if err != nil {
		i.ui.ErrorLinef("Could not install job '%s'", releaseJob.Name)
		return nil, bosherr.WrapErrorf(err, "Installing job '%s' for CPI release", releaseJob.Name)
	}

	return NewInstallation(
		i.target,
		installedJob,
		manifest,
		i.registryServerManager,
	), nil
}
