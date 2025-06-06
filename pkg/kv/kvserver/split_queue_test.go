// Copyright 2015 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package kvserver

import (
	"context"
	"math"
	"testing"

	"github.com/cockroachdb/cockroach/pkg/config"
	"github.com/cockroachdb/cockroach/pkg/config/zonepb"
	"github.com/cockroachdb/cockroach/pkg/keys"
	"github.com/cockroachdb/cockroach/pkg/kv"
	"github.com/cockroachdb/cockroach/pkg/kv/kvserver/stateloader"
	"github.com/cockroachdb/cockroach/pkg/roachpb"
	"github.com/cockroachdb/cockroach/pkg/storage/enginepb"
	"github.com/cockroachdb/cockroach/pkg/util/leaktest"
	"github.com/cockroachdb/cockroach/pkg/util/log"
	"github.com/cockroachdb/cockroach/pkg/util/stop"
	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/require"
)

// TestSplitQueueShouldQueue verifies shouldSplitRange method correctly
// combines splits in zone configs with the size of the range.
func TestSplitQueueShouldQueue(t *testing.T) {
	defer leaktest.AfterTest(t)()
	defer log.Scope(t).Close(t)
	ctx := context.Background()
	tc := testContext{}
	stopper := stop.NewStopper()
	defer stopper.Stop(ctx)
	sc := TestStoreConfig(nil)
	sc.TestingKnobs.DisableCanAckBeforeApplication = true
	tc.StartWithStoreConfig(ctx, t, stopper, sc)

	// Set zone configs.
	config.TestingSetZoneConfig(2000, zonepb.ZoneConfig{RangeMaxBytes: proto.Int64(32 << 20)})
	config.TestingSetZoneConfig(2002, zonepb.ZoneConfig{RangeMaxBytes: proto.Int64(32 << 20)})

	testCases := []struct {
		start, end roachpb.RKey
		bytes      int64
		maxBytes   int64
		shouldQ    bool
		priority   float64
	}{
		// No intersection, no bytes, no load.
		{roachpb.RKeyMin, roachpb.RKey(keys.MetaMax), 0, 64 << 20, false, 0},
		// Intersection in zone, no bytes, no load.
		{roachpb.RKey(keys.SystemSQLCodec.TablePrefix(2001)), roachpb.RKeyMax, 0, 64 << 20, true, 1},
		// Already split at largest ID, no load.
		{roachpb.RKey(keys.SystemSQLCodec.TablePrefix(2002)), roachpb.RKeyMax, 0, 32 << 20, false, 0},
		// Multiple intersections, no bytes, no load.
		{roachpb.RKeyMin, roachpb.RKeyMax, 0, 64 << 20, true, 1},
		// No intersection, max bytes, no load.
		{roachpb.RKeyMin, roachpb.RKey(keys.MetaMax), 64 << 20, 64 << 20, false, 0},
		// No intersection, max bytes+1, no load.
		{roachpb.RKeyMin, roachpb.RKey(keys.MetaMax), 64<<20 + 1, 64 << 20, true, 1},
		// No intersection, max bytes * 2 + 2, no load, should backpressure.
		{roachpb.RKeyMin, roachpb.RKey(keys.MetaMax), 64<<21 + 2, 64 << 20, true, 52},
		// No intersection, max bytes * 4, no load, should not backpressure.
		{roachpb.RKeyMin, roachpb.RKey(keys.MetaMax), 64 << 22, 64 << 20, true, 4},
		// Intersection, max bytes +1, no load.
		{roachpb.RKey(keys.SystemSQLCodec.TablePrefix(2000)), roachpb.RKeyMax, 32<<20 + 1, 32 << 20, true, 2},
		// Split needed at table boundary, but no zone config, no load.
		{roachpb.RKey(keys.SystemSQLCodec.TablePrefix(2001)), roachpb.RKeyMax, 32<<20 + 1, 64 << 20, true, 1},
	}

	cfg, err := tc.store.GetConfReader(ctx)
	if err != nil {
		t.Fatal(err)
	}

	{
		// This test plays fast and loose and if there are raft commands ongoing then
		// we'll hit internal assertions in the tests below. Sending a write through
		// before mucking with internals and waiting for it to show up in the state
		// machine appears to be good enough.
		put := putArgs(tc.repl.Desc().EndKey.AsRawKey().Prevish(5), []byte("foo"))
		_, pErr := kv.SendWrapped(ctx, tc.Sender(), &put)
		require.NoError(t, pErr.GoError())
	}

	for i, test := range testCases {
		// Create a replica for testing that is not hooked up to the store. This
		// ensures that the store won't be mucking with our replica concurrently
		// during testing (e.g. via the system config gossip update).
		cpy := *tc.repl.Desc()
		cpy.StartKey = test.start
		cpy.EndKey = test.end
		replicaID := cpy.Replicas().VoterDescriptors()[0].ReplicaID
		require.NoError(t,
			stateloader.Make(cpy.RangeID).SetRaftReplicaID(ctx, tc.store.TODOEngine(), replicaID))
		repl, err := loadInitializedReplicaForTesting(ctx, tc.store, &cpy, replicaID)
		if err != nil {
			t.Fatal(err)
		}

		repl.mu.Lock()
		repl.shMu.state.Stats = &enginepb.MVCCStats{KeyBytes: test.bytes}
		repl.mu.Unlock()
		conf := roachpb.TestingDefaultSpanConfig()
		conf.RangeMaxBytes = test.maxBytes
		sp := roachpb.Span{
			Key:    cpy.StartKey.AsRawKey().Clone(),
			EndKey: cpy.EndKey.AsRawKey().Clone(),
		}
		repl.SetSpanConfig(conf, sp)

		// Testing using shouldSplitRange instead of shouldQueue to avoid using the splitFinder
		// This tests the merge queue behavior too as a result. For splitFinder tests,
		// see split/split_test.go.
		shouldQ, priority := shouldSplitRange(ctx, repl.Desc(), repl.GetMVCCStats(),
			repl.GetMaxBytes(ctx), repl.ShouldBackpressureWrites(ctx), cfg)
		if shouldQ != test.shouldQ {
			t.Errorf("%d: should queue expected %t; got %t", i, test.shouldQ, shouldQ)
		}
		if math.Abs(priority-test.priority) > 0.00001 {
			t.Errorf("%d: priority expected %f; got %f", i, test.priority, priority)
		}
	}
}
