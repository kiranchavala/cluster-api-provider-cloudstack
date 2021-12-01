/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cloud_test

import (
	"github.com/apache/cloudstack-go/v2/cloudstack"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	infrav1 "gitlab.aws.dev/ce-pike/merida/cluster-api-provider-capc/api/v1alpha4"
	"gitlab.aws.dev/ce-pike/merida/cluster-api-provider-capc/pkg/cloud"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
)

var _ = Describe("Network", func() {
	var ( // Declare shared vars.
		mockCtrl   *gomock.Controller
		mockClient *cloudstack.CloudStackClient
		ns         *cloudstack.MockNetworkServiceIface
		nos        *cloudstack.MockNetworkOfferingServiceIface
		fs         *cloudstack.MockFirewallServiceIface
		as         *cloudstack.MockAddressServiceIface
		lbs        *cloudstack.MockLoadBalancerServiceIface
		csCluster  *infrav1.CloudStackCluster
	)

	BeforeEach(func() {
		// Setup new mock services.
		mockCtrl = gomock.NewController(GinkgoT())
		mockClient = cloudstack.NewMockClient(mockCtrl)
		ns = mockClient.Network.(*cloudstack.MockNetworkServiceIface)
		nos = mockClient.NetworkOffering.(*cloudstack.MockNetworkOfferingServiceIface)
		fs = mockClient.Firewall.(*cloudstack.MockFirewallServiceIface)
		as = mockClient.Address.(*cloudstack.MockAddressServiceIface)
		lbs = mockClient.LoadBalancer.(*cloudstack.MockLoadBalancerServiceIface)

		// Reset csCluster.
		csCluster = &infrav1.CloudStackCluster{
			Spec: infrav1.CloudStackClusterSpec{
				Zone:                 "zone1",
				Network:              "fakeNetName",
				ControlPlaneEndpoint: clusterv1.APIEndpoint{Port: int32(6443)},
			},
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("for an existing network", func() {
		It("resolves network details in cluster status", func() {
			ns.EXPECT().GetNetworkID("fakeNetName").Return("fakeNetID", 1, nil)
			ns.EXPECT().GetNetworkByID("fakeNetID").Return(&cloudstack.Network{Type: "Isolated"}, 0, nil)
			Ω(cloud.ResolveNetwork(mockClient, csCluster)).Should(Succeed())
			Ω(csCluster.Status.NetworkID).Should(Equal("fakeNetID"))
			Ω(csCluster.Status.NetworkType).Should(Equal("Isolated"))
		})

		It("does not call to create a new network via GetOrCreateNetwork", func() {
			ns.EXPECT().GetNetworkID("fakeNetName").Return("fakeNetID", 1, nil)
			ns.EXPECT().GetNetworkByID("fakeNetID").Return(&cloudstack.Network{Type: "Isolated"}, 0, nil)
			Ω(cloud.GetOrCreateNetwork(mockClient, csCluster)).Should(Succeed())
		})
	})

	Context("for a non-existent network", func() {
		It("when GetOrCreateNetwork is called it calls CloudStack to create a network", func() {
			ns.EXPECT().GetNetworkID("fakeNetName").Return("", -1, errors.New("No match found for blah."))
			nos.EXPECT().GetNetworkOfferingID(gomock.Any()).Return("someOfferingID", 1, nil)
			ns.EXPECT().NewCreateNetworkParams(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(&cloudstack.CreateNetworkParams{})
			ns.EXPECT().CreateNetwork(gomock.Any()).Return(&cloudstack.CreateNetworkResponse{Id: "someNetID"}, nil)
			Ω(cloud.GetOrCreateNetwork(mockClient, csCluster)).Should(Succeed())
		})
	})

	Context("for a closed firewall", func() {
		It("OpenFirewallRule asks CloudStack to open the firewall", func() {
			netID := "someNetID"
			csCluster.Status.NetworkID = netID
			fs.EXPECT().NewCreateEgressFirewallRuleParams(netID, "tcp").
				Return(&cloudstack.CreateEgressFirewallRuleParams{})
			fs.EXPECT().CreateEgressFirewallRule(&cloudstack.CreateEgressFirewallRuleParams{}).
				Return(&cloudstack.CreateEgressFirewallRuleResponse{}, nil)

			Ω(cloud.OpenFirewallRules(mockClient, csCluster)).Should(Succeed())
		})
	})

	Context("for an open firewall", func() {
		It("OpenFirewallRule asks CloudStack to open the firewall anyway, but doesn't fail", func() {
			netID := "someNetID"
			csCluster.Status.NetworkID = netID
			fs.EXPECT().NewCreateEgressFirewallRuleParams(netID, "tcp").
				Return(&cloudstack.CreateEgressFirewallRuleParams{})
			fs.EXPECT().CreateEgressFirewallRule(&cloudstack.CreateEgressFirewallRuleParams{}).
				Return(&cloudstack.CreateEgressFirewallRuleResponse{}, errors.New("There is already a rule like this."))
			Ω(cloud.OpenFirewallRules(mockClient, csCluster)).Should(Succeed())
		})
	})

	Context("in an isolated network with public IPs available", func() {
		It("will resolve public IP details given an endpoint spec", func() {
			ipAddress := "192.168.1.14"
			as.EXPECT().NewListPublicIpAddressesParams().Return(&cloudstack.ListPublicIpAddressesParams{})
			as.EXPECT().ListPublicIpAddresses(gomock.Any()).
				Return(&cloudstack.ListPublicIpAddressesResponse{
					Count:             1,
					PublicIpAddresses: []*cloudstack.PublicIpAddress{{Id: "PublicIPID", Ipaddress: ipAddress}},
				}, nil)
			publicIpAddress, err := cloud.ResolvePublicIPDetails(mockClient, csCluster)
			Ω(err).Should(Succeed())
			Ω(publicIpAddress).ShouldNot(BeNil())
			Ω(publicIpAddress.Ipaddress).Should(Equal(ipAddress))
		})
	})

	Context("The specific load balancer rule does exist", func() {
		It("resolves the rule's ID", func() {
			lbs.EXPECT().NewListLoadBalancerRulesParams().Return(&cloudstack.ListLoadBalancerRulesParams{})
			lbs.EXPECT().ListLoadBalancerRules(gomock.Any()).Return(
				&cloudstack.ListLoadBalancerRulesResponse{
					LoadBalancerRules: []*cloudstack.LoadBalancerRule{{Publicport: "6443", Id: "lbRuleID"}}}, nil)
			Ω(cloud.ResolveLoadBalancerRuleDetails(mockClient, csCluster)).Should(Succeed())
			Ω(csCluster.Status.LBRuleID).Should(Equal("lbRuleID"))
		})

		It("doesn't create a new load blancer rule on create", func() {
			lbs.EXPECT().NewListLoadBalancerRulesParams().Return(&cloudstack.ListLoadBalancerRulesParams{})
			lbs.EXPECT().ListLoadBalancerRules(gomock.Any()).
				Return(&cloudstack.ListLoadBalancerRulesResponse{
					LoadBalancerRules: []*cloudstack.LoadBalancerRule{{Publicport: "6443", Id: "lbRuleID"}}}, nil)
			Ω(cloud.GetOrCreateLoadBalancerRule(mockClient, csCluster)).Should(Succeed())
			Ω(csCluster.Status.LBRuleID).Should(Equal("lbRuleID"))
		})
	})

	Context("load balancer rule does not exist", func() {
		It("calls cloudstack to create a new load balancer rule.", func() {
			lbs.EXPECT().NewListLoadBalancerRulesParams().Return(&cloudstack.ListLoadBalancerRulesParams{})
			lbs.EXPECT().ListLoadBalancerRules(gomock.Any()).Return(&cloudstack.ListLoadBalancerRulesResponse{
				LoadBalancerRules: []*cloudstack.LoadBalancerRule{{Publicport: "7443", Id: "lbRuleID"}}}, nil)
			lbs.EXPECT().NewCreateLoadBalancerRuleParams(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(&cloudstack.CreateLoadBalancerRuleParams{})
			lbs.EXPECT().CreateLoadBalancerRule(gomock.Any()).
				Return(&cloudstack.CreateLoadBalancerRuleResponse{Id: "randomID"}, nil)
			Ω(cloud.GetOrCreateLoadBalancerRule(mockClient, csCluster)).Should(Succeed())
			Ω(csCluster.Status.LBRuleID).Should(Equal("randomID"))
		})
	})
})
