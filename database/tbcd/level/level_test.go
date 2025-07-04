// Copyright (c) 2025 Hemi Labs, Inc.
// Use of this source code is governed by the MIT License,
// which can be found in the LICENSE file.

package level

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/davecgh/go-spew/spew"
	"github.com/go-test/deep"

	"github.com/hemilabs/heminetwork/database"
	"github.com/hemilabs/heminetwork/database/tbcd"
	"github.com/hemilabs/heminetwork/hemi"
	"github.com/hemilabs/heminetwork/testutil"
)

func TestMD(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	home := t.TempDir()
	t.Logf("temp: %v", home)

	cfg, err := NewConfig("testnet3", home, "128kb", "1m")
	if err != nil {
		t.Fatal(err)
	}
	db, err := New(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := db.Close()
		if err != nil {
			t.Fatal(err)
		}
	}()

	x := 255
	rows := make([]tbcd.Row, x+1)
	for i := 0; i <= x; i++ {
		y := byte(i)
		rows[i] = tbcd.Row{
			Key:   []byte{y},
			Value: []byte{y, y, y, y},
		}
	}
	err = db.MetadataBatchPut(ctx, rows)
	if err != nil {
		t.Fatal(err)
	}

	qr := make([][]byte, 0, x+1)
	for i := 0; i <= x; i++ {
		qr = append(qr, []byte{byte(i)})
	}
	rrows, err := db.MetadataBatchGet(ctx, true, qr)
	if err != nil {
		t.Fatal(err)
	}
	for k := range rrows {
		if !reflect.DeepEqual(rrows[k], rows[k]) {
			t.Fatalf("expected %v got %v",
				spew.Sdump(rows[k]), spew.Sdump(rrows[k]))
		}
	}

	// fail
	qr = append(qr, []byte{1, 2, 3, 4}) // unknown key
	rrows, err = db.MetadataBatchGet(ctx, true, qr)
	if !errors.Is(err, database.ErrNotFound) {
		t.Fatalf("expected '%v', got '%v'", database.ErrNotFound, err)
	}
	if rrows != nil {
		t.Fatal("expected no return value")
	}

	// don't fail but check error
	rrows, err = db.MetadataBatchGet(ctx, false, qr)
	if err != nil {
		t.Fatal(err)
	}
	if rrows[x+1].Error == nil {
		t.Fatal(err)
	}
	for k := range rrows[:x] {
		if !reflect.DeepEqual(rrows[k], rows[k]) {
			t.Fatalf("expected %v got %v",
				spew.Sdump(rows[k]), spew.Sdump(rrows[k]))
		}
	}

	// Individual put/get
	key := []byte("mysuperkey")
	value := []byte("valuevaluevalue")
	err = db.MetadataPut(ctx, key, value)
	if err != nil {
		t.Fatal(err)
	}
	rv, err := db.MetadataGet(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(rv, value) {
		t.Fatalf("got %s, expected %s", rv, value)
	}

	// fail one
	rv, err = db.MetadataGet(ctx, []byte("nope"))
	if !errors.Is(err, database.ErrNotFound) {
		t.Fatalf("expected '%v', got '%v'", database.ErrNotFound, err)
	}
	if rv != nil {
		t.Fatal("expected no return value")
	}
}

func makeKssMap(kssList []hemi.L2Keystone, blockHashSeed string) map[chainhash.Hash]tbcd.Keystone {
	kssMap := make(map[chainhash.Hash]tbcd.Keystone)
	for _, l2Keystone := range kssList {
		abrvKs := hemi.L2KeystoneAbbreviate(l2Keystone).Serialize()
		kssMap[*hemi.L2KeystoneAbbreviate(l2Keystone).Hash()] = tbcd.Keystone{
			BlockHash:           chainhash.Hash(testutil.FillBytes(blockHashSeed, 32)),
			AbbreviatedKeystone: abrvKs,
		}
	}
	return kssMap
}

func TestKssEncoding(t *testing.T) {
	keystones := []hemi.L2Keystone{
		{
			Version:            1,
			L1BlockNumber:      5,
			L2BlockNumber:      44,
			ParentEPHash:       testutil.FillBytes("parentephash", 32),
			PrevKeystoneEPHash: testutil.FillBytes("prevkeystoneephash", 32),
			StateRoot:          testutil.FillBytes("stateroot", 32),
			EPHash:             testutil.FillBytes("ephash", 32),
		}, {
			Version:            1,
			L1BlockNumber:      5,
			L2BlockNumber:      44,
			ParentEPHash:       testutil.FillBytes("altparentephash", 32),
			PrevKeystoneEPHash: testutil.FillBytes("altprevkeystoneephash", 32),
			StateRoot:          testutil.FillBytes("altstateroot", 32),
			EPHash:             testutil.FillBytes("altephash", 32),
		},
	}

	altKeystone := hemi.L2Keystone{
		Version:            2,
		L1BlockNumber:      6,
		L2BlockNumber:      64,
		ParentEPHash:       testutil.FillBytes("fakeparentephash", 32),
		PrevKeystoneEPHash: testutil.FillBytes("fakeprevkeystoneephash", 32),
		StateRoot:          testutil.FillBytes("fakestateroot", 32),
		EPHash:             testutil.FillBytes("fakeephash", 32),
	}
	altAbrevKeystone := hemi.L2KeystoneAbbreviate(altKeystone).Serialize()

	kssMap := makeKssMap(keystones, "blockhash")
	for _, ks := range kssMap {
		encodedKs := encodeKeystoneToSlice(ks)
		decodedKs := decodeKeystone(encodedKs)

		if !decodedKs.BlockHash.IsEqual(&ks.BlockHash) {
			t.Fatalf("blockhash diff: got %v, expected %v", decodedKs.BlockHash, ks.BlockHash)
		}
		if diff := deep.Equal(decodedKs.AbbreviatedKeystone, ks.AbbreviatedKeystone); len(diff) > 0 {
			t.Fatalf("abrv Ks diff: %s", diff)
		}
	}
	diffKssMap := makeKssMap(keystones, "diffblockhash")
	for key, ks := range diffKssMap {
		dks := kssMap[key]
		encodedKs := encodeKeystoneToSlice(dks)
		decodedKs := decodeKeystone(encodedKs)

		if decodedKs.BlockHash.IsEqual(&ks.BlockHash) {
			t.Fatalf("blockhash not diff: got %v, expected %v", decodedKs.BlockHash, ks.BlockHash)
		}

		if diff := deep.Equal(decodedKs.AbbreviatedKeystone, altAbrevKeystone); len(diff) == 0 {
			t.Fatalf("abrv Ks diff: %v", diff)
		}
	}
}

func TestKeystoneUpdate(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	kssList := []hemi.L2Keystone{
		{
			Version:            1,
			L1BlockNumber:      5,
			L2BlockNumber:      44,
			ParentEPHash:       testutil.FillBytes("v1parentephash", 32),
			PrevKeystoneEPHash: testutil.FillBytes("v1prevkeystoneephash", 32),
			StateRoot:          testutil.FillBytes("v1stateroot", 32),
			EPHash:             testutil.FillBytes("v1ephash", 32),
		},
		{
			Version:            1,
			L1BlockNumber:      6,
			L2BlockNumber:      44,
			ParentEPHash:       testutil.FillBytes("v2parentephash", 32),
			PrevKeystoneEPHash: testutil.FillBytes("v2prevkeystoneephash", 32),
			StateRoot:          testutil.FillBytes("v2stateroot", 32),
			EPHash:             testutil.FillBytes("v2ephash", 32),
		},
		{
			Version:            1,
			L1BlockNumber:      5,
			L2BlockNumber:      44,
			ParentEPHash:       testutil.FillBytes("i1parentephash", 32),
			PrevKeystoneEPHash: testutil.FillBytes("i1prevkeystoneephash", 32),
			StateRoot:          testutil.FillBytes("i1stateroot", 32),
			EPHash:             testutil.FillBytes("i1ephash", 32),
		},
		{
			Version:            1,
			L1BlockNumber:      6,
			L2BlockNumber:      44,
			ParentEPHash:       testutil.FillBytes("i2parentephash", 32),
			PrevKeystoneEPHash: testutil.FillBytes("i2prevkeystoneephash", 32),
			StateRoot:          testutil.FillBytes("i2stateroot", 32),
			EPHash:             testutil.FillBytes("i2ephash", 32),
		},
	}

	type testTableItem struct {
		name           string
		direction      []int
		preInsertValid bool
		kssMap         map[chainhash.Hash]tbcd.Keystone
		expectedInDB   map[chainhash.Hash]tbcd.Keystone
		expectedOutDB  map[chainhash.Hash]tbcd.Keystone
		expectedError  error
	}

	testTable := []testTableItem{
		{
			name:          "invalidDirection",
			direction:     []int{0},
			expectedOutDB: makeKssMap(kssList[:], "blockhash"),
			expectedError: fmt.Errorf("invalid direction: %v", 0),
		},
		{
			name:          "nilMap",
			direction:     []int{-1, 1},
			expectedOutDB: makeKssMap(kssList, "blockhash"),
		},
		{
			name:          "emptyMap",
			direction:     []int{-1, 1},
			kssMap:        makeKssMap(nil, "blockhash"),
			expectedOutDB: makeKssMap(kssList, "blockhash"),
		},

		{
			name:           "duplicateInsert",
			direction:      []int{1},
			expectedError:  nil,
			preInsertValid: true,
			kssMap:         makeKssMap(kssList[:2], "blockhash"),
			expectedInDB:   makeKssMap(kssList[:2], "blockhash"),
			expectedOutDB:  makeKssMap(kssList[2:], "blockhash"),
		},

		{
			name:          "invalidRemove",
			direction:     []int{-1},
			kssMap:        makeKssMap(kssList[2:], "blockhash"),
			expectedOutDB: makeKssMap(kssList, "blockhash"),
		},
		{
			name:          "validInsert",
			direction:     []int{1},
			kssMap:        makeKssMap(kssList[:2], "blockhash"),
			expectedInDB:  makeKssMap(kssList[:2], "blockhash"),
			expectedOutDB: makeKssMap(kssList[2:], "blockhash"),
		},
		{
			name:          "validRemove",
			direction:     []int{1, -1},
			kssMap:        makeKssMap(kssList[2:], "blockhash"),
			expectedOutDB: makeKssMap(kssList, "blockhash"),
		},
		{
			name:           "mixedRemove",
			direction:      []int{-1},
			preInsertValid: true,
			kssMap:         makeKssMap(kssList, "blockhash"),
			expectedOutDB:  makeKssMap(kssList, "blockhash"),
		},
		{
			name:           "mixedInsert",
			direction:      []int{1},
			preInsertValid: true,
			kssMap:         makeKssMap(kssList, "blockhash"),
			expectedInDB:   makeKssMap(kssList, "blockhash"),
		},
		{
			name:           "invalidBlockhashRemove",
			direction:      []int{-1},
			preInsertValid: true,
			kssMap:         makeKssMap(kssList, "fakeblockhash"),
			expectedInDB:   makeKssMap(kssList[:2], "blockhash"),
		},
	}

	blockhash := chainhash.Hash{1, 3, 3, 7}
	for _, tti := range testTable {
		t.Run(tti.name, func(t *testing.T) {
			home := t.TempDir()
			t.Logf("temp: %v", home)

			cfg, err := NewConfig("testnet3", home, "", "")
			if err != nil {
				t.Fatal(err)
			}
			db, err := New(ctx, cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				err := db.Close()
				if err != nil {
					t.Fatal(err)
				}
			}()

			if tti.preInsertValid {
				if err := db.BlockKeystoneUpdate(ctx, 1, makeKssMap(kssList[:2], "blockhash"), blockhash); err != nil {
					t.Fatal(err)
				}
			}

			for _, dir := range tti.direction {
				err := db.BlockKeystoneUpdate(ctx, dir, maps.Clone(tti.kssMap), blockhash)
				if diff := deep.Equal(err, tti.expectedError); len(diff) > 0 {
					t.Fatalf("(direction %v) unexpected error diff: %s", dir, diff)
				}
			}

			for v := range tti.expectedInDB {
				_, err := db.BlockKeystoneByL2KeystoneAbrevHash(ctx, v)
				if err != nil {
					t.Fatalf("keystone not in db: %v", err)
				}
			}

			for k, v := range tti.expectedOutDB {
				_, err := db.BlockKeystoneByL2KeystoneAbrevHash(ctx, k)
				if err == nil {
					t.Fatalf("keystone in db: %v", spew.Sdump(v))
				} else {
					if !errors.Is(err, database.ErrNotFound) {
						t.Fatalf("expected '%v', got '%v'", database.ErrNotFound, err)
					}
				}
			}
		})
	}
}

func newKeystone(blockhash *chainhash.Hash, l1, l2 uint32) (*chainhash.Hash, tbcd.Keystone) {
	hks := hemi.L2Keystone{
		Version:            1,
		L1BlockNumber:      l1,
		L2BlockNumber:      l2,
		ParentEPHash:       testutil.FillBytes("v1parentephash", 32),
		PrevKeystoneEPHash: testutil.FillBytes("v1prevkeystoneephash", 32),
		StateRoot:          testutil.FillBytes("v1stateroot", 32),
		EPHash:             testutil.FillBytes("v1ephash", 32),
	}
	abrvKs := hemi.L2KeystoneAbbreviate(hks)
	return abrvKs.Hash(), tbcd.Keystone{
		BlockHash:           *blockhash,
		AbbreviatedKeystone: abrvKs.Serialize(),
	}
}

func TestKeystoneDBWindUnwind(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	home := t.TempDir()
	t.Logf("temp: %v", home)

	cfg, err := NewConfig("testnet3", home, "", "")
	if err != nil {
		t.Fatal(err)
	}
	db, err := New(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := db.Close()
		if err != nil {
			t.Fatal(err)
		}
	}()

	blockhash := chainhash.Hash{0xde, 0xad, 0xbe, 0xef}
	blk1Hash := chainhash.Hash{1}
	k1hash, k1 := newKeystone(&blk1Hash, 1, 2)
	blk2Hash := chainhash.Hash{1}
	k2hash, k2 := newKeystone(&blk2Hash, 2, 3)
	ksm := map[chainhash.Hash]tbcd.Keystone{
		*k1hash: k1,
		*k2hash: k2,
	}
	err = db.BlockKeystoneUpdate(ctx, 1, maps.Clone(ksm), blockhash)
	if err != nil {
		t.Fatal(err)
	}

	// Get keystones back out
	ks1, err := db.BlockKeystoneByL2KeystoneAbrevHash(ctx, *k1hash)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(k1, *ks1) {
		t.Fatalf("%v%v", spew.Sdump(k1), spew.Sdump(*ks1))
	}
	ks2, err := db.BlockKeystoneByL2KeystoneAbrevHash(ctx, *k2hash)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(k2, *ks2) {
		t.Fatalf("%v%v", spew.Sdump(k2), spew.Sdump(*ks2))
	}

	// Unwind
	// Technically don't need to clone ksm here, but do it for coherency
	err = db.BlockKeystoneUpdate(ctx, -1, maps.Clone(ksm), blockhash)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.BlockKeystoneByL2KeystoneAbrevHash(ctx, *k1hash)
	if err == nil {
		t.Fatal("k1 found")
	}
	_, err = db.BlockKeystoneByL2KeystoneAbrevHash(ctx, *k2hash)
	if err == nil {
		t.Fatal("k2 found")
	}
}

func TestKeystoneDBCache(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	home := t.TempDir()
	t.Logf("temp: %v", home)

	cfg, err := NewConfig("upgradetest", home, "", "")
	if err != nil {
		t.Fatal(err)
	}
	db, err := New(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := db.Close()
		if err != nil {
			t.Fatal(err)
		}
	}()

	// XXX make these higher when cache fixed
	const (
		kssNum = 1
		cycles = 1
	)

	blockhash := chainhash.Hash{0xba, 0xdc, 0x0f, 0xfe}
	for i := range cycles {
		ksm := make(map[chainhash.Hash]tbcd.Keystone, kssNum)
		for j := range kssNum {
			blkHash := chainhash.Hash{byte(i)}
			ksHash, ks := newKeystone(&blkHash, uint32(j), uint32(j))
			ksm[*ksHash] = ks
		}

		err = db.BlockKeystoneUpdate(ctx, 1, ksm, blockhash)
		if err != nil {
			t.Fatal(err)
		}

		for ksHash, ks := range ksm {
			dks, err := db.BlockKeystoneByL2KeystoneAbrevHash(ctx, ksHash)
			if err != nil {
				t.Fatal(err)
			}
			if diff := deep.Equal(ks, *dks); len(diff) > 0 {
				t.Fatalf("(cycle %v) unexpected error diff: %v", i, diff)
			}
		}

		err = db.BlockKeystoneUpdate(ctx, -1, ksm, blockhash)
		if err != nil {
			t.Fatal(err)
		}

		for ksHash := range ksm {
			ks, err := db.BlockKeystoneByL2KeystoneAbrevHash(ctx, ksHash)
			if err == nil {
				t.Fatalf("(cycle %v) deleted keystone found %v", i, spew.Sdump(ks))
			}
		}
	}
}
