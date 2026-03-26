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

		It("Should execute successfully and set empty arrays", func() {
			Expect(f).To(ExecuteSuccessfully())
			Expect(f.ValuesGet("metallb.internal.addressPools").String()).To(MatchJSON(`[]`))
			Expect(f.ValuesGet("metallb.internal.bgpPeers").String()).To(MatchJSON(`[]`))
			Expect(f.ValuesGet("metallb.internal.bgpAdvertisements").String()).To(MatchJSON(`[]`))
			Expect(f.ValuesGet("metallb.internal.bfdProfiles").String()).To(MatchJSON(`[]`))
			Expect(f.ValuesGet("metallb.internal.secretsToCopy").String()).To(MatchJSON(`[]`))
			Expect(f.ValuesGet("metallb.internal.speakerNodeAffinity").Exists()).To(BeFalse())
		})
	})

	Context("With pools, peers and configuration", func() {
		BeforeEach(func() {
			f.BindingContexts.Set(f.KubeStateSet(`
---
apiVersion: network.deckhouse.io/v1alpha1
kind: MetalLoadBalancerPool
metadata:
  name: test-pool
spec:
  addresses:
  - 10.0.0.1-10.0.0.10
---
apiVersion: network.deckhouse.io/v1alpha1
kind: MetalLoadBalancerBGPPeer
metadata:
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

		It("Should generate correct internal values", func() {
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

			// Check Specific Peer
			Expect(f.ValuesGet("metallb.internal.bgpPeers.0.name").String()).To(Equal("test-peer-node-node-1"))
			Expect(f.ValuesGet("metallb.internal.bgpPeers.0.sourceAddress").String()).To(Equal("10.10.10.1"))
			Expect(f.ValuesGet("metallb.internal.bgpPeers.0.nodeSelectors.0.matchLabels.kubernetes\\.io/hostname").String()).To(Equal("node-1"))

			// Check Fallback Peer
			Expect(f.ValuesGet("metallb.internal.bgpPeers.1.name").String()).To(Equal("test-peer-test-config"))
			Expect(f.ValuesGet("metallb.internal.bgpPeers.1.nodeSelectors.0.matchLabels.role").String()).To(Equal("worker"))
			Expect(f.ValuesGet("metallb.internal.bgpPeers.1.nodeSelectors.0.matchExpressions.0.values.0").String()).To(Equal("node-1"))

			// Check Advertisements
			Expect(f.ValuesGet("metallb.internal.bgpAdvertisements").String()).To(MatchJSON(`
[
  {
    "name": "test-config-adv-0",
    "ipAddressPools": ["test-pool"],
    "peers": ["test-peer"],
    "localPref": 100,
    "communities": ["1111:2222"],
    "nodeSelectors": [
      {
        "matchLabels": {
          "role": "worker"
        }
      }
    ]
  }
]`))

			// Check BFD and Secrets
			Expect(f.ValuesGet("metallb.internal.bfdProfiles.0.name").String()).To(Equal("bfd-test-peer"))
			Expect(f.ValuesGet("metallb.internal.secretsToCopy.0.name").String()).To(Equal("secret1"))

			// Check Affinity
			Expect(f.ValuesGet("metallb.internal.speakerNodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms.0.matchExpressions.0.key").String()).To(Equal("role"))
		})
	})
})
