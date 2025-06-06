// Copyright 2018 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package gossip

import (
	"testing"

	"github.com/cockroachdb/cockroach/pkg/util/leaktest"
)

func TestGossipStatus(t *testing.T) {
	defer leaktest.AfterTest(t)()

	ss := ServerStatus{
		ConnStatus: []ConnStatus{
			{NodeID: 1, Address: "localhost:1234", AgeNanos: 17e9},
			{NodeID: 4, Address: "localhost:4567", AgeNanos: 18e9},
		},
		MaxConns: 3,
		MetricSnap: MetricSnap{
			BytesReceived:    1000,
			BytesSent:        2000,
			InfosReceived:    10,
			InfosSent:        20,
			MessagesSent:     2,
			MessagesReceived: 1,
			ConnsRefused:     17,
		},
	}
	if exp, act := `gossip server (2/3 cur/max conns, messages 2/1 sent/received, infos 20/10 sent/received, bytes 2000B/1000B sent/received, refused 17 conns)
  1: localhost:1234 (17s)
  4: localhost:4567 (18s)
`, ss.String(); exp != act {
		t.Errorf("expected:\n%q\ngot:\n%q", exp, act)
	}

	cs := ClientStatus{
		ConnStatus: []OutgoingConnStatus{
			{
				ConnStatus: ss.ConnStatus[0],
				MetricSnap: MetricSnap{BytesReceived: 77, BytesSent: 88, InfosReceived: 11, InfosSent: 22, MessagesReceived: 3, MessagesSent: 4},
			},
		},
		MaxConns: 3,
	}
	if exp, act := `gossip client (1/3 cur/max conns)
  1: localhost:1234 (17s: messages 4/3 sent/received, infos 22/11 sent/received, bytes 88B/77B sent/received)
`, cs.String(); exp != act {
		t.Errorf("expected:\n%q\ngot:\n%q", exp, act)
	}

}
