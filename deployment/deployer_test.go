package deployment_test

import (
	"errors"
	"time"

	. "github.com/cloudfoundry/bosh-micro-cli/deployment"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"code.google.com/p/gomock/gomock"
	mock_blobstore "github.com/cloudfoundry/bosh-micro-cli/blobstore/mocks"
	mock_httpagent "github.com/cloudfoundry/bosh-micro-cli/deployment/agentclient/http/mocks"
	mock_agentclient "github.com/cloudfoundry/bosh-micro-cli/deployment/agentclient/mocks"
	mock_instance_state "github.com/cloudfoundry/bosh-micro-cli/deployment/instance/state/mocks"
	mock_vm "github.com/cloudfoundry/bosh-micro-cli/deployment/vm/mocks"

	boshlog "github.com/cloudfoundry/bosh-agent/logger"
	bmproperty "github.com/cloudfoundry/bosh-micro-cli/common/property"
	bmconfig "github.com/cloudfoundry/bosh-micro-cli/config"
	bmas "github.com/cloudfoundry/bosh-micro-cli/deployment/applyspec"
	bminstance "github.com/cloudfoundry/bosh-micro-cli/deployment/instance"
	bmdeplmanifest "github.com/cloudfoundry/bosh-micro-cli/deployment/manifest"
	bmsshtunnel "github.com/cloudfoundry/bosh-micro-cli/deployment/sshtunnel"
	bminstallmanifest "github.com/cloudfoundry/bosh-micro-cli/installation/manifest"
	bmstemcell "github.com/cloudfoundry/bosh-micro-cli/stemcell"
	bmui "github.com/cloudfoundry/bosh-micro-cli/ui"

	fakebmcloud "github.com/cloudfoundry/bosh-micro-cli/cloud/fakes"
	fakebmconfig "github.com/cloudfoundry/bosh-micro-cli/config/fakes"
	fakebmsshtunnel "github.com/cloudfoundry/bosh-micro-cli/deployment/sshtunnel/fakes"
	fakebmvm "github.com/cloudfoundry/bosh-micro-cli/deployment/vm/fakes"
	fakebmlog "github.com/cloudfoundry/bosh-micro-cli/eventlogger/fakes"
)

var _ = Describe("Deployer", func() {
	var mockCtrl *gomock.Controller

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	var (
		deployer               Deployer
		mockVMManagerFactory   *mock_vm.MockManagerFactory
		fakeVMManager          *fakebmvm.FakeManager
		mockAgentClient        *mock_agentclient.MockAgentClient
		mockAgentClientFactory *mock_httpagent.MockAgentClientFactory
		fakeSSHTunnelFactory   *fakebmsshtunnel.FakeFactory
		fakeSSHTunnel          *fakebmsshtunnel.FakeTunnel
		cloud                  *fakebmcloud.FakeCloud
		deploymentManifest     bmdeplmanifest.Manifest
		diskPool               bmdeplmanifest.DiskPool
		registryConfig         bminstallmanifest.Registry
		eventLogger            *fakebmlog.FakeEventLogger
		fakeStage              *fakebmlog.FakeStage
		sshTunnelConfig        bminstallmanifest.SSHTunnel
		fakeVM                 *fakebmvm.FakeVM

		cloudStemcell bmstemcell.CloudStemcell

		applySpec bmas.ApplySpec

		mockStateBuilderFactory *mock_instance_state.MockBuilderFactory
		mockStateBuilder        *mock_instance_state.MockBuilder
		mockState               *mock_instance_state.MockState

		mockBlobstore *mock_blobstore.MockBlobstore
	)

	BeforeEach(func() {
		diskPool = bmdeplmanifest.DiskPool{
			Name:     "fake-persistent-disk-pool-name",
			DiskSize: 1024,
			CloudProperties: bmproperty.Map{
				"fake-disk-pool-cloud-property-key": "fake-disk-pool-cloud-property-value",
			},
		}
		deploymentManifest = bmdeplmanifest.Manifest{
			Update: bmdeplmanifest.Update{
				UpdateWatchTime: bmdeplmanifest.WatchTime{
					Start: 0,
					End:   5478,
				},
			},
			DiskPools: []bmdeplmanifest.DiskPool{
				diskPool,
			},
			Jobs: []bmdeplmanifest.Job{
				{
					Name:               "fake-job-name",
					PersistentDiskPool: "fake-persistent-disk-pool-name",
					Instances:          1,
				},
			},
		}
		registryConfig = bminstallmanifest.Registry{}
		sshTunnelConfig = bminstallmanifest.SSHTunnel{
			User:       "fake-ssh-username",
			PrivateKey: "fake-private-key-path",
			Password:   "fake-password",
			Host:       "fake-ssh-host",
			Port:       124,
		}

		cloud = fakebmcloud.NewFakeCloud()

		mockAgentClientFactory = mock_httpagent.NewMockAgentClientFactory(mockCtrl)
		mockAgentClient = mock_agentclient.NewMockAgentClient(mockCtrl)
		mockAgentClientFactory.EXPECT().NewAgentClient(gomock.Any(), gomock.Any()).Return(mockAgentClient).AnyTimes()

		mockVMManagerFactory = mock_vm.NewMockManagerFactory(mockCtrl)
		fakeVMManager = fakebmvm.NewFakeManager()
		mockVMManagerFactory.EXPECT().NewManager(cloud, mockAgentClient).Return(fakeVMManager).AnyTimes()

		fakeSSHTunnelFactory = fakebmsshtunnel.NewFakeFactory()
		fakeSSHTunnel = fakebmsshtunnel.NewFakeTunnel()
		fakeSSHTunnelFactory.SSHTunnel = fakeSSHTunnel
		fakeSSHTunnel.SetStartBehavior(nil, nil)

		fakeVM = fakebmvm.NewFakeVM("fake-vm-cid")
		fakeVMManager.CreateVM = fakeVM

		fakeVM.AgentClientReturn = mockAgentClient

		logger := boshlog.NewLogger(boshlog.LevelNone)
		eventLogger = fakebmlog.NewFakeEventLogger()
		fakeStage = fakebmlog.NewFakeStage()

		fakeStemcellRepo := fakebmconfig.NewFakeStemcellRepo()
		stemcellRecord := bmconfig.StemcellRecord{
			ID:      "fake-stemcell-id",
			Name:    "fake-stemcell-name",
			Version: "fake-stemcell-version",
			CID:     "fake-stemcell-cid",
		}
		err := fakeStemcellRepo.SetFindBehavior("fake-stemcell-name", "fake-stemcell-version", stemcellRecord, true, nil)
		Expect(err).ToNot(HaveOccurred())

		cloudStemcell = bmstemcell.NewCloudStemcell(stemcellRecord, fakeStemcellRepo, cloud)

		mockStateBuilderFactory = mock_instance_state.NewMockBuilderFactory(mockCtrl)
		mockStateBuilder = mock_instance_state.NewMockBuilder(mockCtrl)
		mockState = mock_instance_state.NewMockState(mockCtrl)

		instanceFactory := bminstance.NewFactory(mockStateBuilderFactory)
		instanceManagerFactory := bminstance.NewManagerFactory(fakeSSHTunnelFactory, instanceFactory, logger)

		mockBlobstore = mock_blobstore.NewMockBlobstore(mockCtrl)

		pingTimeout := 10 * time.Second
		pingDelay := 500 * time.Millisecond
		deploymentFactory := NewFactory(pingTimeout, pingDelay)

		deployer = NewDeployer(
			mockVMManagerFactory,
			instanceManagerFactory,
			deploymentFactory,
			eventLogger,
			logger,
		)
	})

	JustBeforeEach(func() {
		jobName := "fake-job-name"
		jobIndex := 0

		// since we're just passing this from State.ToApplySpec() to VM.Apply(), it doesn't need to be filled out
		applySpec = bmas.ApplySpec{
			Deployment: "fake-deployment-name",
		}

		mockStateBuilderFactory.EXPECT().NewBuilder(mockBlobstore, mockAgentClient).Return(mockStateBuilder).AnyTimes()
		mockStateBuilder.EXPECT().Build(jobName, jobIndex, deploymentManifest, fakeStage).Return(mockState, nil).AnyTimes()
		mockState.EXPECT().ToApplySpec().Return(applySpec).AnyTimes()
	})

	Context("when a previous instance exists", func() {
		var fakeExistingVM *fakebmvm.FakeVM

		BeforeEach(func() {
			fakeExistingVM = fakebmvm.NewFakeVM("existing-vm-cid")
			fakeVMManager.SetFindCurrentBehavior(fakeExistingVM, true, nil)
			fakeExistingVM.AgentClientReturn = mockAgentClient
		})

		It("deletes existing vm", func() {
			_, err := deployer.Deploy(cloud, deploymentManifest, cloudStemcell, registryConfig, sshTunnelConfig, fakeVMManager, mockBlobstore, fakeStage)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeExistingVM.DeleteCalled).To(Equal(1))

			Expect(fakeStage.Steps).To(ContainElement(&fakebmlog.FakeStep{
				Name: "Waiting for the agent on VM 'existing-vm-cid'",
				States: []bmui.EventState{
					bmui.Started,
					bmui.Finished,
				},
			}))
			Expect(fakeStage.Steps).To(ContainElement(&fakebmlog.FakeStep{
				Name: "Deleting VM 'existing-vm-cid'",
				States: []bmui.EventState{
					bmui.Started,
					bmui.Finished,
				},
			}))
		})
	})

	It("creates a vm", func() {
		_, err := deployer.Deploy(cloud, deploymentManifest, cloudStemcell, registryConfig, sshTunnelConfig, fakeVMManager, mockBlobstore, fakeStage)
		Expect(err).NotTo(HaveOccurred())

		Expect(fakeVMManager.CreateInput).To(Equal(fakebmvm.CreateInput{
			Stemcell: cloudStemcell,
			Manifest: deploymentManifest,
		}))
	})

	Context("when registry & ssh tunnel configs are not empty", func() {
		BeforeEach(func() {
			registryConfig = bminstallmanifest.Registry{
				Username: "fake-username",
				Password: "fake-password",
				Host:     "fake-host",
				Port:     123,
			}
			sshTunnelConfig = bminstallmanifest.SSHTunnel{
				User:       "fake-ssh-username",
				PrivateKey: "fake-private-key-path",
				Password:   "fake-password",
				Host:       "fake-ssh-host",
				Port:       124,
			}
		})

		It("starts the SSH tunnel", func() {
			_, err := deployer.Deploy(cloud, deploymentManifest, cloudStemcell, registryConfig, sshTunnelConfig, fakeVMManager, mockBlobstore, fakeStage)
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeSSHTunnel.Started).To(BeTrue())
			Expect(fakeSSHTunnelFactory.NewSSHTunnelOptions).To(Equal(bmsshtunnel.Options{
				User:              "fake-ssh-username",
				PrivateKey:        "fake-private-key-path",
				Password:          "fake-password",
				Host:              "fake-ssh-host",
				Port:              124,
				LocalForwardPort:  123,
				RemoteForwardPort: 123,
			}))
		})

		Context("when starting SSH tunnel fails", func() {
			BeforeEach(func() {
				fakeSSHTunnel.SetStartBehavior(errors.New("fake-ssh-tunnel-start-error"), nil)
			})

			It("returns an error", func() {
				_, err := deployer.Deploy(cloud, deploymentManifest, cloudStemcell, registryConfig, sshTunnelConfig, fakeVMManager, mockBlobstore, fakeStage)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("fake-ssh-tunnel-start-error"))
			})
		})
	})

	It("waits for the vm", func() {
		_, err := deployer.Deploy(cloud, deploymentManifest, cloudStemcell, registryConfig, sshTunnelConfig, fakeVMManager, mockBlobstore, fakeStage)
		Expect(err).NotTo(HaveOccurred())
		Expect(fakeVM.WaitUntilReadyInputs).To(ContainElement(fakebmvm.WaitUntilReadyInput{
			Timeout: 10 * time.Minute,
			Delay:   500 * time.Millisecond,
		}))
	})

	It("logs start and stop events to the eventLogger", func() {
		_, err := deployer.Deploy(cloud, deploymentManifest, cloudStemcell, registryConfig, sshTunnelConfig, fakeVMManager, mockBlobstore, fakeStage)
		Expect(err).NotTo(HaveOccurred())

		Expect(fakeStage.Steps).To(ContainElement(&fakebmlog.FakeStep{
			Name: "Waiting for the agent on VM 'fake-vm-cid' to be ready",
			States: []bmui.EventState{
				bmui.Started,
				bmui.Finished,
			},
		}))
	})

	Context("when waiting for the agent fails", func() {
		BeforeEach(func() {
			fakeVM.WaitUntilReadyErr = errors.New("fake-wait-error")
		})

		It("logs start and stop events to the eventLogger", func() {
			_, err := deployer.Deploy(cloud, deploymentManifest, cloudStemcell, registryConfig, sshTunnelConfig, fakeVMManager, mockBlobstore, fakeStage)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("fake-wait-error"))

			Expect(fakeStage.Steps).To(ContainElement(&fakebmlog.FakeStep{
				Name: "Waiting for the agent on VM 'fake-vm-cid' to be ready",
				States: []bmui.EventState{
					bmui.Started,
					bmui.Failed,
				},
				FailMessage: "fake-wait-error",
			}))
		})
	})

	It("updates the vm", func() {
		_, err := deployer.Deploy(cloud, deploymentManifest, cloudStemcell, registryConfig, sshTunnelConfig, fakeVMManager, mockBlobstore, fakeStage)
		Expect(err).NotTo(HaveOccurred())

		Expect(fakeVM.ApplyInputs).To(Equal([]fakebmvm.ApplyInput{
			{ApplySpec: applySpec},
		}))
	})

	It("starts the agent", func() {
		_, err := deployer.Deploy(cloud, deploymentManifest, cloudStemcell, registryConfig, sshTunnelConfig, fakeVMManager, mockBlobstore, fakeStage)
		Expect(err).NotTo(HaveOccurred())

		Expect(fakeVM.StartCalled).To(Equal(1))
	})

	It("waits until agent reports state as running", func() {
		_, err := deployer.Deploy(cloud, deploymentManifest, cloudStemcell, registryConfig, sshTunnelConfig, fakeVMManager, mockBlobstore, fakeStage)
		Expect(err).NotTo(HaveOccurred())

		Expect(fakeVM.WaitToBeRunningInputs).To(ContainElement(fakebmvm.WaitInput{
			MaxAttempts: 5,
			Delay:       1 * time.Second,
		}))
	})

	Context("when the deployment has an invalid disk pool specification", func() {
		BeforeEach(func() {
			deploymentManifest.Jobs[0].PersistentDiskPool = "fake-non-existent-persistent-disk-pool-name"
		})

		It("returns an error", func() {
			_, err := deployer.Deploy(cloud, deploymentManifest, cloudStemcell, registryConfig, sshTunnelConfig, fakeVMManager, mockBlobstore, fakeStage)
			Expect(err).To(HaveOccurred())
		})
	})

	It("logs start and stop events to the eventLogger", func() {
		_, err := deployer.Deploy(cloud, deploymentManifest, cloudStemcell, registryConfig, sshTunnelConfig, fakeVMManager, mockBlobstore, fakeStage)
		Expect(err).NotTo(HaveOccurred())

		Expect(fakeStage.Steps).To(ContainElement(&fakebmlog.FakeStep{
			Name: "Updating instance 'fake-job-name/0'",
			States: []bmui.EventState{
				bmui.Started,
				bmui.Finished,
			},
		}))
		Expect(fakeStage.Steps).To(ContainElement(&fakebmlog.FakeStep{
			Name: "Waiting for instance 'fake-job-name/0' to be running",
			States: []bmui.EventState{
				bmui.Started,
				bmui.Finished,
			},
		}))
	})

	Context("when updating instance fails", func() {
		BeforeEach(func() {
			fakeVM.ApplyErr = errors.New("fake-apply-error")
		})

		It("logs start and stop events to the eventLogger", func() {
			_, err := deployer.Deploy(cloud, deploymentManifest, cloudStemcell, registryConfig, sshTunnelConfig, fakeVMManager, mockBlobstore, fakeStage)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("fake-apply-error"))

			Expect(fakeStage.Steps).To(ContainElement(&fakebmlog.FakeStep{
				Name: "Updating instance 'fake-job-name/0'",
				States: []bmui.EventState{
					bmui.Started,
					bmui.Failed,
				},
				FailMessage: "Applying the agent state: fake-apply-error",
			}))
		})
	})

	Context("when starting agent services fails", func() {
		BeforeEach(func() {
			fakeVM.StartErr = errors.New("fake-start-error")
		})

		It("logs start and stop events to the eventLogger", func() {
			_, err := deployer.Deploy(cloud, deploymentManifest, cloudStemcell, registryConfig, sshTunnelConfig, fakeVMManager, mockBlobstore, fakeStage)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("fake-start-error"))

			Expect(fakeStage.Steps).To(ContainElement(&fakebmlog.FakeStep{
				Name: "Updating instance 'fake-job-name/0'",
				States: []bmui.EventState{
					bmui.Started,
					bmui.Failed,
				},
				FailMessage: "Starting the agent: fake-start-error",
			}))
		})
	})

	Context("when waiting for running state fails", func() {
		BeforeEach(func() {
			fakeVM.WaitToBeRunningErr = errors.New("fake-wait-running-error")
		})

		It("logs start and stop events to the eventLogger", func() {
			_, err := deployer.Deploy(cloud, deploymentManifest, cloudStemcell, registryConfig, sshTunnelConfig, fakeVMManager, mockBlobstore, fakeStage)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("fake-wait-running-error"))

			Expect(fakeStage.Steps).To(ContainElement(&fakebmlog.FakeStep{
				Name: "Waiting for instance 'fake-job-name/0' to be running",
				States: []bmui.EventState{
					bmui.Started,
					bmui.Failed,
				},
				FailMessage: "fake-wait-running-error",
			}))
		})
	})
})
