package fakes

import (
	bmrel "github.com/cloudfoundry/bosh-micro-cli/release"
	bmui "github.com/cloudfoundry/bosh-micro-cli/ui"
)

type FakeReleasePackagesCompiler struct {
	CompileError   error
	CompileRelease bmrel.Release
	CompileStage   bmui.Stage
}

func NewFakeReleasePackagesCompiler() *FakeReleasePackagesCompiler {
	return &FakeReleasePackagesCompiler{}
}

func (c *FakeReleasePackagesCompiler) Compile(release bmrel.Release, stage bmui.Stage) error {
	c.CompileRelease = release
	c.CompileStage = stage

	return c.CompileError
}
