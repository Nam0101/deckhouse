/*
Copyright 2026 Flant JSC
Licensed under the Deckhouse Platform Enterprise Edition (EE) license.
See https://github.com/deckhouse/deckhouse/blob/main/ee/LICENSE
*/

package hooks

import (
	"context"
	"fmt"
	"sort"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/flant/addon-operator/pkg/module_manager/go_hook"
	"github.com/flant/addon-operator/sdk"
)

var _ = sdk.RegisterFunc(&go_hook.HookConfig{
	Queue: "/modules/metallb/discovery_bgp",
	Kubernetes: []go_hook.KubernetesConfig{
		{
			Name:       "pools",
			ApiVersion: "network.deckhouse.io/v1alpha1",
			Kind:       "MetalLoadBalancerPool",
			FilterFunc: filterPool,
		},
		{
			Name:       "peers",
			ApiVersion: "network.deckhouse.io/v1alpha1",
			Kind:       "MetalLoadBalancerBGPPeer",
			FilterFunc: filterPeer,
		},
		{
			Name:       "configs",
			ApiVersion: "network.deckhouse.io/v1alpha1",
			Kind:       "MetalLoadBalancerConfiguration",
			FilterFunc: filterConfig,
		},
	},
}, handleBGP)

func filterPool(obj *unstructured.Unstructured) (go_hook.FilterResult, error) {
	var pool MetalLoadBalancerPool
	if err := sdk.FromUnstructured(obj, &pool); err != nil {
		return nil, err
	}
	return pool, nil
}

func filterPeer(obj *unstructured.Unstructured) (go_hook.FilterResult, error) {
	var peer MetalLoadBalancerBGPPeer
	if err := sdk.FromUnstructured(obj, &peer); err != nil {
		return nil, err
	}
	return peer, nil
}

func filterConfig(obj *unstructured.Unstructured) (go_hook.FilterResult, error) {
	var config MetalLoadBalancerConfiguration
	if err := sdk.FromUnstructured(obj, &config); err != nil {
		return nil, err
	}
	return config, nil
}

func handleBGP(_ context.Context, input *go_hook.HookInput) error {
	var pools []MetalLoadBalancerPool
	for _, p := range input.Snapshots.Get("pools") {
		var pool MetalLoadBalancerPool
		if err := p.UnmarshalTo(&pool); err == nil {
			pools = append(pools, pool)
		}
	}

	var peers []MetalLoadBalancerBGPPeer
	for _, p := range input.Snapshots.Get("peers") {
		var peer MetalLoadBalancerBGPPeer
		if err := p.UnmarshalTo(&peer); err == nil {
			peers = append(peers, peer)
		}
	}

	var configs []MetalLoadBalancerConfiguration
	for _, c := range input.Snapshots.Get("configs") {
		var config MetalLoadBalancerConfiguration
		if err := c.UnmarshalTo(&config); err == nil {
			configs = append(configs, config)
		}
	}

	peerMap := make(map[string]MetalLoadBalancerBGPPeer)
	for _, p := range peers {
		peerMap[p.Name] = p
	}

	outPools := make([]IPAddressPoolValue, 0)
	for _, pool := range pools {
		outPools = append(outPools, IPAddressPoolValue{
			Name:      pool.Name,
			Addresses: pool.Spec.Addresses,
		})
	}
	// Sort to keep Helm values stable
	sort.Slice(outPools, func(i, j int) bool { return outPools[i].Name < outPools[j].Name })

	outPeers := make([]BGPPeerValue, 0)
	outAdvs := make([]BGPAdvertisementValue, 0)
	outBFDs := make([]BFDProfileValue, 0)
	outSecrets := make([]SecretToCopy, 0)
	speakerNodeSelectorTerms := make([]v1.NodeSelectorTerm, 0)

	secretsSet := make(map[string]SecretToCopy)
	bfdSet := make(map[string]BFDProfileValue)

	for _, cfg := range configs {
		if cfg.Spec.Mode != "BGP" {
			continue
		}

		// Collect speaker node selector terms
		if len(cfg.Spec.NodeSelector) > 0 {
			var matchExpressions []v1.NodeSelectorRequirement
			// Ensure deterministic order for matchExpressions
			var keys []string
			for k := range cfg.Spec.NodeSelector {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				matchExpressions = append(matchExpressions, v1.NodeSelectorRequirement{
					Key:      k,
					Operator: v1.NodeSelectorOpIn,
					Values:   []string{cfg.Spec.NodeSelector[k]},
				})
			}
			speakerNodeSelectorTerms = append(speakerNodeSelectorTerms, v1.NodeSelectorTerm{
				MatchExpressions: matchExpressions,
			})
		}

		// Generate advertisements
		for i, adv := range cfg.Spec.Advertisements {
			outAdv := BGPAdvertisementValue{
				Name:              fmt.Sprintf("%s-adv-%d", cfg.Name, i),
				IPAddressPools:    adv.PoolNames,
				Peers:             cfg.Spec.BGP.PeerNames,
				Communities:       adv.BGP.Communities,
				LocalPref:         adv.BGP.LocalPref,
				AggregationLength: adv.BGP.AggregationLength,
			}
			if len(cfg.Spec.NodeSelector) > 0 {
				outAdv.NodeSelectors = []metav1.LabelSelector{
					{MatchLabels: cfg.Spec.NodeSelector},
				}
			}
			outAdvs = append(outAdvs, outAdv)
		}

		// Generate peers
		for _, peerName := range cfg.Spec.BGP.PeerNames {
			peer, ok := peerMap[peerName]
			if !ok {
				continue
			}

			// Extract secret if present
			var secretName string
			if peer.Spec.PasswordSecretRef != nil {
				s := *peer.Spec.PasswordSecretRef
				secretName = fmt.Sprintf("bgp-pwd-%s-%s", s.Namespace, s.Name)
				secretsSet[secretName] = SecretToCopy(s)
			}

			// Extract BFD if present
			var bfdName string
			if peer.Spec.BFD != nil {
				bfdName = fmt.Sprintf("bfd-%s", peer.Name)
				bfdSet[bfdName] = BFDProfileValue{
					Name:             bfdName,
					ReceiveInterval:  peer.Spec.BFD.ReceiveInterval,
					TransmitInterval: peer.Spec.BFD.TransmitInterval,
					DetectMultiplier: peer.Spec.BFD.DetectMultiplier,
					EchoInterval:     peer.Spec.BFD.EchoInterval,
					EchoMode:         peer.Spec.BFD.EchoMode,
					PassiveMode:      peer.Spec.BFD.PassiveMode,
					MinimumTtl:       peer.Spec.BFD.MinimumTtl,
				}
			}

			var explicitNodes []string
			for _, sa := range peer.Spec.SourceAddresses {
				explicitNodes = append(explicitNodes, sa.NodeName)

				outPeers = append(outPeers, BGPPeerValue{
					Name:           fmt.Sprintf("%s-node-%s", peer.Name, sa.NodeName),
					MyASN:          peer.Spec.MyASN,
					PeerASN:        peer.Spec.PeerASN,
					PeerAddress:    peer.Spec.PeerAddress,
					RouterID:       peer.Spec.RouterID,
					PeerPort:       peer.Spec.PeerPort,
					HoldTime:       peer.Spec.HoldTime,
					SourceAddress:  sa.Address,
					PasswordSecret: secretName,
					BFDProfile:     bfdName,
					NodeSelectors: []metav1.LabelSelector{
						{
							MatchLabels: map[string]string{
								"kubernetes.io/hostname": sa.NodeName,
							},
						},
					},
				})

				speakerNodeSelectorTerms = append(speakerNodeSelectorTerms, v1.NodeSelectorTerm{
					MatchExpressions: []v1.NodeSelectorRequirement{
						{
							Key:      "kubernetes.io/hostname",
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{sa.NodeName},
						},
					},
				})
			}

			var fallbackNodeSelectors []metav1.LabelSelector
			if len(cfg.Spec.NodeSelector) > 0 || len(explicitNodes) > 0 {
				ls := metav1.LabelSelector{
					MatchLabels: cfg.Spec.NodeSelector,
				}
				if len(explicitNodes) > 0 {
					// Ensure deterministic order
					sort.Strings(explicitNodes)
					ls.MatchExpressions = []metav1.LabelSelectorRequirement{
						{
							Key:      "kubernetes.io/hostname",
							Operator: metav1.LabelSelectorOpNotIn,
							Values:   explicitNodes,
						},
					}
				}
				fallbackNodeSelectors = append(fallbackNodeSelectors, ls)
			}

			fallbackPeerName := fmt.Sprintf("%s-%s", peer.Name, cfg.Name)
			outPeers = append(outPeers, BGPPeerValue{
				Name:           fallbackPeerName,
				MyASN:          peer.Spec.MyASN,
				PeerASN:        peer.Spec.PeerASN,
				PeerAddress:    peer.Spec.PeerAddress,
				RouterID:       peer.Spec.RouterID,
				PeerPort:       peer.Spec.PeerPort,
				HoldTime:       peer.Spec.HoldTime,
				PasswordSecret: secretName,
				BFDProfile:     bfdName,
				NodeSelectors:  fallbackNodeSelectors,
			})
		}
	}

	for _, v := range secretsSet {
		outSecrets = append(outSecrets, v)
	}
	sort.Slice(outSecrets, func(i, j int) bool { return outSecrets[i].Name < outSecrets[j].Name })

	for _, v := range bfdSet {
		outBFDs = append(outBFDs, v)
	}
	sort.Slice(outBFDs, func(i, j int) bool { return outBFDs[i].Name < outBFDs[j].Name })

	// Deduplicate peers by Name, but in this logic they should be mostly unique.
	// Actually, a peer might be referenced in multiple configs,
	// which means multiple fallback peers with different names.
	// That's correct and expected.
	sort.Slice(outPeers, func(i, j int) bool { return outPeers[i].Name < outPeers[j].Name })

	sort.Slice(outAdvs, func(i, j int) bool { return outAdvs[i].Name < outAdvs[j].Name })

	input.Values.Set("metallb.internal.addressPools", outPools)
	input.Values.Set("metallb.internal.bgpPeers", outPeers)
	input.Values.Set("metallb.internal.bgpAdvertisements", outAdvs)
	input.Values.Set("metallb.internal.bfdProfiles", outBFDs)
	input.Values.Set("metallb.internal.secretsToCopy", outSecrets)

	if len(speakerNodeSelectorTerms) > 0 {
		input.Values.Set("metallb.internal.speakerNodeAffinity", map[string]any{
			"requiredDuringSchedulingIgnoredDuringExecution": map[string]any{
				"nodeSelectorTerms": speakerNodeSelectorTerms,
			},
		})
	} else {
		input.Values.Remove("metallb.internal.speakerNodeAffinity")
	}

	return nil
}
