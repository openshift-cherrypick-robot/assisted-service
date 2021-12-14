package subsystem

import (
	"context"
	"encoding/json"
	"time"

	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/openshift/assisted-service/client/installer"
	"github.com/openshift/assisted-service/internal/common"
	"github.com/openshift/assisted-service/internal/host/hostutil"
	"github.com/openshift/assisted-service/models"
)

var _ = Describe("[V2ClusterTests]", func() {
	ctx := context.Background()
	var clusterID strfmt.UUID
	var infraEnvID strfmt.UUID
	var boundInfraEnv strfmt.UUID
	var ips []string
	var h1, h2, h3 *models.Host

	AfterEach(func() {
		clearDB()
	})

	BeforeEach(func() {
		clusterReq, err := userBMClient.Installer.V2RegisterCluster(ctx, &installer.V2RegisterClusterParams{
			NewClusterParams: &models.ClusterCreateParams{
				Name:                     swag.String("test-cluster"),
				OpenshiftVersion:         swag.String(openshiftVersion),
				PullSecret:               swag.String(pullSecret),
				BaseDNSDomain:            "example.com",
				ClusterNetworkHostPrefix: 23,
			},
		})

		Expect(err).NotTo(HaveOccurred())
		clusterID = *clusterReq.GetPayload().ID

		//standalone infraEnv
		infraEnv := registerInfraEnv(nil)
		infraEnvID = *infraEnv.ID

		//bound infraEnv
		infraEnv = registerInfraEnv(&clusterID)
		boundInfraEnv = *infraEnv.ID

		By("register h2 h3 to cluster via the bound infraEnv")
		ips = hostutil.GenerateIPv4Addresses(3, defaultCIDRv4)
		h2 = registerNode(ctx, boundInfraEnv, "h2", ips[1])
		h3 = registerNode(ctx, boundInfraEnv, "h3", ips[2])
		v2UpdateVipParams(ctx, clusterID)
		waitForClusterState(ctx, clusterID, models.ClusterStatusInsufficient, defaultWaitForClusterStateTimeout,
			IgnoreStateInfo)
	})

	It("Bind/Unbind single host in 3 nodes cluster (standalone InfraEnv)", func() {
		//register node to the InfraEnv and get its inventory
		By("register h1 with the stand-alone InfraEnv")
		h1 = &registerHost(infraEnvID).Host
		generateHWPostStepReply(ctx, h1, getDefaultInventory(ips[0]), "h1")
		waitForHostStateV2(ctx, models.HostStatusKnownUnbound, defaultWaitForHostStateTimeout, h1)

		//bind the 3rd node and re-register it
		By("bind h1 to cluster")
		bindHost(infraEnvID, *h1.ID, clusterID)
		waitForHostStateV2(ctx, models.HostStatusBinding, defaultWaitForHostStateTimeout, h1)

		By("register h1 again and define the connectivity to the other hosts")
		h1 = &registerHostByUUID(h1.InfraEnvID, *h1.ID).Host

		generateEssentialHostSteps(ctx, h1, "h1", ips[0])
		generateFullMeshConnectivity(ctx, ips[0], h1, h2, h3)
		waitForHostStateV2(ctx, models.HostStatusKnown, defaultWaitForHostStateTimeout, h1)

		By("cluster is ready")
		generateEssentialPrepareForInstallationSteps(ctx, h1)
		waitForClusterState(ctx, clusterID, models.ClusterStatusReady, defaultWaitForClusterStateTimeout,
			IgnoreStateInfo)

		By("verify host name is set")
		h1 = getHostV2(infraEnvID, *h1.ID)
		Expect(h1.RequestedHostname).To(Equal("h1"))

		By("unbind host and re-register h1 --> cluster return to insufficient")
		unbindHost(infraEnvID, *h1.ID)
		h1 = &registerHost(infraEnvID).Host
		generateHWPostStepReply(ctx, h1, getDefaultInventory(ips[0]), "h1")
		waitForHostStateV2(ctx, models.HostStatusKnownUnbound, defaultWaitForHostStateTimeout, h1)
		waitForClusterState(ctx, clusterID, models.ClusterStatusInsufficient, defaultWaitForClusterStateTimeout,
			IgnoreStateInfo)

		By("verify that the unbound host still retains its name and disks count")
		h1 = getHostV2(infraEnvID, *h1.ID)
		Expect(h1.RequestedHostname).To(Equal(("h1")))
		var inventory models.Inventory
		_ = json.Unmarshal([]byte(h1.Inventory), &inventory)
		Expect(len(inventory.Disks)).To(Equal(2))
	})

	It("register single host in 3 nodes cluster (bound InfraEnv)", func() {
		//register node to the InfraEnv and get its inventory
		By("register h1 with the bound InfraEnv (implicit binding)")
		h1 = &registerHost(boundInfraEnv).Host
		host := getHostV2(boundInfraEnv, *h1.ID)
		Expect(host.ClusterID).NotTo(BeNil())

		generateHWPostStepReply(ctx, h1, getDefaultInventory(ips[0]), "h1")
		generateEssentialHostSteps(ctx, h1, "h1", ips[0])
		generateFullMeshConnectivity(ctx, ips[0], h1, h2, h3)
		waitForHostStateV2(ctx, models.HostStatusKnown, defaultWaitForHostStateTimeout, h1)

		By("cluster is ready")
		generateEssentialPrepareForInstallationSteps(ctx, h1)
		waitForClusterState(ctx, clusterID, models.ClusterStatusReady, defaultWaitForClusterStateTimeout,
			IgnoreStateInfo)

		By("unbind host should fail since infraEnv is bound to cluster")
		_, err := userBMClient.Installer.UnbindHost(ctx, &installer.UnbindHostParams{
			HostID:     *h1.ID,
			InfraEnvID: boundInfraEnv,
		})
		Expect(err).NotTo(BeNil())
	})

	It("Hosts unbinding on cluster delete", func() {
		//register node to the InfraEnv and get its inventory
		By("register h1 with InfraEnv")
		h1 = &registerHost(infraEnvID).Host
		generateHWPostStepReply(ctx, h1, getDefaultInventory(ips[0]), "h1")
		waitForHostStateV2(ctx, models.HostStatusKnownUnbound, defaultWaitForHostStateTimeout, h1)

		//bind the 3rd node and re-register it
		By("bind h1 to cluster")
		bindHost(infraEnvID, *h1.ID, clusterID)
		waitForHostStateV2(ctx, models.HostStatusBinding, defaultWaitForHostStateTimeout, h1)

		By("register h1 again and define the connectivity to the other hosts")
		h1 = &registerHostByUUID(h1.InfraEnvID, *h1.ID).Host

		generateEssentialHostSteps(ctx, h1, "h1", ips[0])
		generateFullMeshConnectivity(ctx, ips[0], h1, h2, h3)
		waitForHostStateV2(ctx, models.HostStatusKnown, defaultWaitForHostStateTimeout, h1)

		By("cluster is ready")
		generateEssentialPrepareForInstallationSteps(ctx, h1)
		waitForClusterState(ctx, clusterID, models.ClusterStatusReady, defaultWaitForClusterStateTimeout,
			IgnoreStateInfo)

		By("Delete Cluster")
		_, err := userBMClient.Installer.V2DeregisterCluster(ctx, &installer.V2DeregisterClusterParams{ClusterID: clusterID})
		Expect(err).NotTo(HaveOccurred())

		By("Wait for unbinding")
		waitForHostStateV2(ctx, models.HostStatusUnbinding, defaultWaitForHostStateTimeout, h1)

		By("Host is unbound")
		h1 = getHostV2(infraEnvID, *h1.ID)
		Expect(h1.ClusterID).To(BeNil())

		By("Other hosts are deleted")
		_, err = userBMClient.Installer.V2GetHost(context.Background(), &installer.V2GetHostParams{
			InfraEnvID: boundInfraEnv,
			HostID:     *h2.ID,
		})

		Expect(err).To(HaveOccurred())
		_, err = userBMClient.Installer.V2GetHost(context.Background(), &installer.V2GetHostParams{
			InfraEnvID: boundInfraEnv,
			HostID:     *h3.ID,
		})
		Expect(err).To(HaveOccurred())
	})

	It("Host unbinding pending user action on cluster delete", func() {
		//register node to the InfraEnv and get its inventory
		By("register h1 with InfraEnv")
		h1 = &registerHost(infraEnvID).Host
		generateHWPostStepReply(ctx, h1, getDefaultInventory(ips[0]), "h1")
		waitForHostStateV2(ctx, models.HostStatusKnownUnbound, defaultWaitForHostStateTimeout, h1)

		//bind the 3rd node and re-register it
		By("bind h1 to cluster")
		bindHost(infraEnvID, *h1.ID, clusterID)
		waitForHostStateV2(ctx, models.HostStatusBinding, defaultWaitForHostStateTimeout, h1)

		By("register h1 again and define the connectivity to the other hosts")
		h1 = &registerHostByUUID(h1.InfraEnvID, *h1.ID).Host

		generateEssentialHostSteps(ctx, h1, "h1", ips[0])
		generateFullMeshConnectivity(ctx, ips[0], h1, h2, h3)
		waitForHostStateV2(ctx, models.HostStatusKnown, defaultWaitForHostStateTimeout, h1)

		By("cluster is ready")
		generateEssentialPrepareForInstallationSteps(ctx, h1)
		waitForClusterState(ctx, clusterID, models.ClusterStatusReady, defaultWaitForClusterStateTimeout,
			IgnoreStateInfo)

		By("Start installation")
		_, err := userBMClient.Installer.V2InstallCluster(ctx, &installer.V2InstallClusterParams{ClusterID: clusterID})
		Expect(err).NotTo(HaveOccurred())
		generateEssentialPrepareForInstallationSteps(ctx, h1, h2, h3)
		waitForClusterState(context.Background(), clusterID, models.ClusterStatusInstalling,
			3*time.Minute, IgnoreStateInfo)

		By("Cancel installation")
		_, err = userBMClient.Installer.V2CancelInstallation(ctx, &installer.V2CancelInstallationParams{ClusterID: clusterID})
		Expect(err).NotTo(HaveOccurred())
		waitForClusterState(context.Background(), clusterID, models.ClusterStatusCancelled,
			3*time.Minute, IgnoreStateInfo)

		By("Delete Cluster")
		_, err = userBMClient.Installer.V2DeregisterCluster(ctx, &installer.V2DeregisterClusterParams{ClusterID: clusterID})
		Expect(err).NotTo(HaveOccurred())

		By("Wait for unbinding Pending User Action")
		waitForHostStateV2(ctx, models.HostStatusUnbindingPendingUserAction, defaultWaitForHostStateTimeout, h1)

		By("Host is unbound")
		h1 = getHostV2(infraEnvID, *h1.ID)
		Expect(h1.ClusterID).To(BeNil())

		By("Other hosts are deleted")
		_, err = userBMClient.Installer.V2GetHost(context.Background(), &installer.V2GetHostParams{
			InfraEnvID: boundInfraEnv,
			HostID:     *h2.ID,
		})

		Expect(err).To(HaveOccurred())
		_, err = userBMClient.Installer.V2GetHost(context.Background(), &installer.V2GetHostParams{
			InfraEnvID: boundInfraEnv,
			HostID:     *h3.ID,
		})
		Expect(err).To(HaveOccurred())

		By("register h1 again")
		h1 = &registerHostByUUID(h1.InfraEnvID, *h1.ID).Host
		generateHWPostStepReply(ctx, h1, getDefaultInventory(ips[0]), "h1")
		waitForHostStateV2(ctx, models.HostStatusKnownUnbound, defaultWaitForHostStateTimeout, h1)
	})

	It("Verify garbage collector inactive cluster and infraenv deregistration", func() {
		By("Update cluster's updated_at attribute to become eligible for deregistration due to inactivity")
		cluster := getCluster(clusterID)
		db.Model(&cluster).UpdateColumn("updated_at", time.Now().AddDate(-1, 0, 0))

		By("Fetch cluster to make sure it was permanently removed by the garbage collector")
		Eventually(func() error {
			_, err := common.GetClusterFromDBWhere(db, common.SkipEagerLoading, common.SkipDeletedRecords, "id = ?", clusterID)
			return err
		}, "1m", "10s").Should(HaveOccurred())

		By("Update infraEnv's updated_at attribute to become eligible for deregistration due to inactivity")
		infraEnv, err := common.GetInfraEnvFromDBWhere(db, "id = ?", boundInfraEnv)
		Expect(err).NotTo(HaveOccurred())
		db.Model(&infraEnv).UpdateColumn("updated_at", time.Now().AddDate(-1, 0, 0))

		By("Fetch bounded InfraEnv to make sure it was permanently removed by the garbage collector")
		Eventually(func() error {
			_, err = common.GetInfraEnvFromDBWhere(db, "id = ?", boundInfraEnv)
			return err
		}, "1m", "10s").Should(HaveOccurred())

		By("Verify that hosts are deleted")
		_, err = userBMClient.Installer.V2GetHost(context.Background(), &installer.V2GetHostParams{
			InfraEnvID: boundInfraEnv,
			HostID:     *h2.ID,
		})
		Expect(err).To(HaveOccurred())
		_, err = userBMClient.Installer.V2GetHost(context.Background(), &installer.V2GetHostParams{
			InfraEnvID: boundInfraEnv,
			HostID:     *h3.ID,
		})
		Expect(err).To(HaveOccurred())

		By("Verify that late-binding infraenv is not deleted")
		_, err = common.GetInfraEnvFromDBWhere(db, "id = ?", infraEnvID)
		Expect(err).NotTo(HaveOccurred())

		By("Update late-binding infraEnv's updated_at attribute to become eligible for deregistration due to inactivity")
		infraEnv, err = common.GetInfraEnvFromDBWhere(db, "id = ?", infraEnvID)
		Expect(err).NotTo(HaveOccurred())
		db.Model(&infraEnv).UpdateColumn("updated_at", time.Now().AddDate(-1, 0, 0))

		By("Fetch late-binding InfraEnv to make sure it was permanently removed by the garbage collector")
		Eventually(func() error {
			_, err := common.GetInfraEnvFromDBWhere(db, "id = ?", infraEnvID)
			return err
		}, "1m", "10s").Should(HaveOccurred())
	})
})
