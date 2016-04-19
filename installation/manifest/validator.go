package manifest

import (
	"net"
	"net/url"
	"strings"

	bosherr "github.com/cloudfoundry/bosh-init/internal/github.com/cloudfoundry/bosh-utils/errors"
	boshlog "github.com/cloudfoundry/bosh-init/internal/github.com/cloudfoundry/bosh-utils/logger"
	biproperty "github.com/cloudfoundry/bosh-init/internal/github.com/cloudfoundry/bosh-utils/property"
	birelsetmanifest "github.com/cloudfoundry/bosh-init/release/set/manifest"
)

type Validator interface {
	Validate(Manifest, birelsetmanifest.Manifest) error
}

type validator struct {
	logger boshlog.Logger
}

func NewValidator(logger boshlog.Logger) Validator {
	return &validator{
		logger: logger,
	}
}

func (v *validator) Validate(manifest Manifest, releaseSetManifest birelsetmanifest.Manifest) error {
	errs := []error{}

	cpiJobName := manifest.Template.Name
	if v.isBlank(cpiJobName) {
		errs = append(errs, bosherr.Error("cloud_provider.template.name must be provided"))
	}

	cpiReleaseName := manifest.Template.Release
	if v.isBlank(cpiReleaseName) {
		errs = append(errs, bosherr.Error("cloud_provider.template.release must be provided"))
	}

	_, found := releaseSetManifest.FindByName(cpiReleaseName)
	if !found {
		errs = append(errs, bosherr.Errorf("cloud_provider.template.release '%s' must refer to a release in releases", cpiReleaseName))
	}

	mbusURLString := manifest.Mbus
	if v.isBlank(mbusURLString) {
		errs = append(errs, bosherr.Error("cloud_provider.mbus must be provided"))
	}

	agentProperties, found := manifest.Properties["agent"]
	if !found {
		errs = append(errs, bosherr.Error("cloud_provider.properties.agent must be specified"))
	}

	agentPropertiesMap, ok := agentProperties.(biproperty.Map)
	if !ok {
		errs = append(errs, bosherr.Error("cloud_provider.properties.agent must be a hash"))
	}

	agentMbusURLProperty, found := agentPropertiesMap["mbus"]
	if !found {
		errs = append(errs, bosherr.Error("cloud_provider.properties.agent.mbus must be specified"))
	}

	agentMbusURLString, ok := agentMbusURLProperty.(string)
	if !ok {
		errs = append(errs, bosherr.Error("cloud_provider.properties.agent.mbus should be string"))
	}

	if !v.isBlank(mbusURLString) && !v.isBlank(agentMbusURLString) {
		mbusURL, mbusURLParseErr := url.ParseRequestURI(mbusURLString)
		if mbusURLParseErr != nil {
			errs = append(errs, bosherr.Error("cloud_provider.mbus should be a valid URL"))
		}

		agentMbusURL, agentMbusURLParseErr := url.ParseRequestURI(agentMbusURLString)
		if agentMbusURLParseErr != nil {
			errs = append(errs, bosherr.Error("cloud_provider.properties.agent.mbus should be a valid URL"))
		}

		if (agentMbusURLParseErr == nil) && (mbusURLParseErr == nil) {
			if mbusURL.Scheme != "https" {
				errs = append(errs, bosherr.Error("cloud_provider.mbus must use https protocol"))
			}

			if agentMbusURL.Scheme != "https" {
				errs = append(errs, bosherr.Error("cloud_provider.properties.agent.mbus must use https protocol"))
			}

			if (mbusURL.User != nil) && (agentMbusURL.User != nil) {
				mbusCredsAreEqual := (agentMbusURL.User.String() == mbusURL.User.String())
				if !mbusCredsAreEqual {
					errs = append(errs, bosherr.Error("cloud_provider.properties.agent.mbus and cloud_provider.mbus should have the same password and username"))
				}
			}

			_, mbusPort, _ := net.SplitHostPort(mbusURL.Host)
			_, agentMbusPort, _ := net.SplitHostPort(agentMbusURL.Host)
			if mbusPort != agentMbusPort {
				errs = append(errs, bosherr.Error("cloud_provider.properties.agent.mbus and cloud_provider.mbus should have the same ports"))
			}

		}

	}

	if len(errs) > 0 {
		return bosherr.NewMultiError(errs...)
	}

	return nil
}

func (v *validator) isBlank(str string) bool {
	return str == "" || strings.TrimSpace(str) == ""
}
