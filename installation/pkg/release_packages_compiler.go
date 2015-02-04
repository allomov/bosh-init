package pkg

import (
	"fmt"

	boshtime "github.com/cloudfoundry/bosh-agent/time"

	bmrel "github.com/cloudfoundry/bosh-micro-cli/release"
	bmrelpkg "github.com/cloudfoundry/bosh-micro-cli/release/pkg"
	bmui "github.com/cloudfoundry/bosh-micro-cli/ui"
)

type ReleasePackagesCompiler interface {
	Compile(bmrel.Release, bmui.Stage) error
}

type releasePackagesCompiler struct {
	packageCompiler PackageCompiler
	timeService     boshtime.Service
}

func NewReleasePackagesCompiler(
	packageCompiler PackageCompiler,
	timeService boshtime.Service,
) ReleasePackagesCompiler {
	return &releasePackagesCompiler{
		packageCompiler: packageCompiler,
		timeService:     timeService,
	}
}

func (c releasePackagesCompiler) Compile(release bmrel.Release, stage bmui.Stage) error {
	//TODO: should just take a list of packages, not a whole release [#85719162]
	// sort release packages in compilation order
	packages := bmrelpkg.Sort(release.Packages())

	for _, pkg := range packages {
		stepName := fmt.Sprintf("Compiling package '%s/%s'", pkg.Name, pkg.Fingerprint)
		err := stage.Perform(stepName, func() error {
			return c.packageCompiler.Compile(pkg)
		})
		if err != nil {
			return err
		}
	}

	return nil
}
