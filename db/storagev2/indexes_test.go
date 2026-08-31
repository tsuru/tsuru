// Copyright 2026 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagev2

import (
	"sync"
	"testing"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestCopyIndexModelsClonesOptions(t *testing.T) {
	original := []mongo.IndexModel{
		{Options: options.Index().SetUnique(true)},
		{Options: nil},
	}

	got := copyIndexModels(original)

	if len(got) != len(original) {
		t.Fatalf("got %d models, want %d", len(got), len(original))
	}
	if got[0].Options == original[0].Options {
		t.Error("options pointer was shared with the original, driver writes would escape")
	}
	if got[0].Options.Unique == nil || !*got[0].Options.Unique {
		t.Error("copy lost the Unique option")
	}
	if got[1].Options != nil {
		t.Error("nil options should stay nil")
	}
}

// TestCopyIndexModelsIsolatesWriters covers the race that CreateMany triggers:
// it names any index that lacks a name, writing back into the IndexOptions it
// was handed. Run under -race this fails if the copy shares state.
func TestCopyIndexModelsIsolatesWriters(t *testing.T) {
	shared := []mongo.IndexModel{{Options: options.Index()}}

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			models := copyIndexModels(shared)
			models[0].Options.SetName("generated")
			_ = models[0].Options.Name
		}()
	}
	wg.Wait()

	if shared[0].Options.Name != nil {
		t.Errorf("writes leaked into the shared options: name = %q", *shared[0].Options.Name)
	}
}
