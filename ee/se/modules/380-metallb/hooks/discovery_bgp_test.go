/*
Copyright 2026 Flant JSC
Licensed under the Deckhouse Platform Enterprise Edition (EE) license.
See https://github.com/deckhouse/deckhouse/blob/main/ee/LICENSE
*/

package hooks

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/deckhouse/deckhouse/testing/hooks"
)

var _ = Describe("Modules :: metallb :: hooks :: discovery_bgp ::", func() {
	f := HookExecutionConfigInit(`{"metallb":{"internal": {}}}`, "")
	f.RegisterCRD("network.deckhouse.io", "v1alpha1", "MetalLoadBalancerPool", false)
	f.RegisterCRD("network.deckhouse.io", "v1alpha1", "MetalLoadBalancerBGPPeer", false)
	f.RegisterCRD("network.deckhouse.io", "v1alpha1", "MetalLoadBalancerConfiguration", false)

	Context("Empty cluster", func() {
		BeforeEach(func() {
			f.BindingContexts.Set(f.KubeStateSet(``))
			f.RunHook()
		})

		It("Should execute successfully and set empty arrays with default affinity", func() {
			Expect(f).To(ExecuteSuccessfully())
			Expect(f.ValuesGet("metallb.internal.addressPools").String()).To(MatchJSON(`[]`))
			Expect(f.ValuesGet("metallb.internal.bgpPeers").String()).To(MatchJSON(`[]`))
			Expect(f.ValuesGet("metallb.internal.bgpAdvertisements").String()).To(MatchJSON(`[]`))
			Expect(f.ValuesGet("metallb.internal.bfdProfiles").String()).To(MatchJSON(`[]`))
			Expect(f.ValuesGet("metallb.internal.secretsToCopy").String()).To(MatchJSON(`[]`))

			// Check default affinity
			Expect(f.ValuesGet("metallb.internal.speakerNodeAffinity").String()).To(MatchJSON(`{}`))
		})
	})

	Context("With pools, peers, configuration and secrets", func() {
		BeforeEach(func() {
			f.BindingContexts.Set(f.KubeStateSet(`
---
apiVersion: v1
kind: Secret
metadata:
  labels:
    network.deckhouse.io/metallb-bgp-password: "true"
  name: secret1
  namespace: ns1
data:
  password: cGFzc3dvcmQ= # "password"
---
apiVersion: network.deckhouse.io/v1alpha1
kind: MetalLoadBalancerPool
metadata:
  labels:
    network.deckhouse.io/metallb-bgp-password: "true"
  name: test-pool
spec:
  addresses:
  - 10.0.0.1-10.0.0.10
---
apiVersion: network.deckhouse.io/v1alpha1
kind: MetalLoadBalancerBGPPeer
metadata:
  labels:
    network.deckhouse.io/metallb-bgp-password: "true"
  name: test-peer
spec:
  peerAddress: 192.168.1.1
  peerASN: 65001
  myASN: 65000
  passwordSecretRef:
    name: secret1
    namespace: ns1
  bfd:
    receiveInterval: 300
    transmitInterval: 300
  sourceAddresses:
  - nodeName: node-1
    address: 10.10.10.1
---
apiVersion: network.deckhouse.io/v1alpha1
kind: MetalLoadBalancerConfiguration
metadata:
  labels:
    network.deckhouse.io/metallb-bgp-password: "true"
  name: test-config
spec:
  mode: BGP
  nodeSelector:
    role: worker
  bgp:
    peerNames:
    - test-peer
  advertisements:
  - poolNames:
    - test-pool
    bgp:
      localPref: 100
      communities:
      - "1111:2222"
`))
			f.RunHook()
		})

		It("Should generate correct internal values with secret data", func() {
			Expect(f).To(ExecuteSuccessfully())

			// Check Pools
			Expect(f.ValuesGet("metallb.internal.addressPools").String()).To(MatchJSON(`
[
  {
    "name": "test-pool",
    "addresses": ["10.0.0.1-10.0.0.10"]
  }
]`))

			// Check Peers (1 specific + 1 fallback)
			peers := f.ValuesGet("metallb.internal.bgpPeers").Array()
			Expect(len(peers)).To(Equal(2))

			// Specific Peer: test-peer-node-node-1
			Expect(f.ValuesGet("metallb.internal.bgpPeers.0.name").String()).To(Equal("test-peer-node-node-1"))
			Expect(f.ValuesGet("metallb.internal.bgpPeers.0.sourceAddress").String()).To(Equal("10.10.10.1"))
			Expect(f.ValuesGet("metallb.internal.bgpPeers.0.passwordSecret").String()).To(Equal("bgp-pwd-ns1-secret1"))

			// Fallback Peer: test-peer-test-config
			Expect(f.ValuesGet("metallb.internal.bgpPeers.1.name").String()).To(Equal("test-peer-test-config"))
			Expect(f.ValuesGet("metallb.internal.bgpPeers.1.nodeSelectors.0.matchLabels.role").String()).To(Equal("worker"))

			// Check Advertisements
			Expect(f.ValuesGet("metallb.internal.bgpAdvertisements.0.name").String()).To(Equal("test-config-adv-0"))
			Expect(f.ValuesGet("metallb.internal.bgpAdvertisements.0.ipAddressPools").String()).To(MatchJSON(`["test-pool"]`))

			// Check BFD Profile
			Expect(f.ValuesGet("metallb.internal.bfdProfiles.0.name").String()).To(Equal("bfd-test-peer"))
			Expect(f.ValuesGet("metallb.internal.bfdProfiles.0.receiveInterval").Int()).To(Equal(int64(300)))

			// Check Secrets to Copy (with actual data!)
			Expect(f.ValuesGet("metallb.internal.secretsToCopy.0.name").String()).To(Equal("bgp-pwd-ns1-secret1"))
			Expect(f.ValuesGet("metallb.internal.secretsToCopy.0.data.password").String()).To(Equal("password"))

			// Check Dynamic Affinity (should be role=worker instead of default)
			Expect(f.ValuesGet("metallb.internal.speakerNodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms.0.matchExpressions.0.key").String()).To(Equal("role"))
		})
	})

	Context("Sorting stability", func() {
		BeforeEach(func() {
			f.BindingContexts.Set(f.KubeStateSet(`
---
apiVersion: network.deckhouse.io/v1alpha1
kind: MetalLoadBalancerPool
metadata:
  labels:
    network.deckhouse.io/metallb-bgp-password: "true"
  name: z-pool
spec:
  addresses: ["1.1.1.1/32"]
---
apiVersion: network.deckhouse.io/v1alpha1
kind: MetalLoadBalancerPool
metadata:
  labels:
    network.deckhouse.io/metallb-bgp-password: "true"
  name: a-pool
spec:
  addresses: ["2.2.2.2/32"]
`))
			f.RunHook()
		})

		It("Should always sort outputs by name", func() {
			Expect(f).To(ExecuteSuccessfully())
			Expect(f.ValuesGet("metallb.internal.addressPools.0.name").String()).To(Equal("a-pool"))
			Expect(f.ValuesGet("metallb.internal.addressPools.1.name").String()).To(Equal("z-pool"))
		})
	})
})
