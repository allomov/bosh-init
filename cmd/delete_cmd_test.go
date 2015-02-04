package cmd_test

import (
	. "github.com/cloudfoundry/bosh-micro-cli/cmd"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"os"

	"code.google.com/p/gomock/gomock"
	mock_blobstore "github.com/cloudfoundry/bosh-micro-cli/blobstore/mocks"
	mock_cloud "github.com/cloudfoundry/bosh-micro-cli/cloud/mocks"
	mock_httpagent "github.com/cloudfoundry/bosh-micro-cli/deployment/agentclient/http/mocks"
	mock_agentclient "github.com/cloudfoundry/bosh-micro-cli/deployment/agentclient/mocks"
	mock_deployment "github.com/cloudfoundry/bosh-micro-cli/deployment/mocks"
	mock_install "github.com/cloudfoundry/bosh-micro-cli/installation/mocks"
	mock_release "github.com/cloudfoundry/bosh-micro-cli/release/mocks"

	boshlog "github.com/cloudfoundry/bosh-agent/logger"
	boshsys "github.com/cloudfoundry/bosh-agent/system"

	fakesys "github.com/cloudfoundry/bosh-agent/system/fakes"
	fakeuuid "github.com/cloudfoundry/bosh-agent/uuid/fakes"

	bmconfig "github.com/cloudfoundry/bosh-micro-cli/config"
	bminstallmanifest "github.com/cloudfoundry/bosh-micro-cli/installation/manifest"
	bmrel "github.com/cloudfoundry/bosh-micro-cli/release"
	bmreljob "github.com/cloudfoundry/bosh-micro-cli/release/job"
	bmrelpkg "github.com/cloudfoundry/bosh-micro-cli/release/pkg"
	bmrelset "github.com/cloudfoundry/bosh-micro-cli/release/set"
	bmrelsetmanifest "github.com/cloudfoundry/bosh-micro-cli/release/set/manifest"
	bmui "github.com/cloudfoundry/bosh-micro-cli/ui"

	fakebmui "github.com/cloudfoundry/bosh-micro-cli/ui/fakes"
	fakeui "github.com/cloudfoundry/bosh-micro-cli/ui/fakes"
)

var _ = Describe("DeleteCmd", func() {
	var mockCtrl *gomock.Controller

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Describe("Run", func() {
		var (
			fs                      boshsys.FileSystem
			logger                  boshlog.Logger
			releaseManager          bmrel.Manager
			mockInstaller           *mock_install.MockInstaller
			mockInstallerFactory    *mock_install.MockInstallerFactory
			mockInstallation        *mock_install.MockInstallation
			mockCloudFactory        *mock_cloud.MockFactory
			mockReleaseExtractor    *mock_release.MockExtractor
			fakeUUIDGenerator       *fakeuuid.FakeGenerator
			deploymentConfigService bmconfig.DeploymentConfigService
			userConfig              bmconfig.UserConfig

			fakeUI *fakeui.FakeUI

			mockBlobstoreFactory *mock_blobstore.MockFactory
			mockBlobstore        *mock_blobstore.MockBlobstore

			mockDeploymentManagerFactory *mock_deployment.MockManagerFactory
			mockDeploymentManager        *mock_deployment.MockManager
			mockDeployment               *mock_deployment.MockDeployment

			mockAgentClient        *mock_agentclient.MockAgentClient
			mockAgentClientFactory *mock_httpagent.MockAgentClientFactory
			mockCloud              *mock_cloud.MockCloud

			fakePerentStage     *fakebmui.FakeStage
			fakeValidatingStage *fakebmui.FakeStage
			fakeInstallingStage *fakebmui.FakeStage
			fakeDeletingStage   *fakebmui.FakeStage

			directorID string

			deploymentManifestPath = "/deployment-dir/fake-deployment-manifest.yml"
			deploymentConfigPath   = "/fake-bosh-deployments.json"

			expectCPIExtractRelease *gomock.Call
			expectCPIInstall        *gomock.Call
			expectNewCloud          *gomock.Call
			expectStartRegistry     *gomock.Call
			expectStopRegistry      *gomock.Call

			mbusURL = "http://fake-mbus-user:fake-mbus-password@fake-mbus-endpoint"
		)

		var writeDeploymentManifest = func() {
			fs.WriteFileString(deploymentManifestPath, `---
name: test-release

cloud_provider:
  release: fake-cpi-release-name
  mbus: http://fake-mbus-user:fake-mbus-password@fake-mbus-endpoint
`)
		}

		var writeCPIReleaseTarball = func() {
			fs.WriteFileString("/fake-cpi-release.tgz", "fake-tgz-content")
		}

		var allowCPIToBeExtracted = func() {
			cpiRelease := bmrel.NewRelease(
				"fake-cpi-release-name",
				"fake-cpi-release-version",
				[]bmreljob.Job{
					{
						Name: "cpi",
						Templates: map[string]string{
							"templates/cpi.erb": "bin/cpi",
						},
					},
				},
				[]*bmrelpkg.Package{},
				"fake-cpi-extracted-dir",
				fs,
			)

			expectCPIExtractRelease = mockReleaseExtractor.EXPECT().Extract("/fake-cpi-release.tgz").Do(func(_ string) {
				err := fs.MkdirAll("fake-cpi-extracted-dir", os.ModePerm)
				Expect(err).ToNot(HaveOccurred())
			}).Return(cpiRelease, nil).AnyTimes()
		}

		var allowCPIToBeInstalled = func() {
			installationManifest := bminstallmanifest.Manifest{
				Name:    "test-release",
				Mbus:    mbusURL,
				Release: "fake-cpi-release-name",
			}

			mockInstallerFactory.EXPECT().NewInstaller().Return(mockInstaller, nil).AnyTimes()

			expectCPIInstall = mockInstaller.EXPECT().Install(installationManifest, fakeInstallingStage).Return(mockInstallation, nil).AnyTimes()

			expectNewCloud = mockCloudFactory.EXPECT().NewCloud(mockInstallation, directorID).Return(mockCloud, nil).AnyTimes()

			expectStartRegistry = mockInstallation.EXPECT().StartRegistry().AnyTimes()
			expectStopRegistry = mockInstallation.EXPECT().StopRegistry().AnyTimes()
		}

		var newDeleteCmd = func() Cmd {
			releaseSetParser := bmrelsetmanifest.NewParser(fs, logger)
			releaseSetResolver := bmrelset.NewResolver(releaseManager, logger)
			releaseSetValidator := bmrelsetmanifest.NewValidator(logger, releaseSetResolver)
			installationValidator := bminstallmanifest.NewValidator(logger, releaseSetResolver)
			installationParser := bminstallmanifest.NewParser(fs, logger)

			return NewDeleteCmd(
				fakeUI,
				userConfig,
				fs,
				releaseSetParser,
				installationParser,
				deploymentConfigService,
				releaseSetValidator,
				installationValidator,
				mockInstallerFactory,
				mockReleaseExtractor,
				releaseManager,
				releaseSetResolver,
				mockCloudFactory,
				mockAgentClientFactory,
				mockBlobstoreFactory,
				mockDeploymentManagerFactory,
				fakePerentStage,
				logger,
			)
		}

		var expectDeleteAndCleanup = func() {
			mockDeploymentManagerFactory.EXPECT().NewManager(mockCloud, mockAgentClient, mockBlobstore).Return(mockDeploymentManager)
			mockDeploymentManager.EXPECT().FindCurrent().Return(mockDeployment, true, nil)

			//TODO: can we check that the stage is "deleting deployment"?
			gomock.InOrder(
				mockDeployment.EXPECT().Delete(gomock.Any()),
				mockDeploymentManager.EXPECT().Cleanup(gomock.Any()),
			)
		}

		var expectCleanup = func() {
			mockDeploymentManagerFactory.EXPECT().NewManager(mockCloud, mockAgentClient, mockBlobstore).Return(mockDeploymentManager).AnyTimes()
			mockDeploymentManager.EXPECT().FindCurrent().Return(nil, false, nil).AnyTimes()

			//TODO: can we check that the stage is "deleting deployment"?
			mockDeploymentManager.EXPECT().Cleanup(gomock.Any())
		}

		var expectValidationInstallationDeletionEvents = func() {
			Expect(fakeUI.Said).To(Equal([]string{
				"Deployment manifest: '/deployment-dir/fake-deployment-manifest.yml'",
				"Deployment state: '/deployment-dir/deployment.json'",
			}))

			Expect(fakePerentStage.NewStageInputs).To(Equal([]fakebmui.NewStageInput{
				{Name: "validating"},
				{Name: "installing CPI"},
				{Name: "deleting deployment"},
			}))

			Expect(fakeValidatingStage.Steps).To(Equal([]*fakebmui.FakeStep{
				&fakebmui.FakeStep{
					Name: "Validating releases",
					States: []bmui.EventState{
						bmui.Started,
						bmui.Finished,
					},
				},
				&fakebmui.FakeStep{
					Name: "Validating deployment manifest",
					States: []bmui.EventState{
						bmui.Started,
						bmui.Finished,
					},
				},
				&fakebmui.FakeStep{
					Name: "Validating cpi release",
					States: []bmui.EventState{
						bmui.Started,
						bmui.Finished,
					},
				},
			}))

			// installing steps handled by installer.Install()
			// deleting steps handled by deployment.Delete()
		}

		BeforeEach(func() {
			fs = fakesys.NewFakeFileSystem()
			logger = boshlog.NewLogger(boshlog.LevelNone)
			fakeUUIDGenerator = fakeuuid.NewFakeGenerator()
			deploymentConfigService = bmconfig.NewFileSystemDeploymentConfigService(deploymentConfigPath, fs, fakeUUIDGenerator, logger)

			fakeUI = &fakeui.FakeUI{}

			fakePerentStage = fakebmui.NewFakeStage()
			fakeValidatingStage = fakebmui.NewFakeStage()
			fakePerentStage.SetNewStageBehavior("validating", fakeValidatingStage)
			fakeInstallingStage = fakebmui.NewFakeStage()
			fakePerentStage.SetNewStageBehavior("installing CPI", fakeInstallingStage)
			fakeDeletingStage = fakebmui.NewFakeStage()
			fakePerentStage.SetNewStageBehavior("deleting deployment", fakeDeletingStage)

			mockCloud = mock_cloud.NewMockCloud(mockCtrl)
			mockCloudFactory = mock_cloud.NewMockFactory(mockCtrl)

			mockInstaller = mock_install.NewMockInstaller(mockCtrl)
			mockInstallerFactory = mock_install.NewMockInstallerFactory(mockCtrl)
			mockInstallation = mock_install.NewMockInstallation(mockCtrl)

			mockBlobstoreFactory = mock_blobstore.NewMockFactory(mockCtrl)
			mockBlobstore = mock_blobstore.NewMockBlobstore(mockCtrl)
			mockBlobstoreFactory.EXPECT().Create(mbusURL).Return(mockBlobstore, nil).AnyTimes()

			mockDeploymentManagerFactory = mock_deployment.NewMockManagerFactory(mockCtrl)
			mockDeploymentManager = mock_deployment.NewMockManager(mockCtrl)
			mockDeployment = mock_deployment.NewMockDeployment(mockCtrl)

			mockReleaseExtractor = mock_release.NewMockExtractor(mockCtrl)
			releaseManager = bmrel.NewManager(logger)

			mockAgentClientFactory = mock_httpagent.NewMockAgentClientFactory(mockCtrl)
			mockAgentClient = mock_agentclient.NewMockAgentClient(mockCtrl)

			userConfig = bmconfig.UserConfig{DeploymentManifestPath: deploymentManifestPath}

			mockAgentClientFactory.EXPECT().NewAgentClient(gomock.Any(), gomock.Any()).Return(mockAgentClient).AnyTimes()

			directorID = "fake-uuid-0"

			writeDeploymentManifest()
			writeCPIReleaseTarball()
		})

		JustBeforeEach(func() {
			allowCPIToBeExtracted()
			allowCPIToBeInstalled()
		})

		Context("when the deployment has not been set", func() {
			BeforeEach(func() {
				userConfig.DeploymentManifestPath = ""
			})

			It("returns an error", func() {
				err := newDeleteCmd().Run([]string{"/fake-cpi-release.tgz"})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("Running delete cmd: Deployment manifest not set"))
				Expect(fakeUI.Errors).To(ContainElement("Deployment manifest not set"))
			})
		})

		Context("when the deployment config file does not exist", func() {
			BeforeEach(func() {
				err := fs.RemoveAll(deploymentConfigPath)
				Expect(err).ToNot(HaveOccurred())
			})

			It("does not delete anything", func() {
				expectCleanup()

				err := newDeleteCmd().Run([]string{"/fake-cpi-release.tgz"})
				Expect(err).ToNot(HaveOccurred())

				expectValidationInstallationDeletionEvents()
			})
		})

		Context("when the deployment has been deployed", func() {
			BeforeEach(func() {
				directorID = "fake-director-id"

				// create deployment manifest yaml file
				deploymentConfigService.Save(bmconfig.DeploymentFile{
					DirectorID: directorID,
				})
			})

			It("extracts & install CPI release tarball", func() {
				expectDeleteAndCleanup()

				gomock.InOrder(
					expectCPIExtractRelease.Times(1),
					expectCPIInstall.Times(1),
					expectNewCloud.Times(1),
				)

				err := newDeleteCmd().Run([]string{"/fake-cpi-release.tgz"})
				Expect(err).NotTo(HaveOccurred())
			})

			It("starts & stops the registry", func() {
				expectDeleteAndCleanup()

				gomock.InOrder(
					expectStartRegistry.Times(1),
					expectStopRegistry.Times(1),
				)

				err := newDeleteCmd().Run([]string{"/fake-cpi-release.tgz"})
				Expect(err).NotTo(HaveOccurred())
			})

			It("deletes the extracted CPI release", func() {
				expectDeleteAndCleanup()

				err := newDeleteCmd().Run([]string{"/fake-cpi-release.tgz"})
				Expect(err).NotTo(HaveOccurred())
				Expect(fs.FileExists("fake-cpi-extracted-dir")).To(BeFalse())
			})

			It("deletes the deployment & cleans up orphans", func() {
				expectDeleteAndCleanup()

				err := newDeleteCmd().Run([]string{"/fake-cpi-release.tgz"})
				Expect(err).ToNot(HaveOccurred())
				Expect(fakeUI.Errors).To(BeEmpty())
			})

			It("logs validating & deleting stages", func() {
				expectDeleteAndCleanup()

				err := newDeleteCmd().Run([]string{"/fake-cpi-release.tgz"})
				Expect(err).ToNot(HaveOccurred())

				expectValidationInstallationDeletionEvents()
			})
		})

		Context("when nothing has been deployed", func() {
			BeforeEach(func() {
				deploymentConfigService.Save(bmconfig.DeploymentFile{})

				directorID = "fake-uuid-0"
			})

			It("cleans up orphans, but does not delete any deployment", func() {
				expectCleanup()

				err := newDeleteCmd().Run([]string{"/fake-cpi-release.tgz"})
				Expect(err).ToNot(HaveOccurred())
				Expect(fakeUI.Errors).To(BeEmpty())
			})
		})
	})
})
