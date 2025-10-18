package main

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// AdminMetrics tracks Raft and FSM metrics for observability
type AdminMetrics struct {
	// Raft state metrics
	raftState prometheus.Gauge
	raftTerm  prometheus.Gauge
	raftIndex prometheus.Gauge

	// Leader election metrics
	leaderElections prometheus.Counter
	leaderChanges   prometheus.Counter
	leadershipLost  prometheus.Counter

	// FSM operation metrics
	fsmCommandsTotal   *prometheus.CounterVec
	fsmCommandDuration *prometheus.HistogramVec
	fsmSnapshotsTotal  prometheus.Counter
	fsmRestoresTotal   prometheus.Counter

	// Raft proposal metrics
	raftProposalsTotal   prometheus.Counter
	raftProposalsFailed  prometheus.Counter
	raftProposalDuration prometheus.Histogram

	// Cluster health metrics
	clusterSize     prometheus.Gauge
	healthyPeers    prometheus.Gauge
	proxiesTotal    prometheus.Gauge
	launchersTotal  prometheus.Gauge
	namespacesTotal prometheus.Gauge
	patternsTotal   prometheus.Gauge

	// Follower forwarding metrics
	forwardedRequests *prometheus.CounterVec
}

// NewAdminMetrics creates and registers Prometheus metrics
func NewAdminMetrics(namespace string) *AdminMetrics {
	m := &AdminMetrics{
		// Raft state
		raftState: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "raft",
			Name:      "state",
			Help:      "Current Raft node state (0=Follower, 1=Candidate, 2=Leader, 3=Shutdown)",
		}),
		raftTerm: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "raft",
			Name:      "term",
			Help:      "Current Raft term",
		}),
		raftIndex: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "raft",
			Name:      "last_index",
			Help:      "Last applied Raft log index",
		}),

		// Leader election
		leaderElections: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "raft",
			Name:      "leader_elections_total",
			Help:      "Total number of leader elections",
		}),
		leaderChanges: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "raft",
			Name:      "leader_changes_total",
			Help:      "Total number of leader changes",
		}),
		leadershipLost: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "raft",
			Name:      "leadership_lost_total",
			Help:      "Total number of times leadership was lost",
		}),

		// FSM operations
		fsmCommandsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "fsm",
				Name:      "commands_total",
				Help:      "Total number of FSM commands by type and status",
			},
			[]string{"command_type", "status"},
		),
		fsmCommandDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: "fsm",
				Name:      "command_duration_seconds",
				Help:      "FSM command execution duration in seconds",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"command_type"},
		),
		fsmSnapshotsTotal: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "fsm",
			Name:      "snapshots_total",
			Help:      "Total number of FSM snapshots created",
		}),
		fsmRestoresTotal: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "fsm",
			Name:      "restores_total",
			Help:      "Total number of FSM restores from snapshot",
		}),

		// Raft proposals
		raftProposalsTotal: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "raft",
			Name:      "proposals_total",
			Help:      "Total number of Raft proposals",
		}),
		raftProposalsFailed: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "raft",
			Name:      "proposals_failed_total",
			Help:      "Total number of failed Raft proposals",
		}),
		raftProposalDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "raft",
			Name:      "proposal_duration_seconds",
			Help:      "Raft proposal duration in seconds",
			Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
		}),

		// Cluster health
		clusterSize: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "cluster",
			Name:      "size",
			Help:      "Total number of nodes in Raft cluster",
		}),
		healthyPeers: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "cluster",
			Name:      "healthy_peers",
			Help:      "Number of healthy peers (excluding self)",
		}),
		proxiesTotal: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "admin",
			Name:      "proxies_total",
			Help:      "Total number of registered proxies",
		}),
		launchersTotal: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "admin",
			Name:      "launchers_total",
			Help:      "Total number of registered launchers",
		}),
		namespacesTotal: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "admin",
			Name:      "namespaces_total",
			Help:      "Total number of namespaces",
		}),
		patternsTotal: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "admin",
			Name:      "patterns_total",
			Help:      "Total number of assigned patterns",
		}),

		// Follower forwarding
		forwardedRequests: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "admin",
				Name:      "forwarded_requests_total",
				Help:      "Total number of requests forwarded to leader",
			},
			[]string{"rpc_method", "status"},
		),
	}

	return m
}

// RecordRaftState updates Raft state metrics
func (m *AdminMetrics) RecordRaftState(state, term, index uint64) {
	m.raftState.Set(float64(state))
	m.raftTerm.Set(float64(term))
	m.raftIndex.Set(float64(index))
}

// RecordLeaderElection increments leader election counter
func (m *AdminMetrics) RecordLeaderElection() {
	m.leaderElections.Inc()
}

// RecordLeaderChange increments leader change counter
func (m *AdminMetrics) RecordLeaderChange() {
	m.leaderChanges.Inc()
}

// RecordLeadershipLost increments leadership lost counter
func (m *AdminMetrics) RecordLeadershipLost() {
	m.leadershipLost.Inc()
}

// RecordFSMCommand records FSM command execution
func (m *AdminMetrics) RecordFSMCommand(commandType string, success bool, duration time.Duration) {
	status := "success"
	if !success {
		status = "error"
	}
	m.fsmCommandsTotal.WithLabelValues(commandType, status).Inc()
	m.fsmCommandDuration.WithLabelValues(commandType).Observe(duration.Seconds())
}

// RecordFSMSnapshot increments snapshot counter
func (m *AdminMetrics) RecordFSMSnapshot() {
	m.fsmSnapshotsTotal.Inc()
}

// RecordFSMRestore increments restore counter
func (m *AdminMetrics) RecordFSMRestore() {
	m.fsmRestoresTotal.Inc()
}

// RecordRaftProposal records Raft proposal metrics
func (m *AdminMetrics) RecordRaftProposal(success bool, duration time.Duration) {
	m.raftProposalsTotal.Inc()
	if !success {
		m.raftProposalsFailed.Inc()
	}
	m.raftProposalDuration.Observe(duration.Seconds())
}

// UpdateClusterMetrics updates cluster health metrics
func (m *AdminMetrics) UpdateClusterMetrics(clusterSize, healthyPeers int) {
	m.clusterSize.Set(float64(clusterSize))
	m.healthyPeers.Set(float64(healthyPeers))
}

// UpdateAdminStateMetrics updates admin state counts from FSM
func (m *AdminMetrics) UpdateAdminStateMetrics(proxies, launchers, namespaces, patterns int) {
	m.proxiesTotal.Set(float64(proxies))
	m.launchersTotal.Set(float64(launchers))
	m.namespacesTotal.Set(float64(namespaces))
	m.patternsTotal.Set(float64(patterns))
}

// RecordForwardedRequest records follower → leader forwarding
func (m *AdminMetrics) RecordForwardedRequest(rpcMethod string, success bool) {
	status := "success"
	if !success {
		status = "error"
	}
	m.forwardedRequests.WithLabelValues(rpcMethod, status).Inc()
}
