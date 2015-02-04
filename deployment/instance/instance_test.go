package instance_test

import (
	. "github.com/cloudfoundry/bosh-micro-cli/deployment/instance"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"errors"
	"time"

	"code.google.com/p/gomock/gomock"
	mock_instance_state "github.com/cloudfoundry/bosh-micro-cli/deployment/instance/state/mocks"

	boshlog "github.com/cloudfoundry/bosh-agent/logger"

	bmcloud "github.com/cloudfoundry/bosh-micro-cli/cloud"
	bmas "github.com/cloudfoundry/bosh-micro-cli/deployment/applyspec"
	bmdisk "github.com/cloudfoundry/bosh-micro-cli/deployment/disk"
	bmdeplmanifest "github.com/cloudfoundry/bosh-micro-cli/deployment/manifest"
	bmsshtunnel "github.com/cloudfoundry/bosh-micro-cli/deployment/sshtunnel"
	bminstallmanifest "github.com/cloudfoundry/bosh-micro-cli/installation/manifest"
	bmui "github.com/cloudfoundry/bosh-micro-cli/ui"

	fakebmdisk "github.com/cloudfoundry/bosh-micro-cli/deployment/disk/fakes"
	fakebmsshtunnel "github.com/cloudfoundry/bosh-micro-cli/deployment/sshtunnel/fakes"
	fakebmvm "github.com/cloudfoundry/bosh-micro-cli/deployment/vm/fakes"
	fakebmlog "github.com/cloudfoundry/bosh-micro-cli/eventlogger/fakes"
)

var _ = Describe("Instance", func() {
	var mockCtrl *gomock.Controller

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	var (
		mockStateBuilder *mock_instance_state.MockBuilder
		mockState        *mock_instance_state.MockState

		fakeVMManager        *fakebmvm.FakeManager
		fakeVM               *fakebmvm.FakeVM
		fakeSSHTunnelFactory *fakebmsshtunnel.FakeFactory
		fakeSSHTunnel        *fakebmsshtunnel.FakeTunnel
		fakeStage            *fakebmlog.FakeStage

		instance Instance

		pingTimeout = 1 * time.Second
		pingDelay   = 500 * time.Millisecond

		jobName  = "fake-job-name"
		jobIndex = 0
	)

	BeforeEach(func() {
		fakeVMManager = fakebmvm.NewFakeManager()
		fakeVM = fakebmvm.NewFakeVM("fake-vm-cid")

		fakeSSHTunnelFactory = fakebmsshtunnel.NewFakeFactory()
		fakeSSHTunnel = fakebmsshtunnel.NewFakeTunnel()
		fakeSSHTunnel.SetStartBehavior(nil, nil)
		fakeSSHTunnelFactory.SSHTunnel = fakeSSHTunnel

		mockStateBuilder = mock_instance_state.NewMockBuilder(mockCtrl)
		mockState = mock_instance_state.NewMockState(mockCtrl)

		logger := boshlog.NewLogger(boshlog.LevelNone)

		instance = NewInstance(
			jobName,
			jobIndex,
			fakeVM,
			fakeVMManager,
			fakeSSHTunnelFactory,
			mockStateBuilder,
			logger,
		)

		fakeStage = fakebmlog.NewFakeStage()
	})

	Describe("Delete", func() {
		It("checks if the agent on the vm is responsive", func() {
			err := instance.Delete(pingTimeout, pingDelay, fakeStage)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeVM.WaitUntilReadyInputs).To(ContainElement(fakebmvm.WaitUntilReadyInput{
				Timeout: pingTimeout,
				Delay:   pingDelay,
			}))
		})

		It("deletes existing vm", func() {
			err := instance.Delete(pingTimeout, pingDelay, fakeStage)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeVM.DeleteCalled).To(Equal(1))
		})

		It("logs start and stop events", func() {
			err := instance.Delete(pingTimeout, pingDelay, fakeStage)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeStage.Steps).To(Equal([]*fakebmlog.FakeStep{
				{
					Name: "Waiting for the agent on VM 'fake-vm-cid'",
					States: []bmui.EventState{
						bmui.Started,
						bmui.Finished,
					},
				},
				{
					Name: "Stopping jobs on instance 'fake-job-name/0'",
					States: []bmui.EventState{
						bmui.Started,
						bmui.Finished,
					},
				},
				{
					Name: "Deleting VM 'fake-vm-cid'",
					States: []bmui.EventState{
						bmui.Started,
						bmui.Finished,
					},
				},
			}))
		})

		Context("when agent is responsive", func() {
			It("logs waiting for the agent event", func() {
				err := instance.Delete(pingTimeout, pingDelay, fakeStage)
				Expect(err).ToNot(HaveOccurred())

				Expect(fakeStage.Steps).To(ContainElement(&fakebmlog.FakeStep{
					Name: "Waiting for the agent on VM 'fake-vm-cid'",
					States: []bmui.EventState{
						bmui.Started,
						bmui.Finished,
					},
				}))
			})

			It("stops vm", func() {
				err := instance.Delete(pingTimeout, pingDelay, fakeStage)
				Expect(err).ToNot(HaveOccurred())

				Expect(fakeVM.StopCalled).To(Equal(1))
			})

			It("unmounts vm disks", func() {
				firstDisk := fakebmdisk.NewFakeDisk("fake-disk-1")
				secondDisk := fakebmdisk.NewFakeDisk("fake-disk-2")
				fakeVM.ListDisksDisks = []bmdisk.Disk{firstDisk, secondDisk}

				err := instance.Delete(pingTimeout, pingDelay, fakeStage)
				Expect(err).ToNot(HaveOccurred())

				Expect(fakeVM.UnmountDiskInputs).To(Equal([]fakebmvm.UnmountDiskInput{
					{Disk: firstDisk},
					{Disk: secondDisk},
				}))

				Expect(fakeStage.Steps).To(ContainElement(&fakebmlog.FakeStep{
					Name: "Unmounting disk 'fake-disk-1'",
					States: []bmui.EventState{
						bmui.Started,
						bmui.Finished,
					},
				}))
				Expect(fakeStage.Steps).To(ContainElement(&fakebmlog.FakeStep{
					Name: "Unmounting disk 'fake-disk-2'",
					States: []bmui.EventState{
						bmui.Started,
						bmui.Finished,
					},
				}))
			})

			Context("when stopping vm fails", func() {
				BeforeEach(func() {
					fakeVM.StopErr = errors.New("fake-stop-error")
				})

				It("returns an error", func() {
					err := instance.Delete(pingTimeout, pingDelay, fakeStage)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("fake-stop-error"))

					Expect(fakeStage.Steps).To(ContainElement(&fakebmlog.FakeStep{
						Name: "Stopping jobs on instance 'fake-job-name/0'",
						States: []bmui.EventState{
							bmui.Started,
							bmui.Failed,
						},
						FailMessage: "fake-stop-error",
					}))
				})
			})

			Context("when unmounting disk fails", func() {
				BeforeEach(func() {
					fakeVM.ListDisksDisks = []bmdisk.Disk{fakebmdisk.NewFakeDisk("fake-disk")}
					fakeVM.UnmountDiskErr = errors.New("fake-unmount-error")
				})

				It("returns an error", func() {
					err := instance.Delete(pingTimeout, pingDelay, fakeStage)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("fake-unmount-error"))

					Expect(fakeStage.Steps).To(ContainElement(&fakebmlog.FakeStep{
						Name: "Unmounting disk 'fake-disk'",
						States: []bmui.EventState{
							bmui.Started,
							bmui.Failed,
						},
						FailMessage: "Unmounting disk 'fake-disk' from VM 'fake-vm-cid': fake-unmount-error",
					}))
				})
			})
		})

		Context("when agent fails to respond", func() {
			BeforeEach(func() {
				fakeVM.WaitUntilReadyErr = errors.New("fake-wait-error")
			})

			It("logs failed event", func() {
				err := instance.Delete(pingTimeout, pingDelay, fakeStage)
				Expect(err).ToNot(HaveOccurred())

				Expect(fakeStage.Steps).To(ContainElement(&fakebmlog.FakeStep{
					Name: "Waiting for the agent on VM 'fake-vm-cid'",
					States: []bmui.EventState{
						bmui.Started,
						bmui.Failed,
					},
					FailMessage: "Agent unreachable: fake-wait-error",
				}))
			})
		})

		Context("when deleting VM fails", func() {
			BeforeEach(func() {
				fakeVM.DeleteErr = errors.New("fake-delete-error")
			})

			It("returns an error", func() {
				err := instance.Delete(pingTimeout, pingDelay, fakeStage)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("fake-delete-error"))

				Expect(fakeStage.Steps).To(ContainElement(&fakebmlog.FakeStep{
					Name: "Deleting VM 'fake-vm-cid'",
					States: []bmui.EventState{
						bmui.Started,
						bmui.Failed,
					},
					FailMessage: "fake-delete-error",
				}))
			})
		})

		Context("when VM does not exist (deleted manually)", func() {
			BeforeEach(func() {
				fakeVM.ExistsFound = false
				fakeVM.DeleteErr = bmcloud.NewCPIError("delete_vm", bmcloud.CmdError{
					Type:    bmcloud.VMNotFoundError,
					Message: "fake-vm-not-found-message",
				})
			})

			It("deletes existing vm", func() {
				err := instance.Delete(pingTimeout, pingDelay, fakeStage)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeVM.DeleteCalled).To(Equal(1))
			})

			It("does not contact the agent", func() {
				err := instance.Delete(pingTimeout, pingDelay, fakeStage)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeVM.WaitUntilReadyInputs).To(HaveLen(0))
				Expect(fakeVM.StopCalled).To(Equal(0))
				Expect(fakeVM.UnmountDiskInputs).To(HaveLen(0))
			})

			It("logs vm delete as skipped", func() {
				err := instance.Delete(pingTimeout, pingDelay, fakeStage)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeStage.Steps).To(Equal([]*fakebmlog.FakeStep{
					{
						Name: "Deleting VM 'fake-vm-cid'",
						States: []bmui.EventState{
							bmui.Started,
							bmui.Skipped,
						},
						SkipMessage: "CPI 'delete_vm' method responded with error: CmdError{\"type\":\"Bosh::Cloud::VMNotFound\",\"message\":\"fake-vm-not-found-message\",\"ok_to_retry\":false}",
					},
				}))
			})
		})
	})

	Describe("UpdateJobs", func() {
		var (
			deploymentManifest bmdeplmanifest.Manifest

			applySpec bmas.ApplySpec

			expectStateBuild *gomock.Call
		)

		BeforeEach(func() {
			// manifest is only being used for the Update.UpdateWatchTime, otherwise it's just being passed through to the StateBuilder
			deploymentManifest = bmdeplmanifest.Manifest{
				Name: "fake-deployment-name",
				Update: bmdeplmanifest.Update{
					UpdateWatchTime: bmdeplmanifest.WatchTime{
						Start: 0,
						End:   5478,
					},
				},
			}

			// apply spec is just returned from instance.State.ToApplySpec() and passed to agentClient.Apply()
			applySpec = bmas.ApplySpec{
				Deployment: "fake-deployment-name",
			}
		})

		JustBeforeEach(func() {
			expectStateBuild = mockStateBuilder.EXPECT().Build(jobName, jobIndex, deploymentManifest, fakeStage).Return(mockState, nil).AnyTimes()
			mockState.EXPECT().ToApplySpec().Return(applySpec).AnyTimes()
		})

		It("builds a new instance state", func() {
			expectStateBuild.Times(1)

			err := instance.UpdateJobs(deploymentManifest, fakeStage)
			Expect(err).ToNot(HaveOccurred())
		})

		It("tells agent to stop jobs, apply a new spec (with new rendered jobs templates), and start jobs", func() {
			err := instance.UpdateJobs(deploymentManifest, fakeStage)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeVM.StopCalled).To(Equal(1))
			Expect(fakeVM.ApplyInputs).To(Equal([]fakebmvm.ApplyInput{
				{ApplySpec: applySpec},
			}))
			Expect(fakeVM.StartCalled).To(Equal(1))
		})

		It("waits until agent reports state as running", func() {
			err := instance.UpdateJobs(deploymentManifest, fakeStage)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeVM.WaitToBeRunningInputs).To(ContainElement(fakebmvm.WaitInput{
				MaxAttempts: 5,
				Delay:       1 * time.Second,
			}))
		})

		It("logs start and stop events to the eventLogger", func() {
			err := instance.UpdateJobs(deploymentManifest, fakeStage)
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

		Context("when instance state building fails", func() {
			JustBeforeEach(func() {
				expectStateBuild.Return(nil, errors.New("fake-template-err")).Times(1)
			})

			It("returns an error", func() {
				err := instance.UpdateJobs(deploymentManifest, fakeStage)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("fake-template-err"))
			})
		})

		Context("when stopping vm fails", func() {
			BeforeEach(func() {
				fakeVM.StopErr = errors.New("fake-stop-error")
			})

			It("logs start and stop events to the eventLogger", func() {
				err := instance.UpdateJobs(deploymentManifest, fakeStage)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("fake-stop-error"))

				Expect(fakeStage.Steps).To(ContainElement(&fakebmlog.FakeStep{
					Name: "Updating instance 'fake-job-name/0'",
					States: []bmui.EventState{
						bmui.Started,
						bmui.Failed,
					},
					FailMessage: "Stopping the agent: fake-stop-error",
				}))
			})
		})

		Context("when applying a new vm state fails", func() {
			BeforeEach(func() {
				fakeVM.ApplyErr = errors.New("fake-apply-error")
			})

			It("logs start and stop events to the eventLogger", func() {
				err := instance.UpdateJobs(deploymentManifest, fakeStage)
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

		Context("when starting vm fails", func() {
			BeforeEach(func() {
				fakeVM.StartErr = errors.New("fake-start-error")
			})

			It("logs start and stop events to the eventLogger", func() {
				err := instance.UpdateJobs(deploymentManifest, fakeStage)
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
				err := instance.UpdateJobs(deploymentManifest, fakeStage)
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

	Describe("WaitUntilReady", func() {
		var (
			registryConfig  bminstallmanifest.Registry
			sshTunnelConfig bminstallmanifest.SSHTunnel
		)

		BeforeEach(func() {
			registryConfig = bminstallmanifest.Registry{
				Port: 125,
			}
			sshTunnelConfig = bminstallmanifest.SSHTunnel{
				Host:       "fake-ssh-host",
				Port:       124,
				User:       "fake-ssh-username",
				Password:   "fake-password",
				PrivateKey: "fake-private-key-path",
			}
		})

		It("starts & stops the SSH tunnel", func() {
			err := instance.WaitUntilReady(registryConfig, sshTunnelConfig, fakeStage)
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeSSHTunnelFactory.NewSSHTunnelOptions).To(Equal(bmsshtunnel.Options{
				User:              "fake-ssh-username",
				PrivateKey:        "fake-private-key-path",
				Password:          "fake-password",
				Host:              "fake-ssh-host",
				Port:              124,
				LocalForwardPort:  125,
				RemoteForwardPort: 125,
			}))
			Expect(fakeSSHTunnel.Started).To(BeTrue())
			Expect(fakeSSHTunnel.Stopped).To(BeTrue())
		})

		It("waits for the vm", func() {
			err := instance.WaitUntilReady(registryConfig, sshTunnelConfig, fakeStage)
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeVM.WaitUntilReadyInputs).To(ContainElement(fakebmvm.WaitUntilReadyInput{
				Timeout: 10 * time.Minute,
				Delay:   500 * time.Millisecond,
			}))
		})

		It("logs start and stop events to the eventLogger", func() {
			err := instance.WaitUntilReady(registryConfig, sshTunnelConfig, fakeStage)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeStage.Steps).To(ContainElement(&fakebmlog.FakeStep{
				Name: "Waiting for the agent on VM 'fake-vm-cid' to be ready",
				States: []bmui.EventState{
					bmui.Started,
					bmui.Finished,
				},
			}))
		})

		Context("when ssh tunnel config is empty", func() {
			BeforeEach(func() {
				sshTunnelConfig = bminstallmanifest.SSHTunnel{}
			})

			It("does not start ssh tunnel", func() {
				err := instance.WaitUntilReady(registryConfig, sshTunnelConfig, fakeStage)
				Expect(err).ToNot(HaveOccurred())
				Expect(fakeSSHTunnel.Started).To(BeFalse())
			})
		})

		Context("when registry config is empty", func() {
			BeforeEach(func() {
				registryConfig = bminstallmanifest.Registry{}
			})

			It("does not start ssh tunnel", func() {
				err := instance.WaitUntilReady(registryConfig, sshTunnelConfig, fakeStage)
				Expect(err).ToNot(HaveOccurred())
				Expect(fakeSSHTunnel.Started).To(BeFalse())
			})
		})

		Context("when starting SSH tunnel fails", func() {
			BeforeEach(func() {
				fakeSSHTunnel.SetStartBehavior(errors.New("fake-ssh-tunnel-start-error"), nil)
			})

			It("returns an error", func() {
				err := instance.WaitUntilReady(registryConfig, sshTunnelConfig, fakeStage)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("fake-ssh-tunnel-start-error"))
			})
		})

		Context("when waiting for the agent fails", func() {
			BeforeEach(func() {
				fakeVM.WaitUntilReadyErr = errors.New("fake-wait-error")
			})

			It("logs start and stop events to the eventLogger", func() {
				err := instance.WaitUntilReady(registryConfig, sshTunnelConfig, fakeStage)
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
	})
})
