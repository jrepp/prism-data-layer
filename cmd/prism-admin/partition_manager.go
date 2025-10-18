package main

import (
	"fmt"
	"hash/crc32"
	"sort"
	"sync"

	adminpb "github.com/jrepp/prism-data-layer/pkg/plugin/gen/prism/admin"
	pb "github.com/jrepp/prism-data-layer/pkg/plugin/gen/prism"
)

// PartitionManager handles partition distribution across proxies.
// Partition assignment is computed on-demand from proxy set (NOT stored in Raft FSM).
// This reduces Raft log entries by ~50% as per RFC-038.
//
// Design: Uses consistent hashing with 256 partitions (0-255).
// Each namespace hashes to a partition via CRC32 % 256.
// Partitions are distributed evenly across proxies using round-robin.
type PartitionManager struct {
	mu sync.RWMutex
}

// NewPartitionManager creates a new partition manager
func NewPartitionManager() *PartitionManager {
	return &PartitionManager{}
}

// HashNamespace calculates partition ID for a namespace using consistent hashing
// Returns partition ID in range [0, 255]
func (pm *PartitionManager) HashNamespace(namespace string) int32 {
	hash := crc32.ChecksumIEEE([]byte(namespace))
	return int32(hash % 256) // 256 partitions
}

// ComputeRangesFromProxySet computes partition ranges for a specific proxy
// given the current set of all proxies in the cluster.
//
// Algorithm: Round-robin distribution based on sorted proxy IDs.
// Example with 3 proxies:
//   proxy-01: [0-85]
//   proxy-02: [86-170]
//   proxy-03: [171-255]
//
// This is deterministic - same proxy set always produces same ranges.
func (pm *PartitionManager) ComputeRangesFromProxySet(
	targetProxyID string,
	allProxies []*adminpb.ProxyEntry,
) []*pb.PartitionRange {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	if len(allProxies) == 0 {
		return []*pb.PartitionRange{}
	}

	// Sort proxies by ID for deterministic ordering
	proxyIDs := make([]string, len(allProxies))
	for i, p := range allProxies {
		proxyIDs[i] = p.ProxyId
	}
	sort.Strings(proxyIDs)

	// Find target proxy's index
	targetIndex := -1
	for i, id := range proxyIDs {
		if id == targetProxyID {
			targetIndex = i
			break
		}
	}

	if targetIndex == -1 {
		// Proxy not found in set
		return []*pb.PartitionRange{}
	}

	// Calculate range size
	proxyCount := len(proxyIDs)
	rangeSize := 256 / proxyCount

	// Calculate start/end for this proxy
	start := int32(targetIndex * rangeSize)
	end := int32(start + int32(rangeSize) - 1)

	// Last proxy gets remaining partitions
	if targetIndex == proxyCount-1 {
		end = 255
	}

	return []*pb.PartitionRange{{
		Start: start,
		End:   end,
	}}
}

// GetProxyForPartitionFromSet determines which proxy handles a partition
// given the current set of all proxies.
//
// This is the inverse of ComputeRangesFromProxySet - given a partition ID,
// find which proxy's range contains it.
func (pm *PartitionManager) GetProxyForPartitionFromSet(
	partitionID int32,
	allProxies []*adminpb.ProxyEntry,
) (string, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	if len(allProxies) == 0 {
		return "", fmt.Errorf("no proxies available")
	}

	if partitionID < 0 || partitionID > 255 {
		return "", fmt.Errorf("invalid partition ID: %d (must be 0-255)", partitionID)
	}

	// Sort proxies by ID for deterministic ordering
	proxyIDs := make([]string, len(allProxies))
	for i, p := range allProxies {
		proxyIDs[i] = p.ProxyId
	}
	sort.Strings(proxyIDs)

	// Calculate which proxy owns this partition
	proxyCount := len(proxyIDs)
	rangeSize := 256 / proxyCount

	// Determine proxy index from partition
	proxyIndex := int(partitionID) / rangeSize

	// Handle edge case for last proxy (gets remaining partitions)
	if proxyIndex >= proxyCount {
		proxyIndex = proxyCount - 1
	}

	return proxyIDs[proxyIndex], nil
}

// ComputeAllRanges computes partition ranges for all proxies in the set.
// Returns map of proxy_id → ranges.
//
// Useful for debugging and displaying cluster partition distribution.
func (pm *PartitionManager) ComputeAllRanges(
	allProxies []*adminpb.ProxyEntry,
) map[string][]*pb.PartitionRange {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	result := make(map[string][]*pb.PartitionRange)

	if len(allProxies) == 0 {
		return result
	}

	// Sort proxies by ID
	proxyIDs := make([]string, len(allProxies))
	for i, p := range allProxies {
		proxyIDs[i] = p.ProxyId
	}
	sort.Strings(proxyIDs)

	// Compute ranges for each proxy
	proxyCount := len(proxyIDs)
	rangeSize := 256 / proxyCount

	for i, proxyID := range proxyIDs {
		start := int32(i * rangeSize)
		end := int32(start + int32(rangeSize) - 1)

		// Last proxy gets remaining partitions
		if i == proxyCount-1 {
			end = 255
		}

		result[proxyID] = []*pb.PartitionRange{{
			Start: start,
			End:   end,
		}}
	}

	return result
}

// RebalanceOnProxyJoin computes new partition distribution when a proxy joins.
// Returns map of proxy_id → new ranges showing what changed.
//
// This is used to understand rebalancing impact but doesn't actually perform it.
// Actual rebalancing happens naturally as partition assignment is computed from proxy set.
func (pm *PartitionManager) RebalanceOnProxyJoin(
	existingProxies []*adminpb.ProxyEntry,
	newProxyID string,
) (oldRanges, newRanges map[string][]*pb.PartitionRange) {
	// Compute ranges with existing proxies
	oldRanges = pm.ComputeAllRanges(existingProxies)

	// Create new proxy entry
	newProxy := &adminpb.ProxyEntry{
		ProxyId: newProxyID,
		Status:  "healthy",
	}

	// Compute ranges with new proxy added
	allProxies := append(existingProxies, newProxy)
	newRanges = pm.ComputeAllRanges(allProxies)

	return oldRanges, newRanges
}

// RebalanceOnProxyLeave computes new partition distribution when a proxy leaves.
// Returns map showing how partitions redistribute.
func (pm *PartitionManager) RebalanceOnProxyLeave(
	allProxies []*adminpb.ProxyEntry,
	leavingProxyID string,
) (oldRanges, newRanges map[string][]*pb.PartitionRange) {
	// Compute ranges with all proxies
	oldRanges = pm.ComputeAllRanges(allProxies)

	// Filter out leaving proxy
	remainingProxies := make([]*adminpb.ProxyEntry, 0, len(allProxies)-1)
	for _, p := range allProxies {
		if p.ProxyId != leavingProxyID {
			remainingProxies = append(remainingProxies, p)
		}
	}

	// Compute ranges with remaining proxies
	newRanges = pm.ComputeAllRanges(remainingProxies)

	return oldRanges, newRanges
}

// GetPartitionCount returns total number of partitions (always 256)
func (pm *PartitionManager) GetPartitionCount() int32 {
	return 256
}

// GetNamespacesInPartition returns namespaces that hash to a specific partition.
// This requires checking all namespaces - use sparingly.
func (pm *PartitionManager) GetNamespacesInPartition(
	partitionID int32,
	allNamespaces []*adminpb.NamespaceEntry,
) []*adminpb.NamespaceEntry {
	result := make([]*adminpb.NamespaceEntry, 0)

	for _, ns := range allNamespaces {
		if ns.PartitionId == partitionID {
			result = append(result, ns)
		}
	}

	return result
}

// ValidatePartitionCoverage checks that all partitions [0-255] are covered
// by proxy ranges with no gaps or overlaps.
//
// Returns error if coverage is invalid.
func (pm *PartitionManager) ValidatePartitionCoverage(
	allProxies []*adminpb.ProxyEntry,
) error {
	if len(allProxies) == 0 {
		return fmt.Errorf("no proxies available")
	}

	ranges := pm.ComputeAllRanges(allProxies)

	// Build coverage bitmap
	covered := make([]bool, 256)

	for proxyID, proxyRanges := range ranges {
		for _, r := range proxyRanges {
			for i := r.Start; i <= r.End; i++ {
				if covered[i] {
					return fmt.Errorf("partition %d covered by multiple proxies (including %s)",
						i, proxyID)
				}
				covered[i] = true
			}
		}
	}

	// Check all partitions covered
	for i := 0; i < 256; i++ {
		if !covered[i] {
			return fmt.Errorf("partition %d not covered by any proxy", i)
		}
	}

	return nil
}
