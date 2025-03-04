// Copyright 2024 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package vecstore

import (
	"context"
	"testing"

	"github.com/cockroachdb/cockroach/pkg/base"
	"github.com/cockroachdb/cockroach/pkg/keys"
	"github.com/cockroachdb/cockroach/pkg/sql/catalog"
	"github.com/cockroachdb/cockroach/pkg/sql/catalog/catenumpb"
	"github.com/cockroachdb/cockroach/pkg/sql/catalog/descpb"
	"github.com/cockroachdb/cockroach/pkg/sql/catalog/descs"
	"github.com/cockroachdb/cockroach/pkg/sql/catalog/desctestutils"
	"github.com/cockroachdb/cockroach/pkg/sql/catalog/tabledesc"
	"github.com/cockroachdb/cockroach/pkg/sql/sem/idxtype"
	"github.com/cockroachdb/cockroach/pkg/sql/vecindex/quantize"
	"github.com/cockroachdb/cockroach/pkg/sql/vecindex/veclib"
	"github.com/cockroachdb/cockroach/pkg/testutils/serverutils"
	"github.com/cockroachdb/cockroach/pkg/testutils/sqlutils"
	"github.com/cockroachdb/cockroach/pkg/util/encoding"
	"github.com/cockroachdb/cockroach/pkg/util/leaktest"
	"github.com/cockroachdb/cockroach/pkg/util/vector"
	"github.com/stretchr/testify/require"
)

func TestPersistentStore(t *testing.T) {
	defer leaktest.AfterTest(t)()

	var workspace veclib.Workspace
	ctx := context.Background()
	srv, sqlDB, kvDB := serverutils.StartServer(t, base.TestServerArgs{})
	internalDB := srv.ApplicationLayer().InternalDB().(descs.DB)
	codec := srv.ApplicationLayer().Codec()
	tdb := sqlutils.MakeSQLRunner(sqlDB)
	defer srv.Stopper().Stop(ctx)

	childKey2 := ChildKey{PartitionKey: 2}
	childKey10 := ChildKey{PartitionKey: 10}
	childKey20 := ChildKey{PartitionKey: 20}
	childKey30 := ChildKey{PartitionKey: 30}
	valueBytes2 := ValueBytes{1, 2}
	valueBytes10 := ValueBytes{3, 4}
	valueBytes20 := ValueBytes{5, 6}
	valueBytes30 := ValueBytes{7, 8}
	primaryKey200 := ChildKey{KeyBytes: KeyBytes{2, 00}}
	primaryKey300 := ChildKey{KeyBytes: KeyBytes{3, 00}}
	primaryKey400 := ChildKey{KeyBytes: KeyBytes{4, 00}}
	valueBytes200 := ValueBytes{9, 10}
	valueBytes300 := ValueBytes{11, 12}
	valueBytes400 := ValueBytes{13, 14}

	tdb.Exec(t, "CREATE TABLE t (id INT PRIMARY KEY, v VECTOR(2))")
	tdb.Exec(t, "INSERT INTO t VALUES (11, '[100, 200]'), (12, '[300, 400]')")

	tableDesc := desctestutils.TestingGetPublicTableDescriptor(kvDB, codec, "defaultdb", "t")

	col, err := catalog.MustFindColumnByName(tableDesc, "v")
	require.NoError(t, err)

	indexDesc := descpb.IndexDescriptor{
		ID: 42, Name: "idx_vector_t",
		Type:                idxtype.VECTOR,
		KeyColumnIDs:        []descpb.ColumnID{col.GetID()},
		KeyColumnNames:      []string{col.GetName()},
		KeyColumnDirections: []catenumpb.IndexColumn_Direction{catenumpb.IndexColumn_ASC},
		KeySuffixColumnIDs:  []descpb.ColumnID{tableDesc.GetPrimaryIndex().GetKeyColumnID(0)},
		Version:             descpb.LatestIndexDescriptorVersion,
		EncodingType:        catenumpb.SecondaryIndexEncoding,
	}
	index := tabledesc.NewTestIndex(&indexDesc, 1)

	quantizer := quantize.NewUnQuantizer(2)
	store, err := NewPersistentStoreWithColumnID(
		ctx,
		internalDB,
		quantizer,
		codec,
		tableDesc,
		index.GetID(),
		col.GetID(),
	)
	require.NoError(t, err)

	pk1 := keys.MakeFamilyKey(encoding.EncodeVarintAscending([]byte{}, 11), 0 /* famID */)
	pk2 := keys.MakeFamilyKey(encoding.EncodeVarintAscending([]byte{}, 12), 0 /* famID */)
	testPKs := []KeyBytes{pk1, pk2}
	testVectors := []vector.T{{100, 200}, {300, 400}}

	// TODO(mw5h): Figure out where to create the empty root partition.
	t.Run("create empty root partition", func(t *testing.T) {
		txn := beginTransaction(ctx, t, &workspace, store)
		defer commitTransaction(ctx, t, store, txn)

		emptyRoot := NewPartition(
			quantizer, quantizer.Quantize(&workspace, vector.Set{}), []ChildKey{}, []ValueBytes{}, LeafLevel)
		require.NoError(t, txn.SetRootPartition(ctx, emptyRoot))
	})

	commonStoreTests(ctx, t, store, quantizer, testPKs, testVectors)

	t.Run("insert a root partition into the store and read it back", func(t *testing.T) {
		txn := beginTransaction(ctx, t, &workspace, store)
		defer commitTransaction(ctx, t, store, txn)

		vectors := vector.T{4, 3}.AsSet()
		quantizedSet := quantizer.Quantize(&workspace, vectors)
		root := NewPartition(
			quantizer, quantizedSet, []ChildKey{childKey2}, []ValueBytes{valueBytes2}, Level(2))
		require.NoError(t, txn.SetRootPartition(ctx, root))
		readRoot, err := txn.GetPartition(ctx, RootKey)
		require.NoError(t, err)
		testingAssertPartitionsEqual(t, root, readRoot)

		vectors = vector.T{4, 3}.AsSet()
		vectors.Add(vector.T{2, 1})
		quantizedSet = quantizer.Quantize(&workspace, vectors)
		root = NewPartition(
			quantizer, quantizedSet, []ChildKey{childKey10, childKey20},
			[]ValueBytes{valueBytes10, valueBytes20}, Level(2))
		require.NoError(t, txn.SetRootPartition(ctx, root))
		readRoot, err = txn.GetPartition(ctx, RootKey)
		require.NoError(t, err)
		testingAssertPartitionsEqual(t, root, readRoot)

		vectors = vector.T{4, 3}.AsSet()
		vectors.Add(vector.T{2, 1})
		vectors.Add(vector.T{5, 6})
		quantizedSet = quantizer.Quantize(&workspace, vectors)
		root = NewPartition(
			quantizer, quantizedSet, []ChildKey{primaryKey200, primaryKey300, primaryKey400},
			[]ValueBytes{valueBytes200, valueBytes300, valueBytes400}, LeafLevel)
		require.NoError(t, txn.SetRootPartition(ctx, root))
		readRoot, err = txn.GetPartition(ctx, RootKey)
		require.NoError(t, err)
		testingAssertPartitionsEqual(t, root, readRoot)
	})

	t.Run("insert a partition and then delete it", func(t *testing.T) {
		txn := beginTransaction(ctx, t, &workspace, store)
		defer commitTransaction(ctx, t, store, txn)

		vectors := vector.T{4, 3}.AsSet()
		quantizedSet := quantizer.Quantize(&workspace, vectors)
		testPartition := NewPartition(
			quantizer, quantizedSet, []ChildKey{childKey2}, []ValueBytes{valueBytes2}, Level(2))
		partitionKey, err := txn.InsertPartition(ctx, testPartition)
		require.NoError(t, err)
		newPartition, err := txn.GetPartition(ctx, partitionKey)
		require.NoError(t, err)
		testingAssertPartitionsEqual(t, testPartition, newPartition)

		err = txn.DeletePartition(ctx, partitionKey)
		require.NoError(t, err)
		_, err = txn.GetPartition(ctx, partitionKey)
		require.Error(t, err)
	})

	t.Run("add to root partition", func(t *testing.T) {
		txn := beginTransaction(ctx, t, &workspace, store)
		defer commitTransaction(ctx, t, store, txn)

		emptySet := vector.MakeSet(2)
		root := NewPartition(
			quantizer, quantizer.Quantize(&workspace, emptySet), []ChildKey{}, []ValueBytes{}, Level(2))
		err := txn.SetRootPartition(ctx, root)
		require.NoError(t, err)

		// Add to root partition.
		metadata, err := txn.AddToPartition(ctx, RootKey, vector.T{1, 2}, childKey10, valueBytes10)
		require.NoError(t, err)
		checkPartitionMetadata(t, metadata, Level(2), vector.T{0, 0}, 1)
		metadata, err = txn.AddToPartition(ctx, RootKey, vector.T{7, 4}, childKey20, valueBytes20)
		require.NoError(t, err)
		checkPartitionMetadata(t, metadata, Level(2), vector.T{0, 0}, 2)
		metadata, err = txn.AddToPartition(ctx, RootKey, vector.T{4, 3}, childKey30, valueBytes30)
		require.NoError(t, err)
		checkPartitionMetadata(t, metadata, Level(2), vector.T{0, 0}, 3)

		// Add duplicate and expect value to be overwritten
		metadata, err = txn.AddToPartition(ctx, RootKey, vector.T{5, 5}, childKey30, valueBytes30)
		require.NoError(t, err)
		checkPartitionMetadata(t, metadata, Level(2), vector.T{0, 0}, 3)

		// Search root partition.
		searchSet := SearchSet{MaxResults: 2}
		partitionCounts := []int{0}
		level, err := txn.SearchPartitions(
			ctx, []PartitionKey{RootKey}, vector.T{1, 1}, &searchSet, partitionCounts)
		require.NoError(t, err)
		require.Equal(t, Level(2), level)
		result1 := SearchResult{
			QuerySquaredDistance: 1, ErrorBound: 0, CentroidDistance: 2.2361, ParentPartitionKey: 1, ChildKey: childKey10, ValueBytes: valueBytes10}
		result2 := SearchResult{
			QuerySquaredDistance: 32, ErrorBound: 0, CentroidDistance: 7.0711, ParentPartitionKey: 1, ChildKey: childKey30, ValueBytes: valueBytes30}
		results := searchSet.PopResults()
		roundResults(results, 4)
		require.Equal(t, SearchResults{result1, result2}, results)
		require.Equal(t, 3, partitionCounts[0])
	})
}
