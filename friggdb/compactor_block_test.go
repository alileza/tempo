package friggdb

import (
	"bytes"
	"io/ioutil"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/grafana/frigg/friggdb/backend"
	"github.com/grafana/frigg/friggdb/wal"

	bloom "github.com/dgraph-io/ristretto/z"
	"github.com/dgryski/go-farm"
	"github.com/stretchr/testify/assert"
)

func TestCompactorBlockError(t *testing.T) {
	_, err := newCompactorBlock(nil, 0, 0, nil)
	assert.Error(t, err)
}

func TestCompactorBlockWrite(t *testing.T) {
	tempDir, err := ioutil.TempDir("/tmp", "")
	defer os.RemoveAll(tempDir)
	assert.NoError(t, err, "unexpected error creating temp dir")

	walCfg := &wal.Config{
		Filepath:        tempDir,
		IndexDownsample: 3,
		BloomFP:         .01,
	}
	wal, err := wal.New(walCfg)
	assert.NoError(t, err)

	metas := []*backend.BlockMeta{
		{
			StartTime: time.Unix(10000, 0),
			EndTime:   time.Unix(20000, 0),
		},
		{
			StartTime: time.Unix(15000, 0),
			EndTime:   time.Unix(25000, 0),
		},
	}

	h, err := wal.NewWorkingBlock(uuid.New(), testTenantID)
	assert.NoError(t, err)

	cb, err := newCompactorBlock(h, .01, 3, metas)
	assert.NoError(t, err)

	var minID backend.ID
	var maxID backend.ID

	numObjects := (rand.Int() % 20) + 1
	ids := make([][]byte, 0)
	for i := 0; i < numObjects; i++ {
		id := make([]byte, 16)
		_, err = rand.Read(id)
		assert.NoError(t, err)

		object := make([]byte, rand.Int()%1024)
		_, err = rand.Read(object)
		assert.NoError(t, err)

		ids = append(ids, id)

		err = cb.write(id, object)
		assert.NoError(t, err)

		if len(minID) == 0 || bytes.Compare(id, minID) == -1 {
			minID = id
		}
		if len(maxID) == 0 || bytes.Compare(id, maxID) == 1 {
			maxID = id
		}
	}

	assert.Equal(t, numObjects, cb.length())

	// test meta
	meta := cb.meta()

	assert.Equal(t, time.Unix(10000, 0), meta.StartTime)
	assert.Equal(t, time.Unix(25000, 0), meta.EndTime)
	assert.Equal(t, minID, meta.MinID)
	assert.Equal(t, maxID, meta.MaxID)
	assert.Equal(t, testTenantID, meta.TenantID)
	assert.Equal(t, numObjects, int(meta.TotalObjects))

	// bloom
	bloomBytes, err := cb.bloom()
	assert.NoError(t, err)

	bloom := bloom.JSONUnmarshal(bloomBytes)
	for _, id := range ids {
		has := bloom.Has(farm.Fingerprint64(id))
		assert.True(t, has)
	}

	// index
	indexBytes, err := cb.index()
	assert.NoError(t, err)

	_, err = backend.UnmarshalRecords(indexBytes)
	assert.NoError(t, err)
}
