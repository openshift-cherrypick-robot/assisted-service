package ignition

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/coreos/ignition/v2/config/merge"
	config_31 "github.com/coreos/ignition/v2/config/v3_1"
	config_latest "github.com/coreos/ignition/v2/config/v3_2"
	config_latest_trans "github.com/coreos/ignition/v2/config/v3_2/translate"
	config_latest_types "github.com/coreos/ignition/v2/config/v3_2/types"
	"github.com/coreos/vcontext/report"
	"github.com/go-openapi/swag"
	bmh_v1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	operatorv1alpha1 "github.com/openshift/api/operator/v1alpha1"
	clusterPkg "github.com/openshift/assisted-service/internal/cluster"
	"github.com/openshift/assisted-service/internal/common"
	"github.com/openshift/assisted-service/internal/constants"
	"github.com/openshift/assisted-service/internal/host/hostutil"
	"github.com/openshift/assisted-service/internal/installcfg"
	"github.com/openshift/assisted-service/internal/installercache"
	"github.com/openshift/assisted-service/internal/manifests"
	"github.com/openshift/assisted-service/internal/network"
	"github.com/openshift/assisted-service/internal/operators"
	"github.com/openshift/assisted-service/internal/provider/registry"
	"github.com/openshift/assisted-service/models"
	"github.com/openshift/assisted-service/pkg/auth"
	logutil "github.com/openshift/assisted-service/pkg/log"
	"github.com/openshift/assisted-service/pkg/mirrorregistries"
	"github.com/openshift/assisted-service/pkg/s3wrapper"
	"github.com/openshift/assisted-service/pkg/staticnetworkconfig"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/thoas/go-funk"
	"github.com/vincent-petithory/dataurl"
	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sjson "k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/client-go/kubernetes/scheme"
	k8syaml "sigs.k8s.io/yaml"
)

const (
	masterIgn = "master.ign"
	workerIgn = "worker.ign"
)

const agentMessageOfTheDay = `
**  **  **  **  **  **  **  **  **  **  **  **  **  **  **  **  **  ** **  **  **  **  **  **  **
This is a host being installed by the OpenShift Assisted Installer.
It will be installed from scratch during the installation.

The primary service is agent.service. To watch its status, run:
sudo journalctl -u agent.service

To view the agent log, run:
sudo journalctl TAG=agent
**  **  **  **  **  **  **  **  **  **  **  **  **  **  **  **  **  ** **  **  **  **  **  **  **
`

const RedhatRootCA = `
-----BEGIN CERTIFICATE-----
MIIENDCCAxygAwIBAgIJANunI0D662cnMA0GCSqGSIb3DQEBCwUAMIGlMQswCQYD
VQQGEwJVUzEXMBUGA1UECAwOTm9ydGggQ2Fyb2xpbmExEDAOBgNVBAcMB1JhbGVp
Z2gxFjAUBgNVBAoMDVJlZCBIYXQsIEluYy4xEzARBgNVBAsMClJlZCBIYXQgSVQx
GzAZBgNVBAMMElJlZCBIYXQgSVQgUm9vdCBDQTEhMB8GCSqGSIb3DQEJARYSaW5m
b3NlY0ByZWRoYXQuY29tMCAXDTE1MDcwNjE3MzgxMVoYDzIwNTUwNjI2MTczODEx
WjCBpTELMAkGA1UEBhMCVVMxFzAVBgNVBAgMDk5vcnRoIENhcm9saW5hMRAwDgYD
VQQHDAdSYWxlaWdoMRYwFAYDVQQKDA1SZWQgSGF0LCBJbmMuMRMwEQYDVQQLDApS
ZWQgSGF0IElUMRswGQYDVQQDDBJSZWQgSGF0IElUIFJvb3QgQ0ExITAfBgkqhkiG
9w0BCQEWEmluZm9zZWNAcmVkaGF0LmNvbTCCASIwDQYJKoZIhvcNAQEBBQADggEP
ADCCAQoCggEBALQt9OJQh6GC5LT1g80qNh0u50BQ4sZ/yZ8aETxt+5lnPVX6MHKz
bfwI6nO1aMG6j9bSw+6UUyPBHP796+FT/pTS+K0wsDV7c9XvHoxJBJJU38cdLkI2
c/i7lDqTfTcfLL2nyUBd2fQDk1B0fxrskhGIIZ3ifP1Ps4ltTkv8hRSob3VtNqSo
GxkKfvD2PKjTPxDPWYyruy9irLZioMffi3i/gCut0ZWtAyO3MVH5qWF/enKwgPES
X9po+TdCvRB/RUObBaM761EcrLSM1GqHNueSfqnho3AjLQ6dBnPWlo638Zm1VebK
BELyhkLWMSFkKwDmne0jQ02Y4g075vCKvCsCAwEAAaNjMGEwHQYDVR0OBBYEFH7R
4yC+UehIIPeuL8Zqw3PzbgcZMB8GA1UdIwQYMBaAFH7R4yC+UehIIPeuL8Zqw3Pz
bgcZMA8GA1UdEwEB/wQFMAMBAf8wDgYDVR0PAQH/BAQDAgGGMA0GCSqGSIb3DQEB
CwUAA4IBAQBDNvD2Vm9sA5A9AlOJR8+en5Xz9hXcxJB5phxcZQ8jFoG04Vshvd0e
LEnUrMcfFgIZ4njMKTQCM4ZFUPAieyLx4f52HuDopp3e5JyIMfW+KFcNIpKwCsak
oSoKtIUOsUJK7qBVZxcrIyeQV2qcYOeZhtS5wBqIwOAhFwlCET7Ze58QHmS48slj
S9K0JAcps2xdnGu0fkzhSQxY8GPQNFTlr6rYld5+ID/hHeS76gq0YG3q6RLWRkHf
4eTkRjivAlExrFzKcljC4axKQlnOvVAzz+Gm32U0xPBF4ByePVxCJUHw1TsyTmel
RxNEp7yHoXcwn+fXna+t5JWh1gxUZty3
-----END CERTIFICATE-----`

const selinuxPolicy = `
module assisted 1.0;
require {
        type chronyd_t;
        type container_file_t;
        type spc_t;
        class unix_dgram_socket sendto;
        class dir search;
        class sock_file write;
}
#============= chronyd_t ==============
allow chronyd_t container_file_t:dir search;
allow chronyd_t container_file_t:sock_file write;
allow chronyd_t spc_t:unix_dgram_socket sendto;
`

const agentFixBZ1964591 = `#!/usr/bin/sh

# This script is a workaround for bugzilla 1964591 where symlinks inside /var/lib/containers/ get
# corrupted under some circumstances.
#
# In order to let agent.service start correctly we are checking here whether the requested
# container image exists and in case "podman images" returns an error we try removing the faulty
# image.
#
# In such a scenario agent.service will detect the image is not present and pull it again. In case
# the image is present and can be detected correctly, no any action is required.

IMAGE=$(echo $1 | sed 's/:.*//')
podman images | grep $IMAGE || podman rmi --force $1 || true
`

const okdBinariesOverlayTemplate = `#!/bin/env bash
set -eux
# Fetch an image with OKD rpms
RPMS_IMAGE="%s"
while ! podman pull --quiet "${RPMS_IMAGE}"
do
    echo "Pull failed. Retrying ${RPMS_IMAGE}..."
    sleep 5
done
mnt=$(podman image mount "${RPMS_IMAGE}")
# Extract machine-config-daemon binary
cp -rvf ${mnt}/binaries/machine-config-daemon /usr/local/bin/machine-config-daemon
chmod a+x /usr/local/bin/machine-config-daemon
restorecon -Rv /usr/local/bin/machine-config-daemon
# Install RPMs in overlayed FS
mkdir /tmp/rpms
cp -rvf ${mnt}/rpms/* /tmp/rpms
tmpd=$(mktemp -d)
mkdir ${tmpd}/{upper,work}
mount -t overlay -o lowerdir=/usr,upperdir=${tmpd}/upper,workdir=${tmpd}/work overlay /usr
rpm -Uvh /tmp/rpms/*
podman rmi -f "${RPMS_IMAGE}"
# Expand /var to 6G if necessary
if (( $(stat -c%%s /run/ephemeral.xfsloop) > 6*1024*1024*1024 )); then
  exit 0
fi
/bin/truncate -s 6G /run/ephemeral.xfsloop
losetup -c /dev/loop0
xfs_growfs /var
mount -o remount,size=6G /run
`

const okdHoldAgentUntilBinariesLanded = `[Unit]
Wants=okd-overlay.service
After=okd-overlay.service
`

const okdHoldPivot = `[Unit]
ConditionPathExists=/enoent
`

const discoveryIgnitionConfigFormat = `{
  "ignition": {
    "version": "3.1.0"{{if .PROXY_SETTINGS}},
    {{.PROXY_SETTINGS}}{{end}}
  },
  "passwd": {
    "users": [
      {{.userSshKey}}
    ]
  },
  "systemd": {
    "units": [{
      "name": "agent.service",
      "enabled": {{if .EnableAgentService}}true{{else}}false{{end}},
      "contents": "[Service]\nType=simple\nRestart=always\nRestartSec=3\nStartLimitInterval=0\nEnvironment=HTTP_PROXY={{.HTTPProxy}}\nEnvironment=http_proxy={{.HTTPProxy}}\nEnvironment=HTTPS_PROXY={{.HTTPSProxy}}\nEnvironment=https_proxy={{.HTTPSProxy}}\nEnvironment=NO_PROXY={{.NoProxy}}\nEnvironment=no_proxy={{.NoProxy}}{{if .PullSecretToken}}\nEnvironment=PULL_SECRET_TOKEN={{.PullSecretToken}}{{end}}\nTimeoutStartSec={{.AgentTimeoutStartSec}}\nExecStartPre=/usr/local/bin/agent-fix-bz1964591 {{.AgentDockerImg}}\nExecStartPre=podman run --privileged --rm -v /usr/local/bin:/hostbin {{.AgentDockerImg}} cp /usr/bin/agent /hostbin\nExecStart=/usr/local/bin/agent --url {{.ServiceBaseURL}} --infra-env-id {{.infraEnvId}} --agent-version {{.AgentDockerImg}} --insecure={{.SkipCertVerification}}  {{if .HostCACertPath}}--cacert {{.HostCACertPath}}{{end}}\n\n[Unit]\nWants=network-online.target\nAfter=network-online.target\n\n[Install]\nWantedBy=multi-user.target"
    },
    {
        "name": "selinux.service",
        "enabled": true,
        "contents": "[Service]\nType=oneshot\nExecStartPre=checkmodule -M -m -o /root/assisted.mod /root/assisted.te\nExecStartPre=semodule_package -o /root/assisted.pp -m /root/assisted.mod\nExecStart=semodule -i /root/assisted.pp\n\n[Install]\nWantedBy=multi-user.target"
    }{{if .StaticNetworkConfig}},
    {
        "name": "pre-network-manager-config.service",
        "enabled": true,
        "contents": "[Unit]\nDescription=Prepare network manager config content\nBefore=dracut-initqueue.service\nAfter=dracut-cmdline.service\nDefaultDependencies=no\n[Service]\nUser=root\nType=oneshot\nTimeoutSec=60\nExecStart=/bin/bash /usr/local/bin/pre-network-manager-config.sh\nPrivateTmp=true\nRemainAfterExit=no\n[Install]\nWantedBy=multi-user.target"
    }{{end}}{{if .OKDBinaries}},
    {
        "name": "okd-overlay.service",
        "enabled": true,
        "contents": "[Service]\nType=oneshot\nExecStart=/usr/local/bin/okd-binaries.sh\n\n[Unit]\nWants=network-online.target\nAfter=network-online.target\n\n[Install]\nWantedBy=multi-user.target"
    },
	{
        "name": "multipathd.service",
        "enabled": true,
    },
    {
        "name": "systemd-journal-gatewayd.socket",
        "enabled": true,
        "contents": "[Unit]\nDescription = Fake systemd-journal-gatewayd.socket\n\n[Socket]\nListenStream = 19531\nAccept = yes\n\n[Install]\nWantedBy = sockets.target"
		}{{end}}
    ]
  },
  "storage": {
    "files": [{
      "overwrite": true,
      "path": "/usr/local/bin/agent-fix-bz1964591",
      "mode": 755,
      "user": {
          "name": "root"
      },
      "contents": { "source": "data:,{{.AGENT_FIX_BZ1964591}}" }
    },
    {
      "overwrite": true,
      "path": "/etc/motd",
      "mode": 420,
      "user": {
          "name": "root"
      },
      "contents": { "source": "data:,{{.AGENT_MOTD}}" }
    },
    {
		"overwrite": true,
		"path": "/etc/multipath.conf",
		"mode": 420,
		"user": {
			"name": "root"
		},
		"contents": { "source": "data:text/plain;charset=utf-8;base64,ZGVmYXVsdHMgewogICAgdXNlcl9mcmllbmRseV9uYW1lcyB5ZXMKICAgIGZpbmRfbXVsdGlwYXRocyB5ZXMKICAgIGVuYWJsZV9mb3JlaWduICJeJCIKfQpibGFja2xpc3RfZXhjZXB0aW9ucyB7CiAgICBwcm9wZXJ0eSAiKFNDU0lfSURFTlRffElEX1dXTikiCn0KYmxhY2tsaXN0IHsKfQo=" }
	},
    {
      "overwrite": true,
      "path": "/etc/NetworkManager/conf.d/01-ipv6.conf",
      "mode": 420,
      "user": {
          "name": "root"
      },
      "contents": { "source": "data:,{{.IPv6_CONF}}" }
    },
    {
        "overwrite": true,
        "path": "/root/.docker/config.json",
        "mode": 420,
        "user": {
            "name": "root"
        },
        "contents": { "source": "data:,{{.PULL_SECRET}}" }
    },
    {
        "overwrite": true,
        "path": "/root/assisted.te",
        "mode": 420,
        "user": {
            "name": "root"
        },
        "contents": { "source": "data:text/plain;base64,{{.SELINUX_POLICY}}" }
    }{{if .RH_ROOT_CA}},
    {
      "overwrite": true,
      "path": "/etc/pki/ca-trust/source/anchors/rh-it-root-ca.crt",
      "mode": 420,
      "user": {
          "name": "root"
      },
      "contents": { "source": "data:,{{.RH_ROOT_CA}}" }
    }{{end}}{{if .HostCACertPath}},
    {
      "path": "{{.HostCACertPath}}",
      "mode": 420,
      "overwrite": true,
      "user": {
        "name": "root"
      },
      "contents": { "source": "{{.ServiceCACertData}}" }
    }{{end}}{{if .ServiceIPs}},
    {
      "path": "/etc/hosts",
      "mode": 420,
      "user": {
        "name": "root"
      },
      "append": [{ "source": "{{.ServiceIPs}}" }]
    }{{end}}{{if .MirrorRegistriesConfig}},
    {
      "path": "/etc/containers/registries.conf",
      "mode": 420,
      "overwrite": true,
      "user": {
        "name": "root"
      },
      "contents": { "source": "data:text/plain;base64,{{.MirrorRegistriesConfig}}"}
    },
    {
      "path": "/etc/pki/ca-trust/source/anchors/domain.crt",
      "mode": 420,
      "overwrite": true,
      "user": {
        "name": "root"
      },
      "contents": { "source": "data:text/plain;base64,{{.MirrorRegistriesCAConfig}}"}
    }{{end}}{{if .StaticNetworkConfig}},
    {
        "path": "/usr/local/bin/pre-network-manager-config.sh",
        "mode": 493,
        "overwrite": true,
        "user": {
            "name": "root"
        },
        "contents": { "source": "data:text/plain;base64,{{.PreNetworkConfigScript}}"}
    }{{end}}{{range .StaticNetworkConfig}},
    {
      "path": "{{.FilePath}}",
      "mode": 384,
      "overwrite": true,
      "user": {
        "name": "root"
      },
      "contents": { "source": "data:text/plain;base64,{{.FileContents}}"}
    }{{end}}{{if .OKDBinaries}},
    {
      "path": "/usr/local/bin/okd-binaries.sh",
      "mode": 755,
      "overwrite": true,
      "user": {
        "name": "root"
      },
      "contents": { "source": "data:text/plain;base64,{{.OKDBinaries}}" }
    }{{end}}{{if .OKDHoldPivot}},{
      "path": "/etc/systemd/system/release-image-pivot.service.d/wait-for-okd.conf",
      "mode": 420,
      "overwrite": true,
      "user": {
        "name": "root"
      },
      "contents": { "source": "data:text/plain;base64,{{.OKDHoldPivot}}" }
    }{{end}}{{if .OKDHoldAgent}},
    {
      "path": "/etc/systemd/system/agent.service.d/wait-for-okd.conf",
      "mode": 420,
      "overwrite": true,
      "user": {
        "name": "root"
      },
      "contents": { "source": "data:text/plain;base64,{{.OKDHoldAgent}}" }
    }{{end}}]
  }
}`

const secondDayWorkerIgnitionFormat = `{
	"ignition": {
	  "version": "3.1.0",
	  "config": {
		"merge": [{
		  "source": "{{.SOURCE}}"{{if .HEADERS}},
          "httpHeaders": [{{range $k,$v := .HEADERS}}{"name": "{{$k}}", "value": "{{$v}}"}{{end}}]{{end}}
		}]
	  }{{if .CACERT}},
          "security": {
            "tls": {
	      "certificateAuthorities": [{
	        "source": "{{.CACERT}}"
	      }]
	    }
	  }{{end}}
    }
 }`

const tempNMConnectionsDir = "/etc/assisted/network"

var fileNames = [...]string{
	"bootstrap.ign",
	masterIgn,
	"metadata.json",
	workerIgn,
	"kubeconfig-noingress",
	"kubeadmin-password",
	"install-config.yaml",
}

// Generator can generate ignition files and upload them to an S3-like service
type Generator interface {
	Generate(ctx context.Context, installConfig []byte, platformType models.PlatformType) error
	UploadToS3(ctx context.Context) error
	UpdateEtcHosts(string) error
}

// IgnitionBuilder defines the ignition formatting methods for the various images
//go:generate mockgen -source=ignition.go -package=ignition -destination=mock_ignition.go
type IgnitionBuilder interface {
	FormatDiscoveryIgnitionFile(ctx context.Context, infraEnv *common.InfraEnv, cfg IgnitionConfig, safeForLogs bool, authType auth.AuthType) (string, error)
	FormatSecondDayWorkerIgnitionFile(url string, caCert *string, bearerToken string, host *models.Host) ([]byte, error)
}

type installerGenerator struct {
	log                           logrus.FieldLogger
	workDir                       string
	cluster                       *common.Cluster
	releaseImage                  string
	releaseImageMirror            string
	installerDir                  string
	serviceCACert                 string
	encodedDhcpFileContents       string
	s3Client                      s3wrapper.API
	enableMetal3Provisioning      bool
	operatorsApi                  operators.API
	installInvoker                string
	providerRegistry              registry.ProviderRegistry
	installerReleaseImageOverride string
	clusterTLSCertOverrideDir     string
}

// IgnitionConfig contains the attributes required to build the discovery ignition file
type IgnitionConfig struct {
	AgentDockerImg       string        `envconfig:"AGENT_DOCKER_IMAGE" default:"quay.io/edge-infrastructure/assisted-installer-agent:latest"`
	AgentTimeoutStart    time.Duration `envconfig:"AGENT_TIMEOUT_START" default:"10m"`
	InstallRHCa          bool          `envconfig:"INSTALL_RH_CA" default:"false"`
	ServiceBaseURL       string        `envconfig:"SERVICE_BASE_URL"`
	ServiceCACertPath    string        `envconfig:"SERVICE_CA_CERT_PATH" default:""`
	ServiceIPs           string        `envconfig:"SERVICE_IPS" default:""`
	SkipCertVerification bool          `envconfig:"SKIP_CERT_VERIFICATION" default:"false"`
	OKDRPMsImage         string        `envconfig:"OKD_RPMS_IMAGE" default:""`
}

type ignitionBuilder struct {
	log                     logrus.FieldLogger
	staticNetworkConfig     staticnetworkconfig.StaticNetworkConfig
	mirrorRegistriesBuilder mirrorregistries.MirrorRegistriesConfigBuilder
}

func NewBuilder(log logrus.FieldLogger, staticNetworkConfig staticnetworkconfig.StaticNetworkConfig, mirrorRegistriesBuilder mirrorregistries.MirrorRegistriesConfigBuilder) IgnitionBuilder {
	builder := &ignitionBuilder{
		log:                     log,
		staticNetworkConfig:     staticNetworkConfig,
		mirrorRegistriesBuilder: mirrorRegistriesBuilder,
	}
	return builder
}

// NewGenerator returns a generator that can generate ignition files
func NewGenerator(workDir string, installerDir string, cluster *common.Cluster, releaseImage string, releaseImageMirror string,
	serviceCACert, installInvoker string, s3Client s3wrapper.API, log logrus.FieldLogger, operatorsApi operators.API,
	providerRegistry registry.ProviderRegistry, installerReleaseImageOverride, clusterTLSCertOverrideDir string) Generator {
	return &installerGenerator{
		cluster:                       cluster,
		log:                           log,
		releaseImage:                  releaseImage,
		releaseImageMirror:            releaseImageMirror,
		workDir:                       workDir,
		installerDir:                  installerDir,
		serviceCACert:                 serviceCACert,
		s3Client:                      s3Client,
		enableMetal3Provisioning:      true,
		operatorsApi:                  operatorsApi,
		installInvoker:                installInvoker,
		providerRegistry:              providerRegistry,
		installerReleaseImageOverride: installerReleaseImageOverride,
		clusterTLSCertOverrideDir:     clusterTLSCertOverrideDir,
	}
}

// UploadToS3 uploads generated ignition and related files to the configured
// S3-compatible storage
func (g *installerGenerator) UploadToS3(ctx context.Context) error {
	return uploadToS3(ctx, g.workDir, g.cluster, g.s3Client, g.log)
}

// Generate generates ignition files and applies modifications.
func (g *installerGenerator) Generate(ctx context.Context, installConfig []byte, platformType models.PlatformType) error {
	var icspFile string
	log := logutil.FromContext(ctx, g.log)

	// In case we don't want to override image for extracting installer use release one
	if g.installerReleaseImageOverride == "" {
		g.installerReleaseImageOverride = g.releaseImage
	}

	// If ImageContentSources are defined, store in a file for the 'oc' command
	icspFile, err := getIcspFileFromInstallConfig(installConfig, g.log)
	if err != nil {
		return errors.Wrap(err, "failed to create file with ImageContentSources")
	}
	defer removeIcspFile(icspFile)

	installerPath, err := installercache.Get(g.installerReleaseImageOverride, g.releaseImageMirror, g.installerDir,
		g.cluster.PullSecret, platformType, icspFile, log)
	if err != nil {
		return errors.Wrap(err, "failed to get installer path")
	}
	installConfigPath := filepath.Join(g.workDir, "install-config.yaml")

	g.enableMetal3Provisioning, err = common.VersionGreaterOrEqual(g.cluster.Cluster.OpenshiftVersion, "4.7")
	if err != nil {
		return err
	}

	g.encodedDhcpFileContents, err = network.GetEncodedDhcpParamFileContents(g.cluster)
	if err != nil {
		wrapped := errors.Wrapf(err, "Could not create DHCP encoded file")
		log.WithError(wrapped).Errorf("GenerateInstallConfig")
		return wrapped
	}
	envVars := append(os.Environ(),
		"OPENSHIFT_INSTALL_RELEASE_IMAGE_OVERRIDE="+g.releaseImage,
		"OPENSHIFT_INSTALL_INVOKER="+g.installInvoker,
	)
	if g.clusterTLSCertOverrideDir != "" {
		envVars = append(envVars, "OPENSHIFT_INSTALL_LOAD_CLUSTER_CERTS=true")
	}

	// write installConfig to install-config.yaml so openshift-install can read it
	err = ioutil.WriteFile(installConfigPath, installConfig, 0600)
	if err != nil {
		log.Errorf("failed to write file %s", installConfigPath)
		return err
	}

	manifestFiles, err := manifests.GetClusterManifests(ctx, g.cluster.ID, g.s3Client)
	if err != nil {
		log.WithError(err).Errorf("failed to check if cluster %s has manifests", g.cluster.ID)
		return err
	}

	err = g.providerRegistry.PreCreateManifestsHook(g.cluster, &envVars, g.workDir)

	if err != nil {
		log.WithError(err).Errorf("failed to run pre manifests creation hook '%s'", common.PlatformTypeValue(g.cluster.Platform.Type))
		return err
	}

	err = g.importClusterTLSCerts(ctx)
	if err != nil {
		log.WithError(err).Error("Failed to import cluster TLS certs")
		return err
	}

	err = g.runCreateCommand(ctx, installerPath, "manifests", envVars)
	if err != nil {
		return err
	}
	err = g.providerRegistry.PostCreateManifestsHook(g.cluster, &envVars, g.workDir)
	if err != nil {
		log.WithError(err).Errorf("failed to run post manifests creation hook '%s'", common.PlatformTypeValue(g.cluster.Platform.Type))
		return err
	}

	// download manifests files to working directory
	for _, manifest := range manifestFiles {
		log.Infof("adding manifest %s to working dir for cluster %s", manifest, g.cluster.ID)
		err = g.downloadManifest(ctx, manifest)
		if err != nil {
			_ = os.Remove(filepath.Join(g.workDir, "manifests"))
			_ = os.Remove(filepath.Join(g.workDir, "openshift"))
			log.WithError(err).Errorf("Failed to download manifest %s to working dir for cluster %s", manifest, g.cluster.ID)
			return err
		}
	}

	if swag.StringValue(g.cluster.HighAvailabilityMode) == models.ClusterHighAvailabilityModeNone {
		err = g.bootstrapInPlaceIgnitionsCreate(ctx, installerPath, envVars)
	} else {
		err = g.runCreateCommand(ctx, installerPath, "ignition-configs", envVars)
	}
	if err != nil {
		log.Error(err)
		return err
	}

	// parse ignition and update BareMetalHosts
	bootstrapPath := filepath.Join(g.workDir, "bootstrap.ign")
	err = g.updateBootstrap(ctx, bootstrapPath)
	if err != nil {
		return err
	}

	err = g.updateIgnitions()
	if err != nil {
		log.Error(err)
		return err
	}

	err = g.createHostIgnitions()
	if err != nil {
		log.Error(err)
		return err
	}

	// move all files into the working directory
	err = os.Rename(filepath.Join(g.workDir, "auth/kubeadmin-password"), filepath.Join(g.workDir, "kubeadmin-password"))
	if err != nil {
		return err
	}
	// after installation completes, a new kubeconfig will be created and made
	// available that includes ingress details, so we rename this one
	err = os.Rename(filepath.Join(g.workDir, "auth/kubeconfig"), filepath.Join(g.workDir, "kubeconfig-noingress"))
	if err != nil {
		return err
	}
	// We want to save install-config.yaml
	// Installer deletes it so we need to write it one more time
	err = ioutil.WriteFile(installConfigPath, installConfig, 0600)
	if err != nil {
		log.Errorf("Failed to write file %s", installConfigPath)
		return err
	}

	err = os.Remove(filepath.Join(g.workDir, "auth"))
	if err != nil {
		return err
	}
	return nil
}

func (g *installerGenerator) importClusterTLSCerts(ctx context.Context) error {
	if g.clusterTLSCertOverrideDir == "" {
		return nil
	}
	log := logutil.FromContext(ctx, g.log).WithField("inputDir", g.clusterTLSCertOverrideDir)
	log.Debug("Checking for cluster TLS certs dir")

	entries, err := os.ReadDir(g.clusterTLSCertOverrideDir)
	if err != nil && !os.IsNotExist(err) {
		return errors.Wrapf(err, "failed to read cluster TLS certs dir \"%s\"", g.clusterTLSCertOverrideDir)
	}
	log.Info("Found cluster TLS certs dir")

	outDir := filepath.Join(g.workDir, "tls")
	log = log.WithField("outputDir", outDir).WithField("cluster", g.cluster.ID)
	if err := os.Mkdir(outDir, 0755); err != nil {
		return errors.Wrapf(err, "failed to create cluster TLS certs output dir \"%s\"", outDir)
	}
	log.Info("Created cluster TLS certs dir")
	tlsFS := os.DirFS(g.clusterTLSCertOverrideDir)

	copyFile := func(filename string) error {
		log.Info("Copying cluster TLS cert file", "filename", filename)

		f, err := tlsFS.Open(filename)
		if err != nil {
			return errors.Wrapf(err, "failed to open cluster TLS cert file \"%s\"", filename)
		}
		defer f.Close()
		c, err := ioutil.ReadAll(f)
		if err != nil {
			return errors.Wrapf(err, "failed to read cluster TLS cert file \"%s\"", filename)
		}
		err = ioutil.WriteFile(filepath.Join(outDir, filename), c, 0600)
		if err != nil {
			return errors.Wrapf(err, "failed to write cluster TLS cert file \"%s\"", filename)
		}

		return nil
	}

	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		if err := copyFile(e.Name()); err != nil {
			return err
		}
	}
	return nil
}

func (g *installerGenerator) bootstrapInPlaceIgnitionsCreate(ctx context.Context, installerPath string, envVars []string) error {
	err := g.runCreateCommand(ctx, installerPath, "single-node-ignition-config", envVars)
	if err != nil {
		return errors.Wrapf(err, "Failed to create single node ignitions")
	}

	bootstrapPath := filepath.Join(g.workDir, "bootstrap.ign")
	// In case of single node rename bootstrap Ignition file
	err = os.Rename(filepath.Join(g.workDir, "bootstrap-in-place-for-live-iso.ign"), bootstrapPath)
	if err != nil {
		return errors.Wrapf(err, "Failed to rename bootstrap-in-place-for-live-iso.ign")
	}

	bootstrapConfig, err := parseIgnitionFile(bootstrapPath)
	if err != nil {
		return err
	}
	//Although BIP works with 4.8 and above we want to support early 4.8 CI images
	// To that end we set the dummy master ignition version to the same version as the bootstrap ignition
	config := config_latest_types.Config{Ignition: config_latest_types.Ignition{Version: bootstrapConfig.Ignition.Version}}
	for _, file := range []string{masterIgn, workerIgn} {
		err = writeIgnitionFile(filepath.Join(g.workDir, file), &config)
		if err != nil {
			return errors.Wrapf(err, "Failed to create %s", file)
		}
	}

	return nil
}

func getHostnames(hosts []*models.Host) []string {
	ret := make([]string, 0)
	for _, h := range hosts {
		ret = append(ret, hostutil.GetHostnameForMsg(h))
	}
	return ret

}

func bmhIsMaster(bmh *bmh_v1alpha1.BareMetalHost, masterHostnames, workerHostnames []string) bool {
	if funk.ContainsString(masterHostnames, bmh.Name) {
		return true
	}
	if funk.ContainsString(workerHostnames, bmh.Name) {
		return false
	}

	// For backward compatibility in case the name is not in the (masterHostnames, workerHostnames)
	return strings.Contains(bmh.Name, "-master-")
}

type clusterVersion struct {
	APIVersion string `yaml:"apiVersion"`
	Metadata   struct {
		Namespace string `yaml:"namespace"`
		Name      string `yaml:"name"`
	} `yaml:"metadata"`
	Spec struct {
		Upstream  string `yaml:"upstream"`
		Channel   string `yaml:"channel"`
		ClusterID string `yaml:"clusterID"`
	} `yaml:"spec"`
}

// ExtractClusterID gets a local path of a "bootstrap.ign" file and extracts the OpenShift cluster ID
func ExtractClusterID(reader io.ReadCloser) (string, error) {
	bs, err := ioutil.ReadAll(reader)
	if err != nil {
		return "", err
	}

	config, err := ParseToLatest(bs)
	if err != nil {
		return "", err
	}

	for _, f := range config.Storage.Files {
		if f.Node.Path != "/opt/openshift/manifests/cvo-overrides.yaml" {
			continue
		}

		source := f.FileEmbedded1.Contents.Key()
		dataURL, err := dataurl.DecodeString(source)
		if err != nil {
			return "", err
		}

		cv := clusterVersion{}
		err = yaml.Unmarshal(dataURL.Data, &cv)
		if err != nil {
			return "", err
		}

		if cv.Spec.ClusterID == "" {
			return "", errors.New("no ClusterID field in cvo-overrides file")
		}

		return cv.Spec.ClusterID, nil
	}

	return "", errors.New("could not find cvo-overrides file")
}

// updateBootstrap adds a status annotation to each BareMetalHost defined in the
// bootstrap ignition file
func (g *installerGenerator) updateBootstrap(ctx context.Context, bootstrapPath string) error {
	log := logutil.FromContext(ctx, g.log)
	config, err := parseIgnitionFile(bootstrapPath)
	if err != nil {
		g.log.Error(err)
		return err
	}

	newFiles := []config_latest_types.File{}

	masters, workers := sortHosts(g.cluster.Hosts)
	for i, file := range config.Storage.Files {
		switch {
		case isBaremetalProvisioningConfig(&config.Storage.Files[i]):
			if !g.enableMetal3Provisioning {
				// drop this from the list of Files because we don't want to run BMO
				continue
			}
		case isMOTD(&config.Storage.Files[i]):
			// workaround for https://github.com/openshift/machine-config-operator/issues/2086
			g.fixMOTDFile(&config.Storage.Files[i])
		case isBMHFile(&config.Storage.Files[i]):
			// extract bmh
			bmh, err := fileToBMH(&config.Storage.Files[i]) //nolint,shadow
			if err != nil {
				log.Errorf("error parsing File contents to BareMetalHost: %v", err)
				return err
			}

			// get corresponding host
			var host *models.Host
			masterHostnames := getHostnames(masters)
			workerHostnames := getHostnames(workers)

			// The BMH files in the ignition are sorted according to hostname (please see the implementation in installcfg/installcfg.go).
			// The masters and workers are also sorted by hostname.  This enables us to correlate correctly the host and the BMH file
			if bmhIsMaster(bmh, masterHostnames, workerHostnames) {
				if len(masters) == 0 {
					return errors.Errorf("Not enough registered masters to match with BareMetalHosts")
				}
				host, masters = masters[0], masters[1:]
			} else {
				if len(workers) == 0 {
					return errors.Errorf("Not enough registered workers to match with BareMetalHosts")
				}
				host, workers = workers[0], workers[1:]
			}

			// modify bmh
			log.Infof("modifying BareMetalHost ignition file %s", file.Node.Path)
			err = g.modifyBMHFile(&config.Storage.Files[i], bmh, host)
			if err != nil {
				return err
			}
		}
		newFiles = append(newFiles, config.Storage.Files[i])
	}

	config.Storage.Files = newFiles
	if swag.StringValue(g.cluster.HighAvailabilityMode) != models.ClusterHighAvailabilityModeNone {
		setFileInIgnition(config, "/opt/openshift/assisted-install-bootstrap", "data:,", false, 420, false)
	}

	// add new Network Manager config file that disables handling of /etc/resolv.conf
	// as there is no network scripts added in SNO mode (None) we should not touch Netmanager config
	if !common.IsSingleNodeCluster(g.cluster) {
		setNMConfigration(config)
	}

	err = writeIgnitionFile(bootstrapPath, config)
	if err != nil {
		log.Error(err)
		return err
	}
	log.Infof("Updated file %s", bootstrapPath)

	return nil
}

func setNMConfigration(config *config_latest_types.Config) {
	fileContents := "data:text/plain;charset=utf-8;base64," + base64.StdEncoding.EncodeToString([]byte(common.UnmanagedResolvConf))
	setFileInIgnition(config, "/etc/NetworkManager/conf.d/99-kni.conf", fileContents, false, 420, false)
}

func isBMHFile(file *config_latest_types.File) bool {
	return strings.Contains(file.Node.Path, "openshift-cluster-api_hosts")
}

func isMOTD(file *config_latest_types.File) bool {
	return file.Node.Path == "/etc/motd"
}

func isBaremetalProvisioningConfig(file *config_latest_types.File) bool {
	return strings.Contains(file.Node.Path, "baremetal-provisioning-config")
}

func fileToBMH(file *config_latest_types.File) (*bmh_v1alpha1.BareMetalHost, error) {
	parts := strings.Split(*file.Contents.Source, "base64,")
	if len(parts) != 2 {
		return nil, errors.Errorf("could not parse source for file %s", file.Node.Path)
	}
	decoded, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}

	bmh := &bmh_v1alpha1.BareMetalHost{}
	_, _, err = scheme.Codecs.UniversalDeserializer().Decode(decoded, nil, bmh)
	if err != nil {
		return nil, err
	}

	return bmh, nil
}

// fixMOTDFile is a workaround for a bug in machine-config-operator, where it
// incorrectly parses igition when a File is configured to append content
// instead of overwrite. Currently, /etc/motd is the only file involved in
// provisioning that is configured for appending. This code converts it to
// overwriting the existing /etc/motd with whatever content had been indended
// to be appened.
// https://github.com/openshift/machine-config-operator/issues/2086
func (g *installerGenerator) fixMOTDFile(file *config_latest_types.File) {
	if file.Contents.Source != nil {
		// the bug only happens if Source == nil, so no need to take action
		return
	}
	if len(file.Append) == 1 {
		file.Contents.Source = file.Append[0].Source
		file.Append = file.Append[:0]
		return
	}
	g.log.Info("could not apply workaround to file /etc/motd for MCO bug. The workaround may no longer be necessary.")
}

// modifyBMHFile modifies the File contents so that the serialized BareMetalHost
// includes a status annotation
func (g *installerGenerator) modifyBMHFile(file *config_latest_types.File, bmh *bmh_v1alpha1.BareMetalHost, host *models.Host) error {
	inventory := models.Inventory{}
	err := json.Unmarshal([]byte(host.Inventory), &inventory)
	if err != nil {
		return err
	}

	hw := bmh_v1alpha1.HardwareDetails{
		CPU: bmh_v1alpha1.CPU{
			Arch:           inventory.CPU.Architecture,
			Model:          inventory.CPU.ModelName,
			ClockMegahertz: bmh_v1alpha1.ClockSpeed(inventory.CPU.Frequency),
			Flags:          inventory.CPU.Flags,
			Count:          int(inventory.CPU.Count),
		},
		Hostname: host.RequestedHostname,
		NIC:      make([]bmh_v1alpha1.NIC, len(inventory.Interfaces)),
		Storage:  make([]bmh_v1alpha1.Storage, len(inventory.Disks)),
	}
	if inventory.Memory != nil {
		hw.RAMMebibytes = int(inventory.Memory.PhysicalBytes / 1024 / 1024)
	}
	for i, iface := range inventory.Interfaces {
		hw.NIC[i] = bmh_v1alpha1.NIC{
			Name:      iface.Name,
			Model:     iface.Product,
			MAC:       iface.MacAddress,
			SpeedGbps: int(iface.SpeedMbps / 1024),
		}
		switch {
		case len(iface.IPV4Addresses) > 0:
			hw.NIC[i].IP = g.getInterfaceIP(iface.IPV4Addresses[0])
		case len(iface.IPV6Addresses) > 0:
			hw.NIC[i].IP = g.getInterfaceIP(iface.IPV6Addresses[0])
		}
	}
	for i, disk := range inventory.Disks {
		hw.Storage[i] = bmh_v1alpha1.Storage{
			Name:         disk.Name,
			Vendor:       disk.Vendor,
			SizeBytes:    bmh_v1alpha1.Capacity(disk.SizeBytes),
			Model:        disk.Model,
			WWN:          disk.Wwn,
			HCTL:         disk.Hctl,
			SerialNumber: disk.Serial,
			Rotational:   (disk.DriveType == models.DriveTypeHDD),
		}
	}
	if inventory.SystemVendor != nil {
		hw.SystemVendor = bmh_v1alpha1.HardwareSystemVendor{
			Manufacturer: inventory.SystemVendor.Manufacturer,
			ProductName:  inventory.SystemVendor.ProductName,
			SerialNumber: inventory.SystemVendor.SerialNumber,
		}
	}
	status := bmh_v1alpha1.BareMetalHostStatus{
		HardwareDetails: &hw,
		PoweredOn:       true,
	}
	statusJSON, err := json.Marshal(status)
	if err != nil {
		return err
	}
	metav1.SetMetaDataAnnotation(&bmh.ObjectMeta, bmh_v1alpha1.StatusAnnotation, string(statusJSON))
	if g.enableMetal3Provisioning {
		bmh.Spec.ExternallyProvisioned = true
	}

	serializer := k8sjson.NewSerializerWithOptions(
		k8sjson.DefaultMetaFactory, nil, nil,
		k8sjson.SerializerOptions{
			Yaml:   true,
			Pretty: true,
			Strict: true,
		},
	)
	buf := bytes.Buffer{}
	err = serializer.Encode(bmh, &buf)
	if err != nil {
		return err
	}

	// remove status if exists
	res := bytes.Split(buf.Bytes(), []byte("status:\n"))
	encodedBMH := base64.StdEncoding.EncodeToString(res[0])
	source := "data:text/plain;charset=utf-8;base64," + encodedBMH
	file.Contents.Source = &source

	return nil
}

func (g *installerGenerator) updateDhcpFiles() error {
	path := filepath.Join(g.workDir, masterIgn)
	config, err := parseIgnitionFile(path)
	if err != nil {
		return err
	}
	setFileInIgnition(config, "/etc/keepalived/unsupported-monitor.conf", g.encodedDhcpFileContents, false, 0o644, false)
	encodedApiVip := network.GetEncodedApiVipLease(g.cluster)
	if encodedApiVip != "" {
		setFileInIgnition(config, "/etc/keepalived/lease-api", encodedApiVip, false, 0o644, false)
	}
	encodedIngressVip := network.GetEncodedIngressVipLease(g.cluster)
	if encodedIngressVip != "" {
		setFileInIgnition(config, "/etc/keepalived/lease-ingress", encodedIngressVip, false, 0o644, false)
	}
	err = writeIgnitionFile(path, config)
	if err != nil {
		return err
	}
	return nil
}

func encodeIpv6Contents(config string) string {
	return fmt.Sprintf("data:,%s", url.PathEscape(config))
}

// addIpv6FileInIgnition adds a NetworkManager configuration ensuring that IPv6 DHCP requests use
// consistent client identification.
func (g *installerGenerator) addIpv6FileInIgnition(ignition string) error {
	path := filepath.Join(g.workDir, ignition)
	config, err := parseIgnitionFile(path)
	if err != nil {
		return err
	}
	is410Version, err := common.VersionGreaterOrEqual(g.cluster.OpenshiftVersion, "4.10.0-0.alpha")
	if err != nil {
		return err
	}
	v6config := common.Ipv6DuidRuntimeConfPre410
	if is410Version {
		v6config = common.Ipv6DuidRuntimeConf
	}
	setFileInIgnition(config, "/etc/NetworkManager/conf.d/01-ipv6.conf", encodeIpv6Contents(v6config), false, 0o644, false)
	err = writeIgnitionFile(path, config)
	if err != nil {
		return err
	}
	return nil
}

func (g *installerGenerator) updateIgnitions() error {
	masterPath := filepath.Join(g.workDir, masterIgn)
	caCertFile := g.serviceCACert

	if caCertFile != "" {
		err := setCACertInIgnition(models.HostRoleMaster, masterPath, g.workDir, caCertFile)
		if err != nil {
			return errors.Wrapf(err, "error adding CA cert to ignition %s", masterPath)
		}
	}

	if g.encodedDhcpFileContents != "" {
		if err := g.updateDhcpFiles(); err != nil {
			return errors.Wrapf(err, "error adding DHCP file to ignition %s", masterPath)
		}
	}

	workerPath := filepath.Join(g.workDir, workerIgn)
	if caCertFile != "" {
		err := setCACertInIgnition(models.HostRoleWorker, workerPath, g.workDir, caCertFile)
		if err != nil {
			return errors.Wrapf(err, "error adding CA cert to ignition %s", workerPath)
		}
	}

	_, ipv6, err := network.GetClusterAddressStack(g.cluster.Hosts)
	if err != nil {
		return err
	}
	if ipv6 {
		for _, ignition := range []string{masterIgn, workerIgn} {
			if err = g.addIpv6FileInIgnition(ignition); err != nil {
				return err
			}
		}
	}
	return nil
}

func (g *installerGenerator) UpdateEtcHosts(serviceIPs string) error {
	masterPath := filepath.Join(g.workDir, masterIgn)

	if serviceIPs != "" {
		err := setEtcHostsInIgnition(models.HostRoleMaster, masterPath, g.workDir, GetServiceIPHostnames(serviceIPs))
		if err != nil {
			return errors.Wrapf(err, "error adding Etc Hosts to ignition %s", masterPath)
		}
	}

	workerPath := filepath.Join(g.workDir, workerIgn)
	if serviceIPs != "" {
		err := setEtcHostsInIgnition(models.HostRoleWorker, workerPath, g.workDir, GetServiceIPHostnames(serviceIPs))
		if err != nil {
			return errors.Wrapf(err, "error adding Etc Hosts to ignition %s", workerPath)
		}
	}
	return nil
}

// sortHosts sorts hosts into masters and workers, excluding disabled hosts
func sortHosts(hosts []*models.Host) ([]*models.Host, []*models.Host) {
	masters := []*models.Host{}
	workers := []*models.Host{}
	for i := range hosts {
		switch {
		case common.GetEffectiveRole(hosts[i]) == models.HostRoleMaster:
			masters = append(masters, hosts[i])
		default:
			workers = append(workers, hosts[i])
		}
	}

	// sort them so the result is repeatable
	sort.SliceStable(masters, func(i, j int) bool {
		return hostutil.GetHostnameForMsg(masters[i]) < hostutil.GetHostnameForMsg(masters[j])
	})
	sort.SliceStable(workers, func(i, j int) bool {
		return hostutil.GetHostnameForMsg(workers[i]) < hostutil.GetHostnameForMsg(workers[j])
	})
	return masters, workers
}

// UploadToS3 uploads the generated files to S3
func uploadToS3(ctx context.Context, workDir string, cluster *common.Cluster, s3Client s3wrapper.API, log logrus.FieldLogger) error {
	toUpload := fileNames[:]
	for _, host := range cluster.Hosts {
		toUpload = append(toUpload, hostutil.IgnitionFileName(host))
	}

	for _, fileName := range toUpload {
		fullPath := filepath.Join(workDir, fileName)
		key := filepath.Join(cluster.ID.String(), fileName)
		err := s3Client.UploadFile(ctx, fullPath, key)
		if err != nil {
			log.Errorf("Failed to upload file %s as object %s", fullPath, key)
			return err
		}
		_, err = s3Client.UpdateObjectTimestamp(ctx, key)
		if err != nil {
			return err
		}
		log.Infof("Uploaded file %s as object %s", fullPath, key)
	}

	return nil
}

// ParseToLatest takes the Ignition config and tries to parse it as v3.2 and if that fails,
// as v3.1. This is in order to support the latest possible Ignition as well as to preserve
// backwards-compatibility with OCP 4.6 that supports only Ignition up to v3.1
func ParseToLatest(content []byte) (*config_latest_types.Config, error) {
	config, _, err := config_latest.Parse(content)
	if err != nil {
		// TODO(deprecate-ignition-3.1.0)
		// We always want to work with the object of the type v3.2 but carry a value of v3.1 inside.
		// For this reason we are translating the config to v3.2 and manually override the Version.
		configBackwards, _, err := config_31.Parse(content)
		if err != nil {
			return nil, errors.Errorf("error parsing ignition: %v", err)
		}
		config = config_latest_trans.Translate(configBackwards)
		config.Ignition.Version = "3.1.0"
	}
	return &config, nil
}

func parseIgnitionFile(path string) (*config_latest_types.Config, error) {
	configBytes, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, errors.Errorf("error reading file %s: %v", path, err)
	}
	return ParseToLatest(configBytes)
}

func (g *installerGenerator) getInterfaceIP(cidr string) string {
	ip, _, err := net.ParseCIDR(cidr)
	if err != nil {
		g.log.Warnf("Failed to parse cidr %s for filling BMH CR", cidr)
		return ""
	}
	return ip.String()
}

// writeIgnitionFile writes an ignition config to a given path on disk
func writeIgnitionFile(path string, config *config_latest_types.Config) error {
	updatedBytes, err := json.Marshal(config)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(path, updatedBytes, 0600)
	if err != nil {
		return errors.Wrapf(err, "error writing file %s", path)
	}

	return nil
}

func setFileInIgnition(config *config_latest_types.Config, filePath string, fileContents string, appendContent bool, mode int, overwrite bool) {
	rootUser := "root"
	file := config_latest_types.File{
		Node: config_latest_types.Node{
			Path:      filePath,
			Overwrite: &overwrite,
			Group:     config_latest_types.NodeGroup{},
			User:      config_latest_types.NodeUser{Name: &rootUser},
		},
		FileEmbedded1: config_latest_types.FileEmbedded1{
			Append: []config_latest_types.Resource{},
			Contents: config_latest_types.Resource{
				Source: &fileContents,
			},
			Mode: &mode,
		},
	}
	if appendContent {
		file.FileEmbedded1.Append = []config_latest_types.Resource{
			{
				Source: &fileContents,
			},
		}
		file.FileEmbedded1.Contents = config_latest_types.Resource{}
	}
	config.Storage.Files = append(config.Storage.Files, file)
}

//lint:ignore U1000 Ignore unused function
//nolint:unused,deadcode
func setUnitInIgnition(config *config_latest_types.Config, contents, name string, enabled bool) {
	newUnit := config_latest_types.Unit{
		Contents: swag.String(contents),
		Name:     name,
		Enabled:  swag.Bool(enabled),
	}
	config.Systemd.Units = append(config.Systemd.Units, newUnit)
}

func setCACertInIgnition(role models.HostRole, path string, workDir string, caCertFile string) error {
	config, err := parseIgnitionFile(path)
	if err != nil {
		return err
	}

	var caCertData []byte
	caCertData, err = ioutil.ReadFile(caCertFile)
	if err != nil {
		return err
	}

	setFileInIgnition(config, common.HostCACertPath, fmt.Sprintf("data:,%s", url.PathEscape(string(caCertData))), false, 420, false)

	fileName := fmt.Sprintf("%s.ign", role)
	err = writeIgnitionFile(filepath.Join(workDir, fileName), config)
	if err != nil {
		return err
	}

	return nil
}

func writeHostFiles(hosts []*models.Host, baseFile string, workDir string) error {
	g := new(errgroup.Group)
	for i := range hosts {
		host := hosts[i]
		g.Go(func() error {
			config, err := parseIgnitionFile(filepath.Join(workDir, baseFile))
			if err != nil {
				return err
			}

			hostname, err := hostutil.GetCurrentHostName(host)
			if err != nil {
				return errors.Wrapf(err, "failed to get hostname for host %s", host.ID)
			}

			setFileInIgnition(config, "/etc/hostname", fmt.Sprintf("data:,%s", hostname), false, 420, true)

			configBytes, err := json.Marshal(config)
			if err != nil {
				return err
			}

			if host.IgnitionConfigOverrides != "" {
				merged, mergeErr := MergeIgnitionConfig(configBytes, []byte(host.IgnitionConfigOverrides))
				if mergeErr != nil {
					return errors.Wrapf(mergeErr, "failed to apply ignition config overrides for host %s", host.ID)
				}
				configBytes = []byte(merged)
			}

			err = ioutil.WriteFile(filepath.Join(workDir, hostutil.IgnitionFileName(host)), configBytes, 0600)
			if err != nil {
				return errors.Wrapf(err, "failed to write ignition for host %s", host.ID)
			}

			return nil
		})
	}

	return g.Wait()
}

// createHostIgnitions builds an ignition file for each host in the cluster based on the generated <role>.ign file
func (g *installerGenerator) createHostIgnitions() error {
	masters, workers := sortHosts(g.cluster.Hosts)

	err := writeHostFiles(masters, masterIgn, g.workDir)
	if err != nil {
		return errors.Wrapf(err, "error writing master host ignition files")
	}

	err = writeHostFiles(workers, workerIgn, g.workDir)
	if err != nil {
		return errors.Wrapf(err, "error writing worker host ignition files")
	}

	return nil
}

func MergeIgnitionConfig(base []byte, overrides []byte) (string, error) {
	baseConfig, err := ParseToLatest(base)
	if err != nil {
		return "", err
	}

	overrideConfig, err := ParseToLatest(overrides)
	if err != nil {
		return "", err
	}

	mergeResult, _ := merge.MergeStructTranscribe(*baseConfig, *overrideConfig)
	res, err := json.Marshal(mergeResult)
	if err != nil {
		return "", err
	}

	// TODO(deprecate-ignition-3.1.0)
	// We want to validate if users do not try to override with putting features of 3.2.0 into
	// ignition manifest of 3.1.0. Because the merger function from ignition package is
	// version-agnostic and returns only interface{}, we need to hack our way into accessing
	// the content as a regular Config
	var report report.Report
	switch v := mergeResult.(type) {
	case config_latest_types.Config:
		if v.Ignition.Version == "3.1.0" {
			_, report, err = config_31.Parse(res)
		} else {
			_, report, err = config_latest.Parse(res)
		}
	default:
		return "", errors.Errorf("merged ignition config has invalid type: %T", v)
	}
	if err != nil {
		return "", err
	}
	if report.IsFatal() {
		return "", errors.Errorf("merged ignition config is invalid: %s", report.String())
	}

	return string(res), nil
}

func setEtcHostsInIgnition(role models.HostRole, path string, workDir string, content string) error {
	config, err := parseIgnitionFile(path)
	if err != nil {
		return err
	}

	setFileInIgnition(config, "/etc/hosts", dataurl.EncodeBytes([]byte(content)), true, 420, false)

	fileName := fmt.Sprintf("%s.ign", role)
	err = writeIgnitionFile(filepath.Join(workDir, fileName), config)
	if err != nil {
		return err
	}
	return nil
}

func GetServiceIPHostnames(serviceIPs string) string {
	ips := strings.Split(strings.TrimSpace(serviceIPs), ",")
	content := ""
	for _, ip := range ips {
		if ip != "" {
			content = content + fmt.Sprintf(ip+" assisted-api.local.openshift.io\n")
		}
	}
	return content
}

func firstN(s string, n int) string {
	const suffix string = " <TRUNCATED>"
	if len(s) > n+len(suffix) {
		return s[:(n-len(suffix))] + suffix
	}
	return s
}

func (g *installerGenerator) runCreateCommand(ctx context.Context, installerPath, command string, envVars []string) error {
	log := logutil.FromContext(ctx, g.log)
	cmd := exec.Command(installerPath, "create", command, "--dir", g.workDir)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	cmd.Env = envVars
	err := cmd.Run()
	if err != nil {
		log.WithError(err).
			Errorf("error running openshift-install create %s, stdout: %s", command, out.String())
		return errors.Wrapf(err, "error running openshift-install %s,  %s", command, firstN(out.String(), 512))
	}
	return nil
}

func (g *installerGenerator) downloadManifest(ctx context.Context, manifest string) error {
	respBody, _, err := g.s3Client.Download(ctx, manifest)
	if err != nil {
		return err
	}
	content, err := ioutil.ReadAll(respBody)
	if err != nil {
		return err
	}
	// manifest has full path as object-key on s3: clusterID/manifests/[manifests|openshift]/filename
	// clusterID/manifests should be trimmed
	prefix := manifests.GetManifestObjectName(*g.cluster.ID, "")
	targetPath := filepath.Join(g.workDir, strings.TrimPrefix(manifest, prefix))
	err = ioutil.WriteFile(targetPath, content, 0600)
	if err != nil {
		return err
	}
	return nil
}

func SetHostnameForNodeIgnition(ignition []byte, host *models.Host) ([]byte, error) {
	config, err := ParseToLatest(ignition)
	if err != nil {
		return nil, errors.Errorf("error parsing ignition: %v", err)
	}

	hostname, err := hostutil.GetCurrentHostName(host)
	if err != nil {
		return nil, errors.Errorf("failed to get hostname for host %s", host.ID)
	}

	setFileInIgnition(config, "/etc/hostname", fmt.Sprintf("data:,%s", hostname), false, 420, true)

	configBytes, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	return configBytes, nil
}

func (ib *ignitionBuilder) FormatDiscoveryIgnitionFile(ctx context.Context, infraEnv *common.InfraEnv, cfg IgnitionConfig, safeForLogs bool, authType auth.AuthType) (string, error) {
	pullSecretToken, err := clusterPkg.AgentToken(infraEnv, authType)
	if err != nil {
		return "", err
	}
	httpProxy, httpsProxy, noProxy := common.GetProxyConfigs(infraEnv.Proxy)
	proxySettings, err := proxySettingsForIgnition(httpProxy, httpsProxy, noProxy)
	if err != nil {
		return "", err
	}
	rhCa := ""
	if cfg.InstallRHCa {
		rhCa = url.PathEscape(RedhatRootCA)
	}
	userSshKey, err := getUserSSHKey(infraEnv.SSHAuthorizedKey)
	if err != nil {
		ib.log.WithError(err).Errorln("Unable to build user SSH public key JSON")
		return "", err
	}

	var ignitionParams = map[string]interface{}{
		"userSshKey":           userSshKey,
		"AgentDockerImg":       cfg.AgentDockerImg,
		"ServiceBaseURL":       strings.TrimSpace(cfg.ServiceBaseURL),
		"infraEnvId":           infraEnv.ID.String(),
		"PullSecretToken":      pullSecretToken,
		"AGENT_MOTD":           url.PathEscape(agentMessageOfTheDay),
		"AGENT_FIX_BZ1964591":  url.PathEscape(agentFixBZ1964591),
		"IPv6_CONF":            url.PathEscape(common.Ipv6DuidDiscoveryConf),
		"PULL_SECRET":          url.PathEscape(infraEnv.PullSecret),
		"RH_ROOT_CA":           rhCa,
		"PROXY_SETTINGS":       proxySettings,
		"HTTPProxy":            httpProxy,
		"HTTPSProxy":           httpsProxy,
		"NoProxy":              noProxy,
		"SkipCertVerification": strconv.FormatBool(cfg.SkipCertVerification),
		"AgentTimeoutStartSec": strconv.FormatInt(int64(cfg.AgentTimeoutStart.Seconds()), 10),
		"SELINUX_POLICY":       base64.StdEncoding.EncodeToString([]byte(selinuxPolicy)),
		"EnableAgentService":   infraEnv.InternalIgnitionConfigOverride == "",
	}
	if safeForLogs {
		for _, key := range []string{"userSshKey", "PullSecretToken", "PULL_SECRET", "RH_ROOT_CA"} {
			ignitionParams[key] = "*****"
		}
	}
	if cfg.ServiceCACertPath != "" {
		var caCertData []byte
		caCertData, err = ioutil.ReadFile(cfg.ServiceCACertPath)
		if err != nil {
			return "", err
		}
		ignitionParams["ServiceCACertData"] = dataurl.EncodeBytes(caCertData)
		ignitionParams["HostCACertPath"] = common.HostCACertPath
	}
	if cfg.ServiceIPs != "" {
		ignitionParams["ServiceIPs"] = dataurl.EncodeBytes([]byte(GetServiceIPHostnames(cfg.ServiceIPs)))
	}

	if infraEnv.StaticNetworkConfig != "" && common.ImageTypeValue(infraEnv.Type) == models.ImageTypeFullIso {
		filesList, newErr := ib.prepareStaticNetworkConfigForIgnition(ctx, infraEnv)
		if newErr != nil {
			ib.log.WithError(newErr).Errorf("Failed to add static network config to ignition for infra env  %s", infraEnv.ID)
			return "", newErr
		}
		ignitionParams["StaticNetworkConfig"] = filesList
		ignitionParams["PreNetworkConfigScript"] = base64.StdEncoding.EncodeToString([]byte(constants.PreNetworkConfigScript))
	}

	if ib.mirrorRegistriesBuilder.IsMirrorRegistriesConfigured() {
		caContents, mirrorsErr := ib.mirrorRegistriesBuilder.GetMirrorCA()
		if mirrorsErr != nil {
			ib.log.WithError(mirrorsErr).Errorf("Failed to get the mirror registries CA contents")
			return "", mirrorsErr
		}
		registriesContents, mirrorsErr := ib.mirrorRegistriesBuilder.GetMirrorRegistries()
		if mirrorsErr != nil {
			ib.log.WithError(mirrorsErr).Errorf("Failed to get the mirror registries config contents")
			return "", mirrorsErr
		}
		ignitionParams["MirrorRegistriesConfig"] = base64.StdEncoding.EncodeToString(registriesContents)
		ignitionParams["MirrorRegistriesCAConfig"] = base64.StdEncoding.EncodeToString(caContents)
	}

	if cfg.OKDRPMsImage != "" {
		okdBinariesOverlay := fmt.Sprintf(okdBinariesOverlayTemplate, cfg.OKDRPMsImage)
		ignitionParams["OKDBinaries"] = base64.StdEncoding.EncodeToString([]byte(okdBinariesOverlay))
		ignitionParams["OKDHoldPivot"] = base64.StdEncoding.EncodeToString([]byte(okdHoldPivot))
		ignitionParams["OKDHoldAgent"] = base64.StdEncoding.EncodeToString([]byte(okdHoldAgentUntilBinariesLanded))
	}
	tmpl, err := template.New("ignitionConfig").Parse(discoveryIgnitionConfigFormat)
	if err != nil {
		return "", err
	}
	buf := &bytes.Buffer{}
	if err = tmpl.Execute(buf, ignitionParams); err != nil {
		return "", err
	}

	res := buf.String()
	if infraEnv.InternalIgnitionConfigOverride != "" {
		res, err = MergeIgnitionConfig([]byte(res), []byte(infraEnv.InternalIgnitionConfigOverride))
		if err != nil {
			return "", err
		}
		ib.log.Infof("Applying internal ignition override %s for infra env %s, resulting ignition: %s", infraEnv.InternalIgnitionConfigOverride, infraEnv.ID, res)
	}

	if infraEnv.IgnitionConfigOverride != "" {
		res, err = MergeIgnitionConfig([]byte(res), []byte(infraEnv.IgnitionConfigOverride))
		if err != nil {
			return "", err
		}
		ib.log.Infof("Applying ignition override %s for infra env %s, resulting ignition: %s", infraEnv.IgnitionConfigOverride, infraEnv.ID, res)
	}

	return res, nil
}

func (ib *ignitionBuilder) prepareStaticNetworkConfigForIgnition(ctx context.Context, infraEnv *common.InfraEnv) ([]staticnetworkconfig.StaticNetworkConfigData, error) {
	filesList, err := ib.staticNetworkConfig.GenerateStaticNetworkConfigData(ctx, infraEnv.StaticNetworkConfig)
	if err != nil {
		ib.log.WithError(err).Errorf("staticNetworkGenerator failed to produce the static network connection files for cluster %s", infraEnv.ID)
		return nil, err
	}
	for i := range filesList {
		filesList[i].FilePath = filepath.Join(tempNMConnectionsDir, filesList[i].FilePath)
		filesList[i].FileContents = base64.StdEncoding.EncodeToString([]byte(filesList[i].FileContents))
	}

	return filesList, nil
}

func (ib *ignitionBuilder) FormatSecondDayWorkerIgnitionFile(url string, caCert *string, bearerToken string, host *models.Host) ([]byte, error) {
	var ignitionParams = map[string]interface{}{
		// https://github.com/openshift/machine-config-operator/blob/master/docs/MachineConfigServer.md#endpoint
		"SOURCE":  url,
		"HEADERS": map[string]string{},
		"CACERT":  "",
	}
	if bearerToken != "" {
		ignitionParams["HEADERS"].(map[string]string)["Authorization"] = fmt.Sprintf("Bearer %s", bearerToken)
	}

	if caCert != nil {
		ignitionParams["CACERT"] = fmt.Sprintf("data:text/plain;base64,%s", *caCert)
	}

	tmpl, err := template.New("nodeIgnition").Parse(secondDayWorkerIgnitionFormat)
	if err != nil {
		return nil, err
	}
	buf := &bytes.Buffer{}
	if err = tmpl.Execute(buf, ignitionParams); err != nil {
		return nil, err
	}

	overrides := buf.String()
	if host.IgnitionConfigOverrides != "" {
		overrides, err = MergeIgnitionConfig(buf.Bytes(), []byte(host.IgnitionConfigOverrides))
		if err != nil {
			return []byte(""), errors.Wrapf(err, "Failed to apply ignition override for host %s", host.ID)
		}
		ib.log.Infof("Applied ignition override for host %s", host.ID)
	}

	res, err := SetHostnameForNodeIgnition([]byte(overrides), host)
	if err != nil {
		return []byte(""), errors.Wrapf(err, "Failed to set hostname in ignition for host %s", host.ID)
	}

	return res, nil
}

func QuoteSshPublicKeys(sshPublicKeys string) string {
	return strings.ReplaceAll(sshPublicKeys, "\n", `", "`)
}

func getUserSSHKey(sshKey string) (string, error) {
	keys := buildUserSshKeysSlice(sshKey)
	if len(keys) == 0 {
		return "", nil
	}
	userAuthBlock := make(map[string]interface{})
	userAuthBlock["name"] = "core"
	userAuthBlock["passwordHash"] = "!"
	userAuthBlock["sshAuthorizedKeys"] = keys
	userAuthBlock["groups"] = [1]string{"sudo"}
	blockByte, err := json.Marshal(userAuthBlock)
	if err != nil {
		return "", fmt.Errorf("failed to build user ssh key block: %w", err)
	}
	return string(blockByte), nil
}

func buildUserSshKeysSlice(sshKey string) []string {
	keys := strings.Split(sshKey, "\n")
	validKeys := []string{}
	// filter only valid non empty keys
	for i := range keys {
		keys[i] = strings.TrimSpace(keys[i])
		if keys[i] != "" {
			validKeys = append(validKeys, keys[i])
		}
	}
	return validKeys
}

func proxySettingsForIgnition(httpProxy, httpsProxy, noProxy string) (string, error) {
	if httpProxy == "" && httpsProxy == "" {
		return "", nil
	}

	proxySettings := `"proxy": { {{.httpProxy}}{{.httpsProxy}}{{.noProxy}} }`
	var httpProxyAttr, httpsProxyAttr, noProxyAttr string
	if httpProxy != "" {
		httpProxyAttr = `"httpProxy": "` + httpProxy + `"`
	}
	if httpsProxy != "" {
		if httpProxy != "" {
			httpsProxyAttr = ", "
		}
		httpsProxyAttr += `"httpsProxy": "` + httpsProxy + `"`
	}
	if noProxy != "" {
		noProxyStr, err := json.Marshal(strings.Split(noProxy, ","))
		if err != nil {
			return "", err
		}
		noProxyAttr = `, "noProxy": ` + string(noProxyStr)
	}
	var proxyParams = map[string]string{
		"httpProxy":  httpProxyAttr,
		"httpsProxy": httpsProxyAttr,
		"noProxy":    noProxyAttr,
	}

	tmpl, err := template.New("proxySettings").Parse(proxySettings)
	if err != nil {
		return "", err
	}
	buf := &bytes.Buffer{}
	if err = tmpl.Execute(buf, proxyParams); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func getIcspFileFromInstallConfig(cfg []byte, log logrus.FieldLogger) (string, error) {
	contents, err := getIcsp(cfg)
	if err != nil {
		return "", err
	}
	if contents == nil {
		log.Infof("No ImageContentSources in install-config to build ICSP file")
		return "", nil
	}

	icspFile, err := ioutil.TempFile("", "icsp-file")
	if err != nil {
		return "", err
	}
	log.Infof("Building ICSP file from install-config with contents %s", contents)
	if _, err := icspFile.Write(contents); err != nil {
		icspFile.Close()
		os.Remove(icspFile.Name())
		return "", err
	}
	icspFile.Close()

	return icspFile.Name(), nil
}

func getIcsp(cfg []byte) ([]byte, error) {

	var installCfg installcfg.InstallerConfigBaremetal
	if err := yaml.Unmarshal(cfg, &installCfg); err != nil {
		return nil, err
	}

	if len(installCfg.ImageContentSources) == 0 {
		// No ImageContentSources were defined
		return nil, nil
	}

	icsp := operatorv1alpha1.ImageContentSourcePolicy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: operatorv1alpha1.SchemeGroupVersion.String(),
			Kind:       "ImageContentSourcePolicy",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "image-policy",
			// not namespaced
		},
	}

	icsp.Spec.RepositoryDigestMirrors = make([]operatorv1alpha1.RepositoryDigestMirrors, len(installCfg.ImageContentSources))
	for i, imageSource := range installCfg.ImageContentSources {
		icsp.Spec.RepositoryDigestMirrors[i] = operatorv1alpha1.RepositoryDigestMirrors{Source: imageSource.Source, Mirrors: imageSource.Mirrors}

	}

	// Convert to json first so json tags are handled
	jsonData, err := json.Marshal(&icsp)
	if err != nil {
		return nil, err
	}
	contents, err := k8syaml.JSONToYAML(jsonData)
	if err != nil {
		return nil, err
	}

	return contents, nil
}

func removeIcspFile(filename string) {
	if filename != "" {
		os.Remove(filename)
	}
}
