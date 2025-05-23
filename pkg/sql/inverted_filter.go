// Copyright 2020 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package sql

import (
	"context"

	"github.com/cockroachdb/cockroach/pkg/sql/catalog/colinfo"
	"github.com/cockroachdb/cockroach/pkg/sql/inverted"
	"github.com/cockroachdb/cockroach/pkg/sql/physicalplan"
	"github.com/cockroachdb/cockroach/pkg/sql/sem/tree"
	"github.com/cockroachdb/cockroach/pkg/sql/types"
)

type invertedFilterNode struct {
	singleInputPlanNode
	invertedFilterPlanningInfo
	columns colinfo.ResultColumns
}

type invertedFilterPlanningInfo struct {
	expression          *inverted.SpanExpression
	preFiltererExpr     tree.TypedExpr
	preFiltererType     *types.T
	invColumn           int
	finalizeLastStageCb func(*physicalplan.PhysicalPlan) // will be nil in the spec factory
}

func (n *invertedFilterNode) startExec(params runParams) error {
	panic("invertedFiltererNode can't be run in local mode")
}
func (n *invertedFilterNode) Close(ctx context.Context) {
	n.input.Close(ctx)
}
func (n *invertedFilterNode) Next(params runParams) (bool, error) {
	panic("invertedFiltererNode can't be run in local mode")
}
func (n *invertedFilterNode) Values() tree.Datums {
	panic("invertedFiltererNode can't be run in local mode")
}
